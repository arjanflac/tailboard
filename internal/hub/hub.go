package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

// Config holds hub configuration.
type Config struct {
	MaxHistory                int
	MaxResidentHistoryBytes   int
	MaxPersistentHistoryItems int
	MaxPersistentHistoryBytes int64
	TTL                       time.Duration
	DBPath                    string // Empty = no persistence.
	SpoolDir                  string
	SpoolQuota                int64
	MaxTransferSize           int64
	TransferTTL               time.Duration
}

const (
	defaultResidentHistoryBytes   = 12 << 20
	defaultPersistentHistoryItems = 200
	defaultPersistentHistoryBytes = int64(64 << 20)
)

// Subscriber receives clipboard updates via a channel.
type Subscriber struct {
	C         chan protocol.ClipItem
	Transfers chan protocol.Transfer
	cancel    context.CancelFunc
}

// Hub is the central clipboard broker.
type Hub struct {
	mu                        sync.RWMutex
	current                   *protocol.ClipItem
	history                   []protocol.ClipItem
	maxHistory                int
	maxResidentHistoryBytes   int
	maxPersistentHistoryItems int
	maxPersistentHistoryBytes int64
	seq                       uint64
	ttl                       time.Duration
	startedAt                 time.Time
	store                     clipStore // nil if no persistence.

	// publishMu serializes post-commit side effects in sequence order so
	// persistence and subscriber delivery stay monotonic after h.mu is released.
	publishMu    sync.Mutex
	publishCond  *sync.Cond
	publishedSeq uint64

	subsMu sync.RWMutex
	subs   map[*Subscriber]struct{}

	devicesMu sync.RWMutex
	devices   map[string]protocol.Device
	transfers *transferStore

	stop      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type clipStore interface {
	Close() error
	LoadState(maxHistory int, maxBytes ...int) (uint64, []protocol.ClipItem, error)
	HistoryPage(limit int, beforeSeq uint64) ([]protocol.ClipItem, error)
	LoadItem(seq uint64) (*protocol.ClipItem, error)
	SaveItem(item protocol.ClipItem) (protocol.ClipItem, error)
	TrimHistory(maxItems int, maxBytes int64) (int, error)
	DeleteExpired(before time.Time) (int, error)
	DeleteAll() error
	LoadDevices() ([]protocol.Device, error)
	SaveDevice(protocol.Device) error
	DeleteDevice(deviceID string) error
}

// New creates a Hub, optionally backed by SQLite, and starts the TTL reaper.
func New(cfg Config) (*Hub, error) {
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 50
	}
	if cfg.MaxResidentHistoryBytes <= 0 {
		cfg.MaxResidentHistoryBytes = defaultResidentHistoryBytes
	}
	if cfg.MaxPersistentHistoryItems <= 0 {
		cfg.MaxPersistentHistoryItems = defaultPersistentHistoryItems
	}
	if cfg.MaxPersistentHistoryBytes <= 0 {
		cfg.MaxPersistentHistoryBytes = defaultPersistentHistoryBytes
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	h := &Hub{
		maxHistory:                cfg.MaxHistory,
		maxResidentHistoryBytes:   cfg.MaxResidentHistoryBytes,
		maxPersistentHistoryItems: cfg.MaxPersistentHistoryItems,
		maxPersistentHistoryBytes: cfg.MaxPersistentHistoryBytes,
		ttl:                       cfg.TTL,
		subs:                      make(map[*Subscriber]struct{}),
		devices:                   make(map[string]protocol.Device),
		startedAt:                 time.Now(),
		stop:                      make(chan struct{}),
	}
	transfers, err := newTransferStore(cfg.SpoolDir, cfg.SpoolQuota, cfg.MaxTransferSize, cfg.TransferTTL)
	if err != nil {
		return nil, fmt.Errorf("open transfer spool: %w", err)
	}
	h.transfers = transfers
	if expired, removed := transfers.reapExpired(time.Now()); len(expired) > 0 || removed > 0 {
		slog.Info("reaped transfers during startup", "component", "hub_transfers", "expired_transfers", len(expired), "removed_metadata", removed)
	}

	if cfg.DBPath != "" {
		st, err := OpenStore(cfg.DBPath)
		if err != nil {
			_ = transfers.close()
			return nil, fmt.Errorf("open clip store: %w", err)
		}
		h.store = st
		seq, items, err := st.LoadState(cfg.MaxHistory, cfg.MaxResidentHistoryBytes)
		if err != nil {
			st.Close()
			_ = transfers.close()
			return nil, fmt.Errorf("load clip state: %w", err)
		}
		h.seq = seq
		h.history = items
		if len(items) > 0 {
			h.current = &items[0]
		}
		devices, err := st.LoadDevices()
		if err != nil {
			st.Close()
			_ = transfers.close()
			return nil, fmt.Errorf("load devices: %w", err)
		}
		for _, device := range devices {
			device.Online = false
			h.devices[device.DeviceID] = device
		}
		slog.Info("loaded state from db", "component", "hub_store", "sequence", seq, "history_items", len(items))
	}

	h.publishCond = sync.NewCond(&h.publishMu)
	h.publishedSeq = h.seq

	go h.reapLoop()
	return h, nil
}

func (h *Hub) RegisterDevice(req protocol.RegisterDeviceRequest) protocol.Device {
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	device := h.devices[req.DeviceID]
	device.DeviceID = req.DeviceID
	device.Name = req.Name
	device.Platform = req.Platform
	device.Capabilities = mergeStrings(device.Capabilities, req.Capabilities)
	device.PublicKey = req.PublicKey
	device.LastSeen = time.Now()
	h.devices[req.DeviceID] = device
	if h.store != nil {
		if err := h.store.SaveDevice(device); err != nil {
			slog.Error("failed to persist device", "component", "hub_store", "device_id", device.DeviceID, "error", err)
		}
	}
	return device
}

func mergeStrings(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]string, 0, len(existing)+len(incoming))
	for _, values := range [][]string{existing, incoming} {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	sort.Strings(merged)
	return merged
}

