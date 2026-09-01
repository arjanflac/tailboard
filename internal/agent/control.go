package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

type controlServer struct {
	URL  string
	http *http.Server
}

type controlState struct {
	DeviceID   string             `json:"device_id"`
	NodeName   string             `json:"node_name"`
	HubURL     string             `json:"hub_url"`
	Paused     bool               `json:"paused"`
	Connection string             `json:"connection"` // synced, offline, no-tailscale
	Clip       *protocol.ClipItem `json:"clip,omitempty"`
	ClipSize   int64              `json:"clip_size,omitempty"`
	Devices    []protocol.Device  `json:"devices"`
}

func (a *Agent) startControlServer(ctx context.Context) (*controlServer, error) {
	if err := validateLoopbackAddress(a.controlAddr); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", a.controlAddr)
	if err != nil {
		return nil, fmt.Errorf("listen for desktop control surface: %w", err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	server := &controlServer{URL: "http://127.0.0.1:" + port}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", a.controlStateHandler)
	mux.HandleFunc("POST /api/pause", a.controlPauseHandler)
	server.http = &http.Server{
		Handler:           sameOrigin(server.URL, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.http.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("desktop control server stopped", "component", "tg-clipd_control", "error", err)
		}
	}()
	return server, nil
}

func (s *controlServer) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// controlStateHandler always answers quickly, even when the hub is
// unreachable: hub calls run under a short timeout, and failures degrade
// to an empty roster plus a connection status instead of hanging the UI.
// Without this, a dead tailnet route makes /api/state block until the
// client gives up and misreports the engine itself as down.
func (a *Agent) controlStateHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	connection := "synced"
	devices, err := a.client.Devices(ctx)
	if err != nil {
		devices = nil
		if hasTailnetInterface() {
			connection = "offline"
		} else {
			connection = "no-tailscale"
		}
	}

	var clip *protocol.ClipItem
	var clipSize int64
	if connection == "synced" {
		// Best-effort: the popover clip card degrades gracefully without it.
		// Binary payloads are stripped — the popover shows a summary, not
		// the bytes, and state is polled frequently.
		if current, err := a.client.Current(ctx); err == nil && current != nil {
			clipSize = int64(len(current.RawBytes()))
			if len(current.Data) > 0 {
				stripped := *current
				stripped.Data = nil
				clip = &stripped
			} else {
				clip = current
			}
		}
	}

	writeControlJSON(w, http.StatusOK, controlState{
		DeviceID:   a.deviceID,
		NodeName:   a.nodeName,
		HubURL:     a.hubURL,
		Paused:     a.isPaused(),
		Connection: connection,
		Clip:       clip,
		ClipSize:   clipSize,
		Devices:    devices,
	})
}

// hasTailnetInterface reports whether a Tailscale (CGNAT 100.64/10)
// address is present on any local interface — the cheapest local signal
// that Tailscale is up without shelling out to the CLI.
func hasTailnetInterface() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.To4() == nil {
			continue
		}
		ip := ipNet.IP.To4()
		if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
			return true
		}
	}
	return false
}

func (a *Agent) controlPauseHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		controlError(w, err, http.StatusBadRequest)
		return
	}
	a.paused.Store(request.Paused)
	writeControlJSON(w, http.StatusOK, map[string]bool{"paused": request.Paused})
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid control address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("desktop control address must be loopback-only")
	}
	return nil
}

func sameOrigin(baseURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != baseURL {
			controlError(w, fmt.Errorf("cross-origin request rejected"), http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func controlError(w http.ResponseWriter, err error, status int) {
	writeControlJSON(w, status, map[string]string{"error": err.Error()})
}
