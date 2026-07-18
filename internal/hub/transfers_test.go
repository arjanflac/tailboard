package hub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

func TestTransferLifecycleWithRangeDownload(t *testing.T) {
	h, err := New(Config{SpoolDir: t.TempDir(), SpoolQuota: 1 << 20, MaxTransferSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	mux := http.NewServeMux()
	Register(mux, h, devIdentity)
	server := httptest.NewServer(mux)
	defer server.Close()

	payload := []byte("hello transfer")
	sum := sha256.Sum256(payload)
	create := protocol.CreateTransferRequest{
		ToDevice: "receiver",
		Files: []protocol.TransferFile{{
			Name: "../unsafe.txt", Size: int64(len(payload)), MIME: "text/plain",
			SHA256: hex.EncodeToString(sum[:]),
		}},
	}
	body, _ := json.Marshal(create)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/transfers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clip-Device-ID", "sender")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created protocol.CreateTransferResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || created.Transfer.Files[0].Name != "unsafe.txt" {
		t.Fatalf("unexpected create response: %d %+v", resp.StatusCode, created)
	}

	upload, _ := http.NewRequest(http.MethodPut, server.URL+created.UploadURLs[0], bytes.NewReader(payload))
	upload.Header.Set("X-Clip-Device-ID", "sender")
	upload.Header.Set("Content-Range", "bytes 0-13/14")
	uploadResp, err := http.DefaultClient.Do(upload)
	if err != nil {
		t.Fatal(err)
	}
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("upload status %d", uploadResp.StatusCode)
	}

	accept, _ := http.NewRequest(http.MethodPost, server.URL+"/api/transfers/"+created.Transfer.TransferID+"/accept", nil)
	accept.Header.Set("X-Clip-Device-ID", "receiver")
	acceptResp, _ := http.DefaultClient.Do(accept)
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("accept status %d", acceptResp.StatusCode)
	}

	download, _ := http.NewRequest(http.MethodGet, server.URL+created.UploadURLs[0], nil)
	download.Header.Set("X-Clip-Device-ID", "receiver")
	download.Header.Set("Range", "bytes=6-13")
	downloadResp, err := http.DefaultClient.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(downloadResp.Body)
	downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusPartialContent || string(got) != "transfer" {
		t.Fatalf("range download: status=%d body=%q", downloadResp.StatusCode, got)
	}

	complete, _ := http.NewRequest(http.MethodPost, server.URL+"/api/transfers/"+created.Transfer.TransferID+"/complete", nil)
	complete.Header.Set("X-Clip-Device-ID", "receiver")
	completeResp, _ := http.DefaultClient.Do(complete)
	defer completeResp.Body.Close()
	var finished protocol.Transfer
	_ = json.NewDecoder(completeResp.Body).Decode(&finished)
	if completeResp.StatusCode != http.StatusOK || finished.State != "complete" {
		t.Fatalf("complete: status=%d transfer=%+v", completeResp.StatusCode, finished)
	}
}

func TestTransferMetadataSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := newTransferStore(dir, 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(nil)
	created, err := store.create("sender", protocol.CreateTransferRequest{
		ToDevice: "receiver",
		Files:    []protocol.TransferFile{{Name: "empty.txt", Size: 0, MIME: "text/plain", SHA256: hex.EncodeToString(sum[:])}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := newTransferStore(dir, 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.get(created.TransferID)
	if err != nil || got.ToDevice != "receiver" {
		t.Fatalf("reloaded transfer = %+v, %v", got, err)
	}
}
