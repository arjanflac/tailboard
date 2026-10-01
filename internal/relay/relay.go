package relay

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const MaxTextBytes = 1 << 20
const maxStreams = 16
const writeTimeout = 5 * time.Second

// Clip is the one current text value; there is no history or expiry schedule.
type Clip struct {
	ID       string `json:"id"`
	Seq      uint64 `json:"seq"`
	Content  string `json:"content"`
	Source   string `json:"source"`
	DeviceID string `json:"device_id,omitempty"`
}

type message struct {
	Type string `json:"type"`
	Item *Clip  `json:"item,omitempty"`
}

// Relay holds one value in memory and fans changes out to connected devices.
type Relay struct {
	mu          sync.RWMutex
	current     *Clip
	seq         uint64
	started     time.Time
	clients     map[chan message]struct{}
	applyRemote func(string) error
	clearRemote func() error
}

func New(applyRemote func(string) error, clearRemote func() error) *Relay {
	return &Relay{
		started:     time.Now(),
		clients:     make(map[chan message]struct{}),
		applyRemote: applyRemote,
		clearRemote: clearRemote,
	}
}

func (r *Relay) PutLocal(content, source string) *Clip {
	return r.put(content, source, "mac")
}

func (r *Relay) put(content, source, deviceID string) *Clip {
	r.mu.Lock()
	if r.current != nil && r.current.Content == content {
		item := *r.current
		r.mu.Unlock()
		return &item
	}
	r.seq++
	item := Clip{
		ID:  rand.Text(),
		Seq: r.seq, Content: content, Source: source, DeviceID: deviceID,
	}
	r.current = &item
	r.broadcastLocked(message{Type: "clip_update", Item: &item})
	r.mu.Unlock()
	return &item
}

func (r *Relay) Current() *Clip {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current == nil {
		return nil
	}
	item := *r.current
	return &item
}

func (r *Relay) clear() {
	r.mu.Lock()
	r.current = nil
	r.seq++
	r.broadcastLocked(message{Type: "clip_clear"})
	r.mu.Unlock()
}

// Queue in mutation order so an older update cannot follow a newer one.
func (r *Relay) broadcastLocked(update message) {
	for client := range r.clients {
		select {
		case client <- update:
		default:
			select {
			case <-client:
			default:
			}
			select {
			case client <- update:
			default:
			}
		}
	}
}

func (r *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /api/status", r.status)
	mux.HandleFunc("GET /api/clip", r.get)
	mux.HandleFunc("POST /api/clip", r.post)
	mux.HandleFunc("DELETE /api/clip", r.delete)
	mux.HandleFunc("GET /api/clip/stream", r.stream)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		// Tailboard has native clients, not a browser UI. A website visited on
		// a tailnet device must not be able to read or mutate its clipboard.
		if len(request.Header.Values("Origin")) != 0 || len(request.Header.Values("Sec-Fetch-Site")) != 0 {
			writeError(w, http.StatusForbidden, "browser requests are not supported")
			return
		}
		mux.ServeHTTP(w, request)
	})
}

func (r *Relay) get(w http.ResponseWriter, _ *http.Request) {
	item := r.Current()
	if item == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (r *Relay) post(w http.ResponseWriter, request *http.Request) {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "use application/json")
		return
	}
	var payload struct {
		Content  string `json:"content"`
		DeviceID string `json:"device_id"`
	}
	// A byte of text can take up to six bytes when escaped in JSON.
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 6*MaxTextBytes+4096))
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid text payload")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid text payload")
		return
	}
	if payload.Content == "" || len(payload.Content) > MaxTextBytes {
		writeError(w, http.StatusBadRequest, "text must contain 1 to 1048576 bytes")
		return
	}
	if len(payload.DeviceID) > 128 || len(request.Header.Get("X-Clip-Source")) > 128 {
		writeError(w, http.StatusBadRequest, "device labels must not exceed 128 bytes")
		return
	}
	if r.applyRemote != nil {
		if err := r.applyRemote(payload.Content); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update the Mac clipboard")
			return
		}
	}
	source := strings.TrimSpace(request.Header.Get("X-Clip-Source"))
	if source == "" {
		source = "Android"
	}
	writeJSON(w, http.StatusCreated, r.put(payload.Content, source, payload.DeviceID))
}

func (r *Relay) delete(w http.ResponseWriter, _ *http.Request) {
	if r.clearRemote != nil {
		if err := r.clearRemote(); err != nil {
			writeError(w, http.StatusInternalServerError, "could not clear the Mac clipboard")
			return
		}
	}
	r.clear()
	w.WriteHeader(http.StatusNoContent)
}

func (r *Relay) status(w http.ResponseWriter, _ *http.Request) {
	r.mu.RLock()
	response := map[string]any{
		"ready": true, "seq": r.seq, "connected_devices": len(r.clients),
		"has_text": r.current != nil, "uptime_seconds": int(time.Since(r.started).Seconds()),
	}
	r.mu.RUnlock()
	writeJSON(w, http.StatusOK, response)
}

func (r *Relay) stream(w http.ResponseWriter, request *http.Request) {
	updates := make(chan message, 1)
	r.mu.Lock()
	if len(r.clients) >= maxStreams {
		r.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "too many connected devices")
		return
	}
	r.clients[updates] = struct{}{}
	current := r.current
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.clients, updates)
		r.mu.Unlock()
	}()
	conn, err := websocket.Accept(w, request, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	peerClosed := conn.CloseRead(ctx)
	if err := writeMessage(ctx, conn, message{Type: "ready"}); err != nil {
		return
	}
	if current != nil {
		item := *current
		if err := writeMessage(ctx, conn, message{Type: "clip_update", Item: &item}); err != nil {
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-peerClosed.Done():
			return
		case update := <-updates:
			if err := writeMessage(ctx, conn, update); err != nil {
				return
			}
		}
	}
}

func writeMessage(ctx context.Context, conn *websocket.Conn, update message) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, update)
}

func Listen(ctx context.Context, address string, r *Relay) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		IdleTimeout: time.Minute, MaxHeaderBytes: 8192,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
