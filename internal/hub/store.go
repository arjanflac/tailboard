package hub

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

// Store provides durable persistence for hub state.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates a SQLite database at path.
func OpenStore(path string) (*Store, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// openSQLite keeps the always-on hub's database footprint deliberately small.
// A single connection is enough for Tailboard's serialized, low-volume writes,
// and avoids multiplying SQLite page caches. Incremental auto-vacuum prevents
// expired clipboard blobs from leaving a permanently large database behind.
func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		"PRAGMA busy_timeout=5000",
		"PRAGMA temp_store=FILE",
		"PRAGMA cache_size=-2048",
		"PRAGMA journal_size_limit=8388608",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
	}

	var autoVacuum int
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&autoVacuum); err != nil {
		db.Close()
		return nil, fmt.Errorf("inspect sqlite auto-vacuum: %w", err)
	}
	if autoVacuum == 0 {
		if _, err := db.Exec("PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable sqlite auto-vacuum: %w", err)
		}
		if _, err := db.Exec("VACUUM"); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize sqlite auto-vacuum: %w", err)
		}
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite durability: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	var schema string
	err = tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'clips'").Scan(&schema)
	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(clipsTableDDL); err != nil {
			return fmt.Errorf("create clips table: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect clips schema: %w", err)
	case !strings.Contains(strings.ToUpper(schema), "AUTOINCREMENT"):
		if err := migrateLegacyClipsTable(tx); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DROP TABLE IF EXISTS meta"); err != nil {
		return fmt.Errorf("drop legacy meta table: %w", err)
	}
	if !strings.Contains(strings.ToLower(schema), "device_id") {
		if _, err := tx.Exec("ALTER TABLE clips ADD COLUMN device_id TEXT NOT NULL DEFAULT ''"); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("add device_id column: %w", err)
		}
	}
	if _, err := tx.Exec(devicesTableDDL); err != nil {
		return fmt.Errorf("create devices table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (s *Store) LoadDevices() ([]protocol.Device, error) {
	rows, err := s.db.Query("SELECT device_id, name, platform, capabilities, last_seen FROM devices ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []protocol.Device
	for rows.Next() {
		var device protocol.Device
		var capabilities, lastSeen string
		if err := rows.Scan(&device.DeviceID, &device.Name, &device.Platform, &capabilities, &lastSeen); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(capabilities), &device.Capabilities)
		device.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Store) SaveDevice(device protocol.Device) error {
	capabilities, err := json.Marshal(device.Capabilities)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO devices (device_id, name, platform, capabilities, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			name=excluded.name, platform=excluded.platform,
			capabilities=excluded.capabilities,
			last_seen=excluded.last_seen
	`, device.DeviceID, device.Name, device.Platform, string(capabilities), device.LastSeen.Format(time.RFC3339Nano))
	return err
}

// DeleteDevice removes a device row; deleting an unknown ID is a no-op.
func (s *Store) DeleteDevice(deviceID string) error {
	_, err := s.db.Exec(`DELETE FROM devices WHERE device_id = ?`, deviceID)
	return err
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// LoadState restores the seq counter and up to maxHistory items from the DB.
// Items are returned newest-first.
func (s *Store) LoadState(maxHistory int, maxBytes ...int) (uint64, []protocol.ClipItem, error) {
	seq, err := s.loadSeq()
	if err != nil {
		return 0, nil, err
	}

	rows, err := s.db.Query(selectClipColumns+" ORDER BY seq DESC LIMIT ?", maxHistory)
	if err != nil {
		return seq, nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()

	residentLimit := 0
	if len(maxBytes) > 0 {
		residentLimit = maxBytes[0]
	}
	var items []protocol.ClipItem
	residentBytes := 0
	for rows.Next() {
		item, err := scanClip(rows)
		if err != nil {
			return seq, nil, fmt.Errorf("scan clip: %w", err)
		}
		itemBytes := clipPayloadBytes(item)
		if len(items) > 0 && residentLimit > 0 && residentBytes+itemBytes > residentLimit {
			continue
		}
		items = append(items, item)
		residentBytes += itemBytes
	}
	return seq, items, rows.Err()
}

// HistoryPage returns up to limit items newer than the optional beforeSeq cursor.
// Items are returned newest-first.
func (s *Store) HistoryPage(limit int, beforeSeq uint64) ([]protocol.ClipItem, error) {
	query := selectClipColumns
	args := make([]any, 0, 2)
	if beforeSeq > 0 {
		query += " WHERE seq < ?"
		args = append(args, beforeSeq)
	}
	query += " ORDER BY seq DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load history page: %w", err)
	}
	defer rows.Close()

	var items []protocol.ClipItem
	for rows.Next() {
		item, err := scanClip(rows)
		if err != nil {
			return nil, fmt.Errorf("scan clip: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// LoadItem fetches a single clip by sequence number.
func (s *Store) LoadItem(seq uint64) (*protocol.ClipItem, error) {
	row := s.db.QueryRow(selectClipColumns+" WHERE seq = ?", seq)

	item, err := scanClip(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load clip %d: %w", seq, err)
	}
	return &item, nil
}

// SaveItem persists a clip item and returns it with seq assigned.
func (s *Store) SaveItem(item protocol.ClipItem) (protocol.ClipItem, error) {
	if item.Seq == 0 {
		result, err := s.db.Exec(
			"INSERT INTO clips (mime_type, content, data, hash, source, device_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			item.MimeType, item.Content, item.Data, item.Hash, item.Source, item.DeviceID,
			item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return item, err
		}
		seq, err := result.LastInsertId()
		if err != nil {
			return item, fmt.Errorf("read inserted seq: %w", err)
		}
		item.Seq = uint64(seq)
		return item, nil
	}

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO clips (seq, mime_type, content, data, hash, source, device_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.Seq, item.MimeType, item.Content, item.Data, item.Hash, item.Source, item.DeviceID,
		item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
	)
	return item, err
}

// TrimHistory bounds durable history by both count and payload bytes. The most
// recent item is always kept even when a single clipboard payload exceeds the
// byte budget.
func (s *Store) TrimHistory(maxItems int, maxBytes int64) (int, error) {
	if maxItems <= 0 || maxBytes <= 0 {
		return 0, nil
	}
	result, err := s.db.Exec(`
		DELETE FROM clips
		WHERE seq IN (
			SELECT seq
			FROM (
				SELECT
					seq,
					ROW_NUMBER() OVER (ORDER BY seq DESC) AS item_number,
					SUM(COALESCE(length(content), 0) + COALESCE(length(data), 0))
						OVER (ORDER BY seq DESC) AS cumulative_bytes
				FROM clips
			)
			WHERE item_number > ? OR (item_number > 1 AND cumulative_bytes > ?)
		)
	`, maxItems, maxBytes)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		s.reclaimFreePages(256)
	}
	return int(n), nil
}

// DeleteExpired removes clips that have expired before the given time.
func (s *Store) DeleteExpired(before time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM clips WHERE expires_at < ?", before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		s.reclaimFreePages(256)
	}
	return int(n), nil
}

// DeleteAll removes all persisted clipboard history while preserving the
// AUTOINCREMENT sequence so future clips remain monotonic.
func (s *Store) DeleteAll() error {
	if _, err := s.db.Exec("DELETE FROM clips"); err != nil {
		return err
	}
	s.reclaimFreePages(2048)
	return nil
}

func (s *Store) reclaimFreePages(pages int) {
	_, _ = s.db.Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", pages))
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(PASSIVE)")
}

const clipsTableDDL = `
	CREATE TABLE clips (
		seq        INTEGER PRIMARY KEY AUTOINCREMENT,
		mime_type  TEXT    NOT NULL,
		content    TEXT,
		data       BLOB,
		hash       TEXT    NOT NULL,
		source     TEXT    NOT NULL,
		device_id  TEXT    NOT NULL DEFAULT '',
		created_at TEXT    NOT NULL,
		expires_at TEXT    NOT NULL
	);
`

const devicesTableDDL = `
	CREATE TABLE IF NOT EXISTS devices (
		device_id    TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		platform     TEXT NOT NULL,
		capabilities TEXT NOT NULL DEFAULT '[]',
		last_seen    TEXT NOT NULL
	);
`

func migrateLegacyClipsTable(tx *sql.Tx) error {
	if _, err := tx.Exec("ALTER TABLE clips RENAME TO clips_legacy"); err != nil {
		return fmt.Errorf("rename legacy clips table: %w", err)
	}
	if _, err := tx.Exec(clipsTableDDL); err != nil {
		return fmt.Errorf("create migrated clips table: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO clips (seq, mime_type, content, data, hash, source, device_id, created_at, expires_at)
		SELECT seq, mime_type, content, data, hash, source, '', created_at, expires_at
		FROM clips_legacy
		ORDER BY seq
	`); err != nil {
		return fmt.Errorf("copy legacy clips rows: %w", err)
	}
	if _, err := tx.Exec("DROP TABLE clips_legacy"); err != nil {
		return fmt.Errorf("drop legacy clips table: %w", err)
	}
	return nil
}

func (s *Store) loadSeq() (uint64, error) {
	var seq uint64
	row := s.db.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'clips'")
	if err := row.Scan(&seq); err != nil {
		if err != sql.ErrNoRows {
			return 0, fmt.Errorf("load seq from sqlite_sequence: %w", err)
		}

		row = s.db.QueryRow("SELECT COALESCE(MAX(seq), 0) FROM clips")
		if err := row.Scan(&seq); err != nil {
			return 0, fmt.Errorf("load seq from clips: %w", err)
		}
	}
	return seq, nil
}

const selectClipColumns = "SELECT seq, mime_type, content, data, hash, source, device_id, created_at, expires_at FROM clips"

type clipScanner interface {
	Scan(dest ...any) error
}

func scanClip(scanner clipScanner) (protocol.ClipItem, error) {
	var item protocol.ClipItem
	var content sql.NullString
	var data []byte
	var createdAt, expiresAt string

	if err := scanner.Scan(&item.Seq, &item.MimeType, &content, &data, &item.Hash, &item.Source, &item.DeviceID, &createdAt, &expiresAt); err != nil {
		return protocol.ClipItem{}, err
	}
	item.Content = content.String
	item.Data = data
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	return item, nil
}

func clipPayloadBytes(item protocol.ClipItem) int {
	return len(item.Content) + len(item.Data)
}
