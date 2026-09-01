package hub

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/arjanflac/tailboard/internal/protocol"
)

// Config holds hub configuration.
type Config struct {
	DBPath string // Empty = no persistence.
}

const (
	// ClipItem retains the historical expires_at protocol field as a hint for
	// mobile caches. The relay itself is last-value state and does not expire its
	// single current value.
	mobileHistoryTTL   = 24 * time.Hour
	deviceReapInterval = 6 * time.Hour
)

// Subscriber receives clipboard updates via a channel.
type Subscriber struct {
	C      chan protocol.ClipItem
	cancel context.CancelFunc
}

// Hub is the central clipboard broker.
type Hub struct {
	mu        sync.RWMutex
	current   *protocol.ClipItem
	seq       uint64
	startedAt time.Time
	store     clipStore // nil if no persistence.

	// publishMu serializes post-commit side effects in sequence order so
	// persistence and subscriber delivery stay monotonic after h.mu is released.
	publishMu    sync.Mutex
	publishCond  *sync.Cond
	publishedSeq uint64

	subsMu sync.RWMutex
	subs   map[*Subscriber]struct{}

	devicesMu sync.RWMutex
	devices   map[string]protocol.Device
	// deviceConnections counts live WebSockets per device. Scene transitions
	// can briefly overlap an old and a new mobile connection; a close from the
	// old socket must not mark the replacement offline.
	deviceConnections map[string]int

	stop      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type clipStore interface {
	Close() error
	LoadState() (uint64, *protocol.ClipItem, error)
	ReplaceCurrent(item protocol.ClipItem) error
	DeleteAll() error
	LoadDevices() ([]protocol.Device, error)
	SaveDevice(protocol.Device) error
	DeleteDevice(deviceID string) error
}

// New creates a last-value relay, optionally backed by SQLite.
func New(cfg Config) (*Hub, error) {
	h := &Hub{
		subs:              make(map[*Subscriber]struct{}),
		devices:           make(map[string]protocol.Device),
		deviceConnections: make(map[string]int),
		startedAt:         time.Now(),
		stop:              make(chan struct{}),
	}
	if cfg.DBPath != "" {
		st, err := OpenStore(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("open clip store: %w", err)
		}
		h.store = st
		seq, current, err := st.LoadState()
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("load clip state: %w", err)
		}
		h.seq = seq
		h.current = current
		devices, err := st.LoadDevices()
		if err != nil {
			st.Close()
			return nil, fmt.Errorf("load devices: %w", err)
		}
		for _, device := range devices {
			device.Online = false
			h.devices[device.DeviceID] = device
		}
		slog.Info("loaded relay state from db", "component", "hub_store", "sequence", seq, "has_current", current != nil)
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
	if req.ReplaceCapabilities {
		device.Capabilities = mergeStrings(nil, req.Capabilities)
	} else {
		device.Capabilities = mergeStrings(device.Capabilities, req.Capabilities)
	}
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
	delete(h.deviceConnections, deviceID)
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

func (h *Hub) DeviceConnected(deviceID string) {
	if deviceID == "" {
		return
	}
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	if h.devices == nil {
		h.devices = make(map[string]protocol.Device)
	}
	if h.deviceConnections == nil {
		h.deviceConnections = make(map[string]int)
	}
	h.deviceConnections[deviceID]++
	device, ok := h.devices[deviceID]
	if !ok {
		device = protocol.Device{DeviceID: deviceID, Name: deviceID}
	}
	device.Online = true
	device.LastSeen = time.Now()
	h.devices[deviceID] = device
}

func (h *Hub) DeviceDisconnected(deviceID string) {
	if deviceID == "" {
		return
	}
	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()
	connections := h.deviceConnections[deviceID]
	if connections > 1 {
		h.deviceConnections[deviceID] = connections - 1
		return
	}
	delete(h.deviceConnections, deviceID)
	device, ok := h.devices[deviceID]
	if !ok {
		return
	}
	device.Online = false
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
		if h.store != nil {
			h.closeErr = h.store.Close()
		}
	})
	return h.closeErr
}

// PutInput describes a new clipboard item to store.
type PutInput struct {
	Content  string
	Source   string
	DeviceID string
}

// Put stores a new clipboard item. Returns the item and true if it was new,
// or the existing current item and false if it was a duplicate.
func (h *Hub) Put(in PutInput) (protocol.ClipItem, bool) {
	hash := protocol.HashContent(in.Content)

	h.mu.Lock()
	if h.current != nil && h.current.Hash == hash {
		item := cloneClipItem(*h.current)
		h.mu.Unlock()
		return item, false
	}

	h.seq++
	now := time.Now()
	item := protocol.ClipItem{
		Seq:       h.seq,
		Content:   in.Content,
		Hash:      hash,
		Source:    in.Source,
		DeviceID:  in.DeviceID,
		CreatedAt: now,
		ExpiresAt: now.Add(mobileHistoryTTL),
	}

	h.current = &item
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

// Clear removes the current clip while preserving the
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
	return nil
}

// Subscribe creates a new subscriber. Cancel the context to unsubscribe.
func (h *Hub) Subscribe(ctx context.Context) *Subscriber {
	ctx, cancel := context.WithCancel(ctx)
	sub := &Subscriber{
		C:      make(chan protocol.ClipItem, 1),
		cancel: cancel,
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
		if err := h.store.ReplaceCurrent(item); err != nil {
			slog.Error("failed to persist current clip", "component", "hub_store", "sequence", item.Seq, "error", err)
		}
	}

	h.publishedSeq = item.Seq
	h.publishCond.Broadcast()
	h.publishMu.Unlock()
}

// offerLatestClip makes each subscriber queue represent clipboard state rather
// than a backlog. ClipItems are immutable after publication and safe to share.
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

func cloneClipItem(item protocol.ClipItem) protocol.ClipItem {
	return item
}

func (h *Hub) reapLoop() {
	ticker := time.NewTicker(deviceReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.stop:
			return
		case now := <-ticker.C:
			h.reapStaleDevices(now)
		}
	}
}
