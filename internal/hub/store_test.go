package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

func testClip(seq uint64, content string, createdAt time.Time) protocol.ClipItem {
	return protocol.ClipItem{
		Seq:       seq,
		Content:   content,
		Hash:      protocol.HashContent(content),
		Source:    "node1",
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(24 * time.Hour),
	}
}

func TestStoreRecoversFromInterruptedLegacyWrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := openSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE clips (
			seq INTEGER PRIMARY KEY, mime_type TEXT NOT NULL, content TEXT,
			data BLOB, hash TEXT NOT NULL, source TEXT NOT NULL,
			created_at TEXT NOT NULL, expires_at TEXT NOT NULL
		);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(
		"INSERT INTO clips (seq, mime_type, content, data, hash, source, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		7, "text/plain", "persisted clip", nil, protocol.HashContent("persisted clip"), "node1",
		now.Format(time.RFC3339Nano), now.Add(time.Hour).Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seq, current, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 7 || current == nil || current.Seq != 7 || current.Content != "persisted clip" {
		t.Fatalf("unexpected recovered state: seq=%d current=%+v", seq, current)
	}
}

func TestDeviceRegistryPersistsCapabilities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	device := protocol.Device{
		DeviceID: "device-1", Name: "MacBook", Platform: "darwin",
		Capabilities: []string{"clipboard", "privacy-detector:appkit"},
		LastSeen:     time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := store.SaveDevice(device); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices, err := store.LoadDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].DeviceID != device.DeviceID || len(devices[0].Capabilities) != 2 {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}

func TestReplaceCurrentKeepsOneRow(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.ReplaceCurrent(testClip(1, "first", now)); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceCurrent(testClip(2, "second", now.Add(time.Second))); err != nil {
		t.Fatal(err)
	}

	seq, current, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 2 || current == nil || current.Seq != 2 || current.Content != "second" {
		t.Fatalf("unexpected relay state: seq=%d current=%+v", seq, current)
	}
	var rows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM clips").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("expected one persisted clip, got %d", rows)
	}
}

func TestOpenStoreCompactsExistingHistoryToLatest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old-history.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for seq := 1; seq <= 400; seq++ {
		content := strings.Repeat(string(rune('a'+seq%26)), 8<<10)
		item := testClip(uint64(seq), content, now.Add(time.Duration(seq)*time.Second))
		_, err := store.db.Exec(
			"INSERT OR REPLACE INTO clips (seq, content, hash, source, device_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			item.Seq, item.Content, item.Hash, item.Source, item.DeviceID,
			item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	store.Close()

	store, err = OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seq, current, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 400 || current == nil || current.Seq != 400 {
		t.Fatalf("unexpected compacted state: seq=%d current=%+v", seq, current)
	}
	var rows int
	_ = store.db.QueryRow("SELECT COUNT(*) FROM clips").Scan(&rows)
	if rows != 1 {
		t.Fatalf("expected startup compaction to leave one row, got %d", rows)
	}
	var pageCount, freePages int
	_ = store.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	_ = store.db.QueryRow("PRAGMA freelist_count").Scan(&freePages)
	if freePages > 128 || pageCount > 32 {
		t.Fatalf("expected compact database, got page_count=%d freelist_count=%d", pageCount, freePages)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 256<<10 {
		t.Fatalf("expected compact file on disk, got %d bytes", info.Size())
	}
}

func TestDeleteAllPreservesSequence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "relay.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.ReplaceCurrent(testClip(1, "first", now)); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store, err = OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seq, current, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if seq != 1 || current != nil {
		t.Fatalf("expected empty state at sequence 1, got seq=%d current=%+v", seq, current)
	}
}

func TestHubPersistsCurrentAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "relay.db")
	hub, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	hub.Put(PutInput{Content: "survive restart", Source: "node"})
	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.Seq() != 1 {
		t.Fatalf("expected seq 1 after restart, got %d", restarted.Seq())
	}
	current := restarted.Get()
	if current == nil || current.Content != "survive restart" {
		t.Fatalf("current clip not recovered: %+v", current)
	}
}
