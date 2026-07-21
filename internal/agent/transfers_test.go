package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thalysguimaraes/tg-clipboard/internal/hubclient"
	"github.com/thalysguimaraes/tg-clipboard/internal/protocol"
)

func TestTransferHelpers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := transferFileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("hash = %q", hash)
	}
	summary := transferSummary(protocol.Transfer{
		FromDevice: "phone",
		Files:      []protocol.TransferFile{{Size: 2}, {Size: 3}},
	})
	if summary != "2 file(s), 5 bytes from phone" {
		t.Fatalf("summary = %q", summary)
	}
}

func TestDownloadTransferFileDirectFetch(t *testing.T) {
	payload := []byte("direct from sender")
	token := "direct-test-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client, err := hubclient.New(hubclient.Config{BaseURL: "http://hub.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	downloadDir := t.TempDir()
	agent := &Agent{client: client, deviceID: "receiver", downloadDir: downloadDir}
	sum := sha256.Sum256(payload)
	transfer := protocol.Transfer{
		TransferID: "direct-id", Mode: "direct", DirectURL: server.URL, DirectToken: token,
	}
	manifest := protocol.TransferFile{
		Name: "direct.txt", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]),
	}
	if err := agent.downloadTransferFile(context.Background(), transfer, 0, manifest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(downloadDir, "direct.txt"))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("downloaded payload=%q err=%v", got, err)
	}
}

func TestRejectsInvalidTransferPolicy(t *testing.T) {
	_, err := New(Config{Clipboard: &fakeClipboard{}, TransferPolicy: "always"})
	if err == nil {
		t.Fatal("expected invalid policy error")
	}
}