// RemoveDevice deletes a device from the roster and the persistent store.
// Returns false when the device is unknown.
func (h *Hub) RemoveDevice(deviceID string) bool {
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	if _, ok := h.devices[deviceID]; !ok {
		return false
	}
	delete(h.devices, deviceID)
	if h.store != nil {
		if err := h.store.DeleteDevice(deviceID); err != nil {
			slog.Error("failed to delete persisted device", "component", "hub_store", "device_id", deviceID, "error", err)
		}
	}
	return true
}

// staleDeviceTTL is how long an offline device stays on the roster after
// it was last seen. Devices that never come back (reinstalls, test
// registrations, retired hardware) age out instead of accumulating.
const staleDeviceTTL = 30 * 24 * time.Hour

func (h *Hub) reapStaleDevices(now time.Time) {
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	for id, device := range h.devices {
		if device.Online || now.Sub(device.LastSeen) < staleDeviceTTL {
			continue
		}
		delete(h.devices, id)
		if h.store != nil {
			if err := h.store.DeleteDevice(id); err != nil {
				slog.Error("failed to delete stale device", "component", "hub_store", "device_id", id, "error", err)
			}
		}
		slog.Info("reaped stale device", "component", "hub_devices", "device_id", id, "name", device.Name, "last_seen", device.LastSeen)
	}
}

func (h *Hub) SetDeviceOnline(deviceID string, online bool) {
	if deviceID == "" {
		return
	}
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	device, ok := h.devices[deviceID]
	if !ok {
		device = protocol.Device{DeviceID: deviceID, Name: deviceID}
	}
	device.Online = online
	device.LastSeen = time.Now()
	h.devices[deviceID] = device
}

func (h *Hub) Devices() []protocol.Device {
	h.devicesMu.RLock()
	defer h.devicesMu.RUnlock()
	devices := make([]protocol.Device, 0, len(h.devices))
	for _, device := range h.devices {
		device.Capabilities = append([]string(nil), device.Capabilities...)
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices
}

// Close shuts down the hub's persistent store.
func (h *Hub) Close() error {
	h.closeOnce.Do(func() {
		close(h.stop)
		var transferErr, storeErr error
		if h.transfers != nil {
			transferErr = h.transfers.close()
		}
		if h.store != nil {
			storeErr = h.store.Close()
		}
		h.closeErr = errors.Join(transferErr, storeErr)
	})
	return h.closeErr
}

// PutInput describes a new clipboard item to store.
type PutInput struct {
	MimeType string
	Content  string // For text/* types.
	Data     []byte // For binary types.
	Source   string
	DeviceID string
}

// Put stores a new clipboard item. Returns the item and true if it was new,
// or the existing current item and false if it was a duplicate.
func (h *Hub) Put(in PutInput) (protocol.ClipItem, bool) {
	var hash string
	if strings.HasPrefix(in.MimeType, "text/") {
		hash = protocol.HashContent(in.Content)
	} else {
		hash = protocol.HashBytes(in.Data)
	}

	h.mu.Lock()
	if h.current != nil && h.current.Hash == hash && h.current.MimeType == in.MimeType {
		item := cloneClipItem(*h.current)
		h.mu.Unlock()
		return item, false
	}

	h.seq++
	now := time.Now()
	item := protocol.ClipItem{
		Seq:       h.seq,
		MimeType:  in.MimeType,
		Content:   in.Content,
		Data:      cloneBytes(in.Data),
		Hash:      hash,
		Source:    in.Source,
		DeviceID:  in.DeviceID,
		CreatedAt: now,
		ExpiresAt: now.Add(h.ttl),
	}

	h.current = &item
	h.history = append([]protocol.ClipItem{item}, h.history...)
	h.trimResidentHistoryLocked()
	h.mu.Unlock()
	h.publish(item)
	return cloneClipItem(item), true
}

// Get returns the current clipboard item, or nil.
func (h *Hub) Get() *protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneClipItemPtr(h.current)
}

