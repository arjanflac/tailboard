package hub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestTransferIsOfferedOnlyAfterEveryFileIsVerified(t *testing.T) {
	store, err := newTransferStore(t.TempDir(), 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	first := []byte("one")
	second := []byte("two")
	firstHash := sha256.Sum256(first)
	secondHash := sha256.Sum256(second)
	transfer, err := store.create("sender", protocol.CreateTransferRequest{
		ToDevice: "receiver",
		Files: []protocol.TransferFile{
			{Name: "one.txt", Size: 3, SHA256: hex.EncodeToString(firstHash[:])},
			{Name: "two.txt", Size: 3, SHA256: hex.EncodeToString(secondHash[:])},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transfer.State != "uploading" {
		t.Fatalf("create state = %s", transfer.State)
	}
	transfer, err = store.upload(transfer.TransferID, 0, 0, bytes.NewReader(first))
	if err != nil || transfer.State != "uploading" {
		t.Fatalf("first upload state=%s err=%v", transfer.State, err)
	}
	transfer, err = store.upload(transfer.TransferID, 1, 0, bytes.NewReader(second))
	if err != nil || transfer.State != "offered" {
		t.Fatalf("second upload state=%s err=%v", transfer.State, err)
	}
}

func TestDirectTransferIsOfferedWithoutUsingSpoolQuota(t *testing.T) {
	store, err := newTransferStore(t.TempDir(), 1, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	transfer, err := store.create("sender", protocol.CreateTransferRequest{
		ToDevice:    "receiver",
		Mode:        "direct",
		DirectURL:   "http://100.64.0.1:12345",
		DirectToken: strings.Repeat("a", 64),
		Files: []protocol.TransferFile{{
			Name: "large.bin", Size: 1024, SHA256: strings.Repeat("b", 64),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transfer.State != "offered" || transfer.Mode != "direct" || transfer.Files[0].Uploaded != transfer.Files[0].Size {
		t.Fatalf("unexpected direct transfer: %+v", transfer)
	}
	if _, _, err := store.openFile(transfer.TransferID, 0); !errors.Is(err, ErrTransferConflict) {
		t.Fatalf("direct transfer unexpectedly opened from spool: %v", err)
	}
}

func TestTransferExpiryNotifiesThenRemovesTerminalMetadata(t *testing.T) {
	store, err := newTransferStore(t.TempDir(), 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	transfer, err := store.create("sender", protocol.CreateTransferRequest{
		ToDevice: "receiver",
		Files: []protocol.TransferFile{{
			Name: "pending.txt", Size: 1, SHA256: strings.Repeat("a", 64),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expired, removed := store.reapExpired(transfer.ExpiresAt.Add(time.Minute))
	if len(expired) != 1 || expired[0].State != "expired" || removed != 0 {
		t.Fatalf("first reap expired=%+v removed=%d", expired, removed)
	}
	finishedAt := expired[0].FinishedAt
	if finishedAt == nil {
		t.Fatal("expired transfer did not record terminal time")
	}
	expired, removed = store.reapExpired(finishedAt.Add(25 * time.Hour))
	if len(expired) != 0 || removed != 1 {
		t.Fatalf("cleanup reap expired=%+v removed=%d", expired, removed)
	}
	if _, err := store.get(transfer.TransferID); !errors.Is(err, ErrTransferNotFound) {
		t.Fatalf("terminal transfer metadata remains: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM transfers WHERE transfer_id = ?", transfer.TransferID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("terminal SQLite rows=%d err=%v", count, err)
	}
}

func TestTransferUploadResumesFromPersistedOffset(t *testing.T) {
	store, err := newTransferStore(t.TempDir(), 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	payload := []byte("resume-me")
	sum := sha256.Sum256(payload)
	transfer, err := store.create("sender", protocol.CreateTransferRequest{
		ToDevice: "receiver",
		Files: []protocol.TransferFile{{
			Name: "resume.txt", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err = store.upload(transfer.TransferID, 0, 0, bytes.NewReader(payload[:4]))
	if err != nil || transfer.Files[0].Uploaded != 4 || transfer.State != "uploading" {
		t.Fatalf("partial upload = %+v, %v", transfer, err)
	}
	transfer, err = store.upload(transfer.TransferID, 0, 4, bytes.NewReader(payload[4:]))
	if err != nil || transfer.Files[0].Uploaded != int64(len(payload)) || transfer.State != "offered" {
		t.Fatalf("resumed upload = %+v, %v", transfer, err)
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

func TestTransferSidecarMigratesToSQLite(t *testing.T) {
	dir := t.TempDir()
	transfer := protocol.Transfer{
		TransferID: "legacy-transfer", FromDevice: "sender", ToDevice: "receiver",
		Files: []protocol.TransferFile{{Name: "file.txt", Size: 1, SHA256: strings.Repeat("a", 64)}},
		State: "offered", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	transferDir := filepath.Join(dir, transfer.TransferID)
	if err := os.MkdirAll(transferDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(transfer)
	sidecar := filepath.Join(transferDir, "metadata.json")
	if err := os.WriteFile(sidecar, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newTransferStore(dir, 1<<20, 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	if _, err := store.get(transfer.TransferID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("legacy sidecar was not removed: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM transfers WHERE transfer_id = ?", transfer.TransferID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("SQLite migration count=%d err=%v", count, err)
	}
}
