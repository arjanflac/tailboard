package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arjanflac/tailboard/internal/hub"
	"github.com/arjanflac/tailboard/internal/hubclient"
	"github.com/arjanflac/tailboard/internal/protocol"
)

func TestControlServerStatePauseAndDropSend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	broker, err := hub.New(hub.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = broker.Close() })
	mux := http.NewServeMux()
	hub.Register(mux, broker, func(r *http.Request) string {
		if id := r.Header.Get("X-Clip-Device-ID"); id != "" {
			return id
		}
		return "test"
	})
	hubServer := httptest.NewServer(mux)
	defer hubServer.Close()
	client, err := hubclient.New(hubclient.Config{BaseURL: hubServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RegisterDevice(context.Background(), protocol.RegisterDeviceRequest{
		DeviceID: "receiver", Name: "Desktop", Platform: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := New(Config{
		Client:      client,
		DeviceID:    "sender",
		NodeName:    "Laptop",
		Clipboard:   &fakeClipboard{},
		ControlAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := agent.startControlServer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	resp, err := http.Get(server.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var state controlState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(state.Devices) != 1 || state.Devices[0].DeviceID != "receiver" {
		t.Fatalf("unexpected devices: %+v", state.Devices)
	}

	pauseBody := bytes.NewBufferString(`{"paused":true}`)
	resp, err = http.Post(server.URL+"/api/pause", "application/json", pauseBody)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !agent.paused.Load() {
		t.Fatal("pause endpoint did not pause the agent")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("files", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("hello control surface"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/send?to=receiver", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		t.Fatalf("send returned %d: %v", resp.StatusCode, payload)
	}
	var transfer protocol.Transfer
	if err := json.NewDecoder(resp.Body).Decode(&transfer); err != nil {
		t.Fatal(err)
	}
	if transfer.State != "offered" || len(transfer.Files) != 1 || transfer.Files[0].Name != "hello.txt" {
		t.Fatalf("unexpected transfer: %+v", transfer)
	}
}

func TestControlServerRejectsNonLoopback(t *testing.T) {
	agent := &Agent{controlAddr: "0.0.0.0:9438"}
	if _, err := agent.startControlServer(context.Background()); err == nil {
		t.Fatal("expected non-loopback control address rejection")
	}
}
