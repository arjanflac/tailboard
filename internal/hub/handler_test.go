package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func devIdentity(r *http.Request) string { return "test-node" }

func setupServer(t *testing.T) (*Hub, *httptest.Server) {
	h, _, srv := setupServerWithObserver(t)
	return h, srv
}

func TestCapabilitiesAndDeviceID(t *testing.T) {
	h, _ := New(Config{})
	mux := http.NewServeMux()
	Register(mux, h, func(*http.Request) string { return "test-node" })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var capabilities protocol.Capabilities
	if err := json.NewDecoder(resp.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if capabilities.ProtocolVersion != protocol.ProtocolVersion || !capabilities.Features["devices"] {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/clip", bytes.NewBufferString(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clip-Device-ID", "2bd59f07-f7c4-4cc9-98a9-c52af6e56642")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()
	var item protocol.ClipItem
	if err := json.NewDecoder(putResp.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if item.DeviceID != "2bd59f07-f7c4-4cc9-98a9-c52af6e56642" {
		t.Fatalf("device id = %q", item.DeviceID)
	}
}

func setupServerWithObserver(t *testing.T) (*Hub, *Observer, *httptest.Server) {
	t.Helper()
	h := newTestHub()
	obs := NewObserver()
	mux := http.NewServeMux()
	Register(mux, h, devIdentity, obs)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return h, obs, srv
}

func TestPostAndGetClip(t *testing.T) {
	_, srv := setupServer(t)

	body := `{"content":"hello world"}`
	resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var item protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&item)
	resp.Body.Close()

	if item.Seq != 1 || item.Content != "hello world" {
		t.Fatal("unexpected item", item)
	}

	resp, err = http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var got protocol.ClipItem
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()

	if got.Content != "hello world" {
		t.Fatal("GET returned wrong content")
	}
}

func TestGetClipEmpty(t *testing.T) {
	_, srv := setupServer(t)

	resp, err := http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestPostClipDedup(t *testing.T) {
	_, srv := setupServer(t)

	body := `{"content":"same"}`
	http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))

	resp, _ := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dedup should return 200, got %d", resp.StatusCode)
	}
}

func TestPostClipEmptyContent(t *testing.T) {
	_, srv := setupServer(t)

	resp, _ := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(`{"content":""}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty content, got %d", resp.StatusCode)
	}

	var apiErr protocol.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if apiErr.Error.Code != "empty_content" {
		t.Fatalf("expected typed empty_content error, got %+v", apiErr.Error)
	}
}

func TestHistoryEndpointIsRemoved(t *testing.T) {
	_, srv := setupServer(t)
	resp, err := http.Get(srv.URL + "/api/clip/history")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected removed history endpoint to return 404, got %d", resp.StatusCode)
	}
}

func TestClearClip(t *testing.T) {
	_, srv := setupServer(t)

	for _, content := range []string{"first", "second"} {
		body, _ := json.Marshal(map[string]string{"content": content})
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBuffer(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/clip", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected empty current clip after clear, got %d", resp.StatusCode)
	}

}

func TestStatus(t *testing.T) {
	_, srv := setupServer(t)

	resp, _ := http.Get(srv.URL + "/api/status")
	var status map[string]any
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()

	if _, ok := status["uptime"]; !ok {
		t.Fatal("missing uptime")
	}
	if _, ok := status["ready"]; !ok {
		t.Fatal("missing ready")
	}
}

func TestWebSocketStream(t *testing.T) {
	_, srv := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/clip/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	assertWebSocketReady(t, ctx, conn)

	body := `{"content":"ws test"}`
	http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))

	var msg protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "clip_update" || msg.Item.Content != "ws test" {
		t.Fatalf("unexpected ws message: %+v", msg)
	}
}

func TestWebSocketStreamReplaySinceSeq(t *testing.T) {
	_, srv := setupServer(t)

	post := func(body string) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	post(`{"content":"first"}`)
	post(`{"content":"second"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL+"/api/clip/stream?since_seq=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	assertWebSocketReady(t, ctx, conn)

	var replay protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &replay); err != nil {
		t.Fatal(err)
	}
	if replay.Type != "clip_update" || replay.Item == nil || replay.Item.Seq != 2 || replay.Item.Content != "second" {
		t.Fatalf("unexpected replay message: %+v", replay)
	}

	post(`{"content":"third"}`)

	var live protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &live); err != nil {
		t.Fatal(err)
	}
	if live.Type != "clip_update" || live.Item == nil || live.Item.Seq != 3 || live.Item.Content != "third" {
		t.Fatalf("unexpected live message after replay: %+v", live)
	}
}

func TestHealthReadinessAndMetrics(t *testing.T) {
	_, obs, srv := setupServerWithObserver(t)

	post := func(body string) {
		t.Helper()
		resp, err := http.Post(srv.URL+"/api/clip", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	healthResp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected healthz 200, got %d", healthResp.StatusCode)
	}
	if requestID := healthResp.Header.Get("X-Request-ID"); requestID == "" {
		t.Fatal("expected X-Request-ID header on healthz")
	}
	var health map[string]any
	if err := json.NewDecoder(healthResp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	healthResp.Body.Close()
	if ready, ok := health["ready"].(bool); !ok || !ready {
		t.Fatalf("expected health ready=true, got %#v", health["ready"])
	}

	readyResp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	if readyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected readyz 200, got %d", readyResp.StatusCode)
	}
	readyResp.Body.Close()

	post(`{"content":"operable"}`)
	post(`{"content":"operable"}`)

	metricsResp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	metricsResp.Body.Close()

	metrics := string(body)
	for _, expected := range []string{
		"tg-clipboard_ready 1",
		"tg-clipboard_clips_stored_total 1",
		"tg-clipboard_clips_deduplicated_total 1",
		"tg-clipboard_http_requests_total",
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %q, got %s", expected, metrics)
		}
	}

	obs.BeginShutdown()
	notReadyResp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	if notReadyResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected readyz 503 after shutdown, got %d", notReadyResp.StatusCode)
	}
	notReadyResp.Body.Close()
}

func TestLifecycleShutdownClosesStreams(t *testing.T) {
	h := newTestHub()
	obs := NewObserver()
	mux := http.NewServeMux()
	Register(mux, h, devIdentity, obs)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(ln)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	baseURL := "http://" + ln.Addr().String()
	wsURL := "ws://" + ln.Addr().String() + "/api/clip/stream"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	assertWebSocketReady(t, ctx, conn)

	obs.BeginShutdown()

	resp, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected readyz 503 after shutdown, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	readErr := make(chan error, 1)
	go func() {
		var msg protocol.WSMessage
		readErr <- wsjson.Read(ctx, conn, &msg)
	}()

	select {
	case err := <-readErr:
		if err == nil {
			t.Fatal("expected websocket read to fail after shutdown")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket shutdown")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}

	if err := <-serveDone; err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve returned unexpected error %v", err)
	}
}

func assertWebSocketReady(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()
	var ready protocol.WSMessage
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatalf("read websocket ready message: %v", err)
	}
	if ready.Type != "ready" {
		t.Fatalf("expected websocket ready message, got %+v", ready)
	}
}
