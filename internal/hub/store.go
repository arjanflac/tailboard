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
	store := &Store{db: db}
	if err := store.retainLatestOnly(); err != nil {
		db.Close()
		return nil, fmt.Errorf("compact relay state: %w", err)
	}
	return store, nil
}

// openSQLite keeps the always-on hub's database footprint deliberately small.
// A single connection is enough for Tailboard's serialized, low-volume writes,
// and avoids multiplying SQLite page caches. Incremental auto-vacuum prevents
// expired clipboard history from leaving a permanently large database behind.
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
		schema = clipsTableDDL
	case err != nil:
		return fmt.Errorf("inspect clips schema: %w", err)
	case !strings.Contains(strings.ToUpper(schema), "AUTOINCREMENT") ||
		strings.Contains(strings.ToLower(schema), "mime_type") ||
		strings.Contains(strings.ToLower(schema), " data "):
		if err := migrateLegacyClipsTable(tx, strings.Contains(strings.ToLower(schema), "device_id")); err != nil {
			return err
		}
		schema = clipsTableDDL
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

// LoadState restores the monotonic sequence and the relay's single current
// value. Older rows are removed during OpenStore migration.
func (s *Store) LoadState() (uint64, *protocol.ClipItem, error) {
	seq, err := s.loadSeq()
	if err != nil {
		return 0, nil, err
	}

	row := s.db.QueryRow(selectClipColumns + " ORDER BY seq DESC LIMIT 1")
	item, err := scanClip(row)
	if err == sql.ErrNoRows {
		return seq, nil, nil
	}
	if err != nil {
		return seq, nil, fmt.Errorf("load current clip: %w", err)
	}
	return seq, &item, nil
}

// ReplaceCurrent atomically stores one value and removes its predecessor.
func (s *Store) ReplaceCurrent(item protocol.ClipItem) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO clips (seq, content, hash, source, device_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		item.Seq, item.Content, item.Hash, item.Source, item.DeviceID,
		item.CreatedAt.Format(time.RFC3339Nano), item.ExpiresAt.Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM clips WHERE seq <> ?", item.Seq); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAll removes the persisted current value while preserving the
// AUTOINCREMENT sequence so future clips remain monotonic.
func (s *Store) DeleteAll() error {
	if _, err := s.db.Exec("DELETE FROM clips"); err != nil {
		return err
	}
	s.reclaimFreePages()
	return nil
}

func (s *Store) retainLatestOnly() error {
	_, err := s.db.Exec("DELETE FROM clips WHERE seq <> (SELECT MAX(seq) FROM clips)")
	if err != nil {
		return err
	}
	// First checkpoint any migration deletes out of WAL, then reclaim a legacy
	// history database when most of its pages are now empty. This runs only at
	// startup and only for a materially bloated file; ordinary one-row updates
	// never pay the cost of VACUUM.
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	var pageCount, freePages int
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return err
	}
	if err := s.db.QueryRow("PRAGMA freelist_count").Scan(&freePages); err != nil {
		return err
	}
	if freePages > 128 && freePages*2 > pageCount {
		if _, err := s.db.Exec("VACUUM"); err != nil {
			return err
		}
		_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
	return nil
}

func (s *Store) reclaimFreePages() {
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	_, _ = s.db.Exec("PRAGMA incremental_vacuum")
}

const clipsTableDDL = `
	CREATE TABLE clips (
		seq        INTEGER PRIMARY KEY AUTOINCREMENT,
		content    TEXT    NOT NULL,
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

func migrateLegacyClipsTable(tx *sql.Tx, hasDeviceID bool) error {
	if _, err := tx.Exec("ALTER TABLE clips RENAME TO clips_legacy"); err != nil {
		return fmt.Errorf("rename legacy clips table: %w", err)
	}
	if _, err := tx.Exec(clipsTableDDL); err != nil {
		return fmt.Errorf("create migrated clips table: %w", err)
	}
	deviceID := "''"
	if hasDeviceID {
		deviceID = "device_id"
	}
	query := `
		INSERT INTO clips (seq, content, hash, source, device_id, created_at, expires_at)
		SELECT seq, content, hash, source, ` + deviceID + `, created_at, expires_at
		FROM clips_legacy
		WHERE content IS NOT NULL AND content <> ''
		ORDER BY seq
	`
	if _, err := tx.Exec(query); err != nil {
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

const selectClipColumns = "SELECT seq, content, hash, source, device_id, created_at, expires_at FROM clips"

type clipScanner interface {
	Scan(dest ...any) error
}

func scanClip(scanner clipScanner) (protocol.ClipItem, error) {
	var item protocol.ClipItem
	var createdAt, expiresAt string

	if err := scanner.Scan(&item.Seq, &item.Content, &item.Hash, &item.Source, &item.DeviceID, &createdAt, &expiresAt); err != nil {
		return protocol.ClipItem{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	return item, nil
}