// History returns up to limit items from history.
func (h *Hub) History(limit int) []protocol.ClipItem {
	if limit <= 0 || limit > h.maxHistory {
		limit = h.maxHistory
	}
	if h.store != nil {
		if items, err := h.store.HistoryPage(limit, 0); err == nil {
			return items
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit > len(h.history) {
		limit = len(h.history)
	}
	return cloneClipItems(h.history[:limit])
}

// HistoryPage returns a cursor-addressable page of history items, newest-first.
// When a persistent store is available, paging can extend beyond the in-memory history window.
func (h *Hub) HistoryPage(limit int, beforeSeq uint64) ([]protocol.ClipItem, string, bool, error) {
	if limit <= 0 {
		limit = protocol.DefaultHistoryLimit
	}

	pageLimit := limit + 1
	var items []protocol.ClipItem
	var err error

	if h.store != nil {
		items, err = h.store.HistoryPage(pageLimit, beforeSeq)
		if err != nil {
			return nil, "", false, err
		}
	} else {
		items = h.historyPageFromMemory(pageLimit, beforeSeq)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = strconv.FormatUint(items[len(items)-1].Seq, 10)
	}
	return cloneClipItems(items), nextCursor, hasMore, nil
}

// Clear removes the current clip and persisted history while preserving the
// sequence counter for future writes.
func (h *Hub) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.store != nil {
		if err := h.store.DeleteAll(); err != nil {
			return err
		}
	}

	h.current = nil
	h.history = nil
	return nil
}

// Since returns all items in history with seq > afterSeq, in chronological order.
func (h *Hub) Since(afterSeq uint64) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var out []protocol.ClipItem
	for i := len(h.history) - 1; i >= 0; i-- {
		if h.history[i].Seq > afterSeq {
			out = append(out, cloneClipItem(h.history[i]))
		}
	}
	return out
}

// GetBySeq returns a historical clip by sequence number, or nil when not found.
func (h *Hub) GetBySeq(seq uint64) (*protocol.ClipItem, error) {
	h.mu.RLock()
	if h.current != nil && h.current.Seq == seq {
		item := cloneClipItem(*h.current)
		h.mu.RUnlock()
		return &item, nil
	}
	for _, item := range h.history {
		if item.Seq == seq {
			cloned := cloneClipItem(item)
			h.mu.RUnlock()
			return &cloned, nil
		}
	}
	h.mu.RUnlock()

	if h.store == nil {
		return nil, nil
	}
	return h.store.LoadItem(seq)
}

// Subscribe creates a new subscriber. Cancel the context to unsubscribe.
func (h *Hub) Subscribe(ctx context.Context) *Subscriber {
	ctx, cancel := context.WithCancel(ctx)
	sub := &Subscriber{
		C:         make(chan protocol.ClipItem, 1),
		Transfers: make(chan protocol.Transfer, 16),
		cancel:    cancel,
	}

	h.subsMu.Lock()
	h.subs[sub] = struct{}{}
	h.subsMu.Unlock()

	go func() {
		<-ctx.Done()
		h.unsubscribe(sub)
	}()

	return sub
}

func (h *Hub) publishTransfer(transfer protocol.Transfer) {
	for _, sub := range h.snapshotSubscribers() {
		select {
		case sub.Transfers <- transfer:
		default:
		}
	}
}

func (h *Hub) unsubscribe(sub *Subscriber) {
	h.subsMu.Lock()
	delete(h.subs, sub)
	h.subsMu.Unlock()
}

// SubscriberCount returns the number of active subscribers.
func (h *Hub) SubscriberCount() int {
	h.subsMu.RLock()
	defer h.subsMu.RUnlock()
	return len(h.subs)
}

// Seq returns the current sequence number.
func (h *Hub) Seq() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.seq
}

// StartedAt returns when the hub was created.
func (h *Hub) StartedAt() time.Time {
	return h.startedAt
}

