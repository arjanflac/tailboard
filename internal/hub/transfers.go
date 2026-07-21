package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/protocol"
)

var (
	ErrTransferNotFound = errors.New("transfer not found")
	ErrTransferConflict = errors.New("transfer state conflict")
	ErrSpoolQuota       = errors.New("spool quota exceeded")
)

type transferStore struct {
	mu        sync.Mutex
	dir       string
	quota     int64
	maxSize   int64
	ttl       time.Duration
	transfers map[string]protocol.Transfer
	temporary bool
	db        *sql.DB
}

func newTransferStore(dir string, quota, maxSize int64, ttl time.Duration) (*transferStore, error) {
	temporary := false
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "cliphub-spool-*")
		if err != nil {
			return nil, err
		}
		temporary = true
	}
	if quota <= 0 {
		quota = 10 << 30
	}
	if maxSize <= 0 {
		maxSize = protocol.DefaultMaxTransferSize
	}
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "metadata.db"))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS transfers (
			transfer_id TEXT PRIMARY KEY,
			metadata    TEXT NOT NULL,
			state       TEXT NOT NULL,
			expires_at  TEXT NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, err
	}
	s := &transferStore{dir: dir, quota: quota, maxSize: maxSize, ttl: ttl, transfers: map[string]protocol.Transfer{}, temporary: temporary, db: db}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *transferStore) close() error {
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return err
		}
	}
	if s.temporary {
		return os.RemoveAll(s.dir)
	}
	return nil
}

func (s *transferStore) create(from string, req protocol.CreateTransferRequest) (protocol.Transfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if from == "" || req.ToDevice == "" || len(req.Files) == 0 {
		return protocol.Transfer{}, fmt.Errorf("sender, receiver, and files are required")
	}
	mode := req.Mode
	if mode == "" {
		mode = "spool"
	}
	if mode != "spool" && mode != "direct" {
		return protocol.Transfer{}, fmt.Errorf("invalid transfer mode")
	}
	if mode == "direct" {
		directURL, err := url.ParseRequestURI(req.DirectURL)
		if err != nil || directURL.Host == "" || (directURL.Scheme != "http" && directURL.Scheme != "https") {
			return protocol.Transfer{}, fmt.Errorf("invalid direct transfer URL")
		}
		if len(req.DirectToken) < 32 {
			return protocol.Transfer{}, fmt.Errorf("direct transfer token is too short")
		}
	}
	var total int64
	for i := range req.Files {
		req.Files[i].Name = safeFilename(req.Files[i].Name)
		req.Files[i].Uploaded = 0
		if req.Files[i].Name == "" || req.Files[i].Size < 0 || len(req.Files[i].SHA256) != 64 {
			return protocol.Transfer{}, fmt.Errorf("invalid file manifest")
		}
		total += req.Files[i].Size
		if mode == "direct" {
			req.Files[i].Uploaded = req.Files[i].Size
		}
	}
	if total > s.maxSize {
		return protocol.Transfer{}, fmt.Errorf("transfer exceeds maximum size")
	}
	if mode == "spool" {
		used, err := s.usedLocked()
		if err != nil {
			return protocol.Transfer{}, err
		}
		if used+total > s.quota {
			return protocol.Transfer{}, ErrSpoolQuota
		}
	}
	id, err := randomID()
	if err != nil {
		return protocol.Transfer{}, err
	}
	now := time.Now()
	state := "uploading"
	if mode == "direct" {
		state = "offered"
	}
	transfer := protocol.Transfer{
		TransferID: id, FromDevice: from, ToDevice: req.ToDevice,
		Files: req.Files, Note: req.Note, Mode: mode, DirectURL: req.DirectURL,
		DirectToken: req.DirectToken, State: state, CreatedAt: now, ExpiresAt: now.Add(s.ttl),
	}
	if mode == "spool" {
		if err := os.MkdirAll(s.transferDir(id), 0o700); err != nil {
			return protocol.Transfer{}, err
		}
	}
	s.transfers[id] = transfer
	if err := s.persistLocked(transfer); err != nil {
		delete(s.transfers, id)
		return protocol.Transfer{}, err
	}
	return transfer, nil
}

