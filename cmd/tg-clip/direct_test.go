package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectTransferServerRequiresTokenAndSupportsRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := startDirectTransferServer(context.Background(), "127.0.0.1:0", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	resp, err := http.Get(server.URL + "/files/0")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", resp.StatusCode)
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/files/0", nil)
	request.Header.Set("Authorization", "Bearer "+server.Token)
	request.Header.Set("Range", "bytes=3-6")
	resp, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != "3456" {
		t.Fatalf("range status=%d body=%q", resp.StatusCode, body)
	}
}