func (h *Hub) publish(item protocol.ClipItem) {
	if h.publishCond == nil {
		h.publishCond = sync.NewCond(&h.publishMu)
		h.publishedSeq = h.seq - 1
	}

	h.publishMu.Lock()
	for item.Seq != h.publishedSeq+1 {
		h.publishCond.Wait()
	}

	for _, sub := range h.snapshotSubscribers() {
		offerLatestClip(sub.C, item)
	}

	if h.store != nil {
		if _, err := h.store.SaveItem(item); err != nil {
			slog.Error("failed to persist clip", "component", "hub_store", "sequence", item.Seq, "error", err)
		} else if removed, err := h.store.TrimHistory(h.maxPersistentHistoryItems, h.maxPersistentHistoryBytes); err != nil {
			slog.Error("failed to trim persisted history", "component", "hub_store", "error", err)
		} else if removed > 0 {
			slog.Info("trimmed persisted history", "component", "hub_store", "removed_items", removed)
		}
	}

	h.publishedSeq = item.Seq
	h.publishCond.Broadcast()
	h.publishMu.Unlock()
}

// offerLatestClip makes each subscriber queue represent clipboard state rather
// than a backlog. ClipItems are immutable after publication, so sharing the
// payload slice is safe and avoids one full binary copy per connected device.
func offerLatestClip(ch chan protocol.ClipItem, item protocol.ClipItem) {
	select {
	case ch <- item:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- item:
	default:
	}
}

func (h *Hub) snapshotSubscribers() []*Subscriber {
	h.subsMu.RLock()
	defer h.subsMu.RUnlock()

	subs := make([]*Subscriber, 0, len(h.subs))
	for sub := range h.subs {
		subs = append(subs, sub)
	}
	return subs
}

func cloneClipItemPtr(item *protocol.ClipItem) *protocol.ClipItem {
	if item == nil {
		return nil
	}
	cloned := cloneClipItem(*item)
	return &cloned
}

func cloneClipItems(items []protocol.ClipItem) []protocol.ClipItem {
	cloned := make([]protocol.ClipItem, len(items))
	for i := range items {
		cloned[i] = cloneClipItem(items[i])
	}
	return cloned
}

func cloneClipItem(item protocol.ClipItem) protocol.ClipItem {
	item.Data = cloneBytes(item.Data)
	return item
}

// trimResidentHistoryLocked keeps metadata/history useful without allowing a
// run of image clips to turn the always-on broker into a hundreds-of-megabytes
// cache. The current clipboard item is always retained; older entries share
// the remaining byte budget.
func (h *Hub) trimResidentHistoryLocked() {
	if len(h.history) == 0 {
		return
	}
	kept := h.history[:0]
	residentBytes := 0
	for index, item := range h.history {
		itemBytes := clipPayloadBytes(item)
		if index > 0 && (len(kept) >= h.maxHistory || residentBytes+itemBytes > h.maxResidentHistoryBytes) {
			continue
		}
		kept = append(kept, item)
		residentBytes += itemBytes
	}
	// The backing array can be larger than the retained slice. Clear discarded
	// entries so their binary payloads become collectible immediately.
	clear(h.history[len(kept):])
	h.history = kept
}

func (h *Hub) historyPageFromMemory(limit int, beforeSeq uint64) []protocol.ClipItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	start := 0
	if beforeSeq > 0 {
		start = len(h.history)
		for i, item := range h.history {
			if item.Seq < beforeSeq {
				start = i
				break
			}
		}
	}
	if start >= len(h.history) {
		return nil
	}

	end := start + limit
	if end > len(h.history) {
		end = len(h.history)
	}
	return cloneClipItems(h.history[start:end])
}

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

func (h *Hub) reapLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-ticker.C:
			h.reapExpired()
		}
	}
}

func (h *Hub) reapExpired() {
	now := time.Now()
	h.reapStaleDevices(now)
	if h.transfers != nil {
		expired, removed := h.transfers.reapExpired(now)
		for _, transfer := range expired {
			h.publishTransfer(transfer)
		}
		if len(expired) > 0 || removed > 0 {
			slog.Info("reaped expired transfers", "component", "hub_transfers", "expired_transfers", len(expired), "removed_metadata", removed)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.current != nil && now.After(h.current.ExpiresAt) {
		h.current = nil
		slog.Info("current clip expired", "component", "hub_ttl")
	}

	kept := h.history[:0]
	for _, item := range h.history {
		if !now.After(item.ExpiresAt) {
			kept = append(kept, item)
		}
	}
	if reaped := len(h.history) - len(kept); reaped > 0 {
		slog.Info("reaped expired clips", "component", "hub_ttl", "expired_items", reaped)
	}
	h.history = kept

	if h.store != nil {
		go func() {
			if n, err := h.store.DeleteExpired(now); err != nil {
				slog.Error("failed to delete expired from db", "component", "hub_store", "error", err)
			} else if n > 0 {
				slog.Info("reaped expired clips from db", "component", "hub_store", "expired_items", n)
			}
		}()
	}
}