func (s *transferStore) list(deviceID, role, state string) []protocol.Transfer {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Non-nil so an empty roster serializes as [] — clients decode a JSON
	// null as a hard error, and this endpoint is part of every app refresh.
	out := make([]protocol.Transfer, 0, len(s.transfers))
	for _, transfer := range s.transfers {
		if state != "" && transfer.State != state {
			continue
		}
		if role == "receiver" && transfer.ToDevice != deviceID {
			continue
		}
		if role == "sender" && transfer.FromDevice != deviceID {
			continue
		}
		if role == "" && deviceID != "" && transfer.ToDevice != deviceID && transfer.FromDevice != deviceID {
			continue
		}
		out = append(out, transfer)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *transferStore) get(id string) (protocol.Transfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transfer, ok := s.transfers[id]
	if !ok {
		return protocol.Transfer{}, ErrTransferNotFound
	}
	return transfer, nil
}

func (s *transferStore) transition(id, actor, action string) (protocol.Transfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transfer, ok := s.transfers[id]
	if !ok {
		return protocol.Transfer{}, ErrTransferNotFound
	}
	switch action {
	case "accept":
		if actor != transfer.ToDevice || transfer.State != "offered" {
			return protocol.Transfer{}, ErrTransferConflict
		}
		transfer.State = "accepted"
	case "decline":
		if actor != transfer.ToDevice || transfer.State != "offered" {
			return protocol.Transfer{}, ErrTransferConflict
		}
		transfer.State = "declined"
	case "complete":
		if actor != transfer.ToDevice || (transfer.State != "accepted" && transfer.State != "transferring") {
			return protocol.Transfer{}, ErrTransferConflict
		}
		for _, file := range transfer.Files {
			if file.Uploaded != file.Size {
				return protocol.Transfer{}, ErrTransferConflict
			}
		}
		transfer.State = "complete"
	case "cancel":
		if actor != transfer.FromDevice && actor != transfer.ToDevice {
			return protocol.Transfer{}, ErrTransferConflict
		}
		transfer.State = "canceled"
	default:
		return protocol.Transfer{}, ErrTransferConflict
	}
	if action == "complete" || action == "decline" || action == "cancel" {
		now := time.Now()
		transfer.FinishedAt = &now
	}
	s.transfers[id] = transfer
	if err := s.persistLocked(transfer); err != nil {
		return protocol.Transfer{}, err
	}
	if action == "complete" || action == "decline" || action == "cancel" {
		_ = s.deletePayloadsLocked(id)
	}
	return transfer, nil
}

func (s *transferStore) upload(id string, index int, start int64, src io.Reader) (protocol.Transfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transfer, ok := s.transfers[id]
	if !ok {
		return protocol.Transfer{}, ErrTransferNotFound
	}
	if index < 0 || index >= len(transfer.Files) || transfer.State != "uploading" {
		return protocol.Transfer{}, ErrTransferConflict
	}
	file := &transfer.Files[index]
	if start != file.Uploaded {
		return protocol.Transfer{}, ErrTransferConflict
	}
	path := s.filePath(id, index)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return protocol.Transfer{}, err
	}
	defer out.Close()
	if _, err := out.Seek(start, io.SeekStart); err != nil {
		return protocol.Transfer{}, err
	}
	written, err := io.Copy(out, io.LimitReader(src, file.Size-start+1))
	if err != nil {
		return protocol.Transfer{}, err
	}
	if start+written > file.Size {
		_ = os.Truncate(path, start)
		return protocol.Transfer{}, fmt.Errorf("upload exceeds declared size")
	}
	file.Uploaded = start + written
	allUploaded := true
	for _, candidate := range transfer.Files {
		if candidate.Uploaded != candidate.Size {
			allUploaded = false
			break
		}
	}
	if allUploaded {
		transfer.State = "offered"
	}
	if file.Uploaded == file.Size {
		if err := verifyFile(path, file.SHA256); err != nil {
			_ = os.Remove(path)
			file.Uploaded = 0
			return protocol.Transfer{}, err
		}
	}
	s.transfers[id] = transfer
	if err := s.persistLocked(transfer); err != nil {
		return protocol.Transfer{}, err
	}
	return transfer, nil
}

