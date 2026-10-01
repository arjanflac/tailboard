package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestPostGetAndClear(t *testing.T) {
	var applied string
	cleared := false
	relay := New(func(text string) error { applied = text; return nil }, func() error {
		cleared = true
		return nil
	})
	server := httptest.NewServer(relay.Handler())
	defer server.Close()

	body := bytes.NewBufferString(`{"content":"hello","device_id":"phone"}`)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/clip", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Clip-Source", "Pixel")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated || applied != "hello" {
		t.Fatalf("post status=%d applied=%q", response.StatusCode, applied)
	}

	response, err = http.Get(server.URL + "/api/clip")
	if err != nil {
		t.Fatal(err)
	}
	var item Clip
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if item.Content != "hello" || item.Source != "Pixel" || item.DeviceID != "phone" {
		t.Fatalf("clip = %#v", item)
	}

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/api/clip", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || !cleared || relay.Current() != nil {
		t.Fatalf("clear status=%d cleared=%t current=%#v", response.StatusCode, cleared, relay.Current())
	}
}

func TestRejectsEmptyAndOversizedText(t *testing.T) {
	relay := New(nil, nil)
	for name, body := range map[string][]byte{
		"empty":     []byte(`{"content":""}`),
		"oversized": append([]byte(`{"content":"`), append(bytes.Repeat([]byte("x"), MaxTextBytes+1), []byte(`"}`)...)...),
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/clip", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			relay.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestDuplicateTextDoesNotAdvanceSequence(t *testing.T) {
	relay := New(nil, nil)
	first := relay.PutLocal("same", "Mac")
	second := relay.PutLocal("same", "Mac")
	if first.Seq != 1 || second.Seq != 1 {
		t.Fatalf("sequences = %d, %d", first.Seq, second.Seq)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatal("duplicate text must retain its receipt ID")
	}
}

func TestReconnectReplaysSameReceiptAndNewTextGetsNewReceipt(t *testing.T) {
	r := New(nil, nil)
	first := r.PutLocal("old text", "Mac")
	server := httptest.NewServer(r.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for range 3 {
		conn, _, err := websocket.Dial(ctx, server.URL+"/api/clip/stream", nil)
		if err != nil {
			t.Fatal(err)
		}
		var ready, update message
		if err := wsjson.Read(ctx, conn, &ready); err != nil {
			t.Fatal(err)
		}
		if err := wsjson.Read(ctx, conn, &update); err != nil {
			t.Fatal(err)
		}
		conn.CloseNow()
		if ready.Type != "ready" || update.Item == nil || update.Item.ID != first.ID {
			t.Fatal("reconnect changed the receipt ID")
		}
	}
	r.PutLocal("different text", "Mac")
	again := r.PutLocal("old text", "Mac")
	if again.ID == first.ID {
		t.Fatal("a new copy needs a new receipt, even with the same text")
	}
	restarted := New(nil, nil).PutLocal("old text", "Mac")
	if restarted.ID == first.ID {
		t.Fatal("receipt IDs must not collide after a Mac restart")
	}
}

func TestListenerShutdownDisconnectsStreams(t *testing.T) {
	// Reserve an ephemeral loopback port before starting the production listener.
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	reservation.Close()
	r := New(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Listen(ctx, address, r) }()
	testContext, finish := context.WithTimeout(context.Background(), 3*time.Second)
	defer finish()
	var conn *websocket.Conn
	for testContext.Err() == nil {
		conn, _, err = websocket.Dial(testContext, "http://"+address+"/api/clip/stream", nil)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var ready message
	if err := wsjson.Read(testContext, conn, &ready); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := wsjson.Read(testContext, conn, &ready); err == nil || testContext.Err() != nil {
		t.Fatalf("stream was not disconnected on shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-testContext.Done():
		t.Fatal("listener did not stop")
	}
}

func TestRejectsBrowserAndFormClipboardRequests(t *testing.T) {
	for _, tc := range []struct {
		name, method, origin, contentType string
		want                              int
	}{
		{"cross-origin POST", "POST", "https://example.org", "application/json", 403},
		{"opaque-origin POST", "POST", "null", "application/json", 403},
		{"cross-origin DELETE", "DELETE", "https://example.org", "", 403},
		{"cross-origin read", "GET", "https://example.org", "", 403},
		{"HTML form", "POST", "", "text/plain", 415},
		{"missing content type", "POST", "", "", 415},
		{"native JSON", "POST", "", "application/json; charset=utf-8", 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := 0
			r := New(func(string) error { mutations++; return nil }, func() error { mutations++; return nil })
			req := httptest.NewRequest(tc.method, "/api/clip", bytes.NewBufferString(`{"content":"synthetic text"}`))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			response := httptest.NewRecorder()
			r.Handler().ServeHTTP(response, req)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.want != 201 && mutations != 0 {
				t.Fatal("rejected request changed the clipboard")
			}
			if tc.want == 201 && mutations != 1 {
				t.Fatal("native request did not update clipboard")
			}
		})
	}
}