func (s *transferStore) openFile(id string, index int) (*os.File, protocol.TransferFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transfer, ok := s.transfers[id]
	if !ok || index < 0 || index >= len(transfer.Files) {
		return nil, protocol.TransferFile{}, ErrTransferNotFound
	}
	if transfer.Mode == "direct" {
		return nil, protocol.TransferFile{}, ErrTransferConflict
	}
	file := transfer.Files[index]
	if file.Uploaded != file.Size {
		return nil, protocol.TransferFile{}, ErrTransferConflict
	}
	if transfer.State == "accepted" {
		transfer.State = "transferring"
		s.transfers[id] = transfer
		if err := s.persistLocked(transfer); err != nil {
			return nil, protocol.TransferFile{}, err
		}
	}
	handle, err := os.Open(s.filePath(id, index))
	return handle, file, err
}

func (s *transferStore) cancel(id, actor string) (protocol.Transfer, error) {
	return s.transition(id, actor, "cancel")
}

func (s *transferStore) reapExpired(now time.Time) ([]protocol.Transfer, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []protocol.Transfer
	removed := 0
	for id, transfer := range s.transfers {
		if !now.After(transfer.ExpiresAt) {
			continue
		}
		if transfer.State != "complete" && transfer.State != "declined" && transfer.State != "canceled" && transfer.State != "expired" {
			transfer.State = "expired"
			finishedAt := now
			transfer.FinishedAt = &finishedAt
			s.transfers[id] = transfer
			_ = s.persistLocked(transfer)
			_ = s.deletePayloadsLocked(id)
			expired = append(expired, transfer)
			continue
		}
		if transfer.FinishedAt == nil {
			finishedAt := now
			transfer.FinishedAt = &finishedAt
			s.transfers[id] = transfer
			_ = s.persistLocked(transfer)
			continue
		}
		if now.Sub(*transfer.FinishedAt) >= 24*time.Hour {
			delete(s.transfers, id)
			_ = s.deletePayloadsLocked(id)
			if s.db != nil {
				_, _ = s.db.Exec("DELETE FROM transfers WHERE transfer_id = ?", id)
			}
			removed++
		}
	}
	return expired, removed
}

func (s *transferStore) load() error {
	rows, err := s.db.Query("SELECT metadata FROM transfers")
	if err != nil {
		return err
	}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			rows.Close()
			return err
		}
		var transfer protocol.Transfer
		if json.Unmarshal([]byte(data), &transfer) == nil && transfer.TransferID != "" {
			s.transfers[transfer.TransferID] = transfer
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Migrate spools created before transfer metadata moved into SQLite.
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var transfer protocol.Transfer
		if json.Unmarshal(data, &transfer) == nil && transfer.TransferID != "" {
			s.transfers[transfer.TransferID] = transfer
			if err := s.persistLocked(transfer); err != nil {
				return err
			}
			_ = os.Remove(filepath.Join(s.dir, entry.Name(), "metadata.json"))
		}
	}
	return nil
}

func (s *transferStore) persistLocked(transfer protocol.Transfer) error {
	data, err := json.Marshal(transfer)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO transfers (transfer_id, metadata, state, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(transfer_id) DO UPDATE SET
			metadata=excluded.metadata, state=excluded.state,
			expires_at=excluded.expires_at
	`, transfer.TransferID, string(data), transfer.State, transfer.ExpiresAt.Format(time.RFC3339Nano))
	return err
}

func (s *transferStore) usedLocked() (int64, error) {
	var used int64
	for _, transfer := range s.transfers {
		switch transfer.State {
		case "complete", "declined", "canceled", "expired":
			continue
		}
		for _, file := range transfer.Files {
			used += file.Size
		}
	}
	return used, nil
}

func (s *transferStore) deletePayloadsLocked(id string) error {
	files, _ := filepath.Glob(filepath.Join(s.transferDir(id), "*.part"))
	for _, file := range files {
		_ = os.Remove(file)
	}
	return nil
}

func (s *transferStore) transferDir(id string) string { return filepath.Join(s.dir, id) }
func (s *transferStore) filePath(id string, index int) string {
	return filepath.Join(s.transferDir(id), fmt.Sprintf("%06d.part", index))
}

func safeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == ".." {
		return ""
	}
	return name
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func verifyFile(path, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), want) {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}
