package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/arjanflac/tailboard/internal/clipboard"
	"github.com/arjanflac/tailboard/internal/hubclient"
	"github.com/arjanflac/tailboard/internal/privacy"
	"github.com/arjanflac/tailboard/internal/protocol"
)

// Config holds agent configuration.
type Config struct {
	HubURL          string              // Base URL of the hub.
	Client          *hubclient.Client   // Shared hub client (preferred when set).
	NodeName        string              // This node's name.
	DeviceID        string              // Stable install UUID.
	PollInterval    time.Duration       // Clipboard poll interval.
	Clipboard       clipboard.Clipboard // Clipboard backend (nil = system default).
	Privacy         privacy.Config      // Optional privacy policy for outbound clips.
	ContextProvider contextProvider     // Optional active app/process detector.
	ControlAddr     string              // Loopback address for the desktop control surface; empty disables it.
}

// Agent is the local clipboard sync agent.
type Agent struct {
	hubURL       string
	nodeName     string
	deviceID     string
	pollInterval time.Duration
	monitor      *ClipboardMonitor
	client       *hubclient.Client
	paused       atomic.Bool
	bootstrapped atomic.Bool
	privacy      privacy.Config
	ctxProvider  contextProvider
	warnedCtx    atomic.Bool
	controlAddr  string
}

// ClipboardInitError reports a failure to initialize the default clipboard backend.
type ClipboardInitError struct {
	Err error
}

func (e *ClipboardInitError) Error() string {
	if e == nil || e.Err == nil {
		return "clipboard init failed"
	}
	return e.Err.Error()
}

func (e *ClipboardInitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var newClipboard = clipboard.New

// New creates an Agent.
func New(cfg Config) (*Agent, error) {
	clip := cfg.Clipboard
	if clip == nil {
		c, err := newClipboard()
		if err != nil {
			return nil, &ClipboardInitError{Err: err}
		}
		clip = c
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	client := cfg.Client
	if client == nil && cfg.HubURL != "" {
		var err error
		client, err = hubclient.New(hubclient.Config{BaseURL: cfg.HubURL})
		if err != nil {
			return nil, err
		}
	}
	if client != nil {
		cfg.HubURL = client.BaseURL()
	}

	return &Agent{
		hubURL:       cfg.HubURL,
		nodeName:     cfg.NodeName,
		deviceID:     cfg.DeviceID,
		pollInterval: cfg.PollInterval,
		monitor:      NewClipboardMonitor(clip),
		client:       client,
		privacy:      cfg.Privacy,
		ctxProvider:  resolveContextProvider(cfg),
		controlAddr:  cfg.ControlAddr,
	}, nil
}

// Run starts the clipboard poll loop and WebSocket listener.
func (a *Agent) Run(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("hub client is not configured")
	}
	if a.controlAddr != "" {
		controlServer, err := a.startControlServer(ctx)
		if err != nil {
			return err
		}
		defer controlServer.Shutdown(context.Background())
		slog.Info("desktop control surface active", "component", "tg-clipd", "url", controlServer.URL)
	}

	ws := &WSClient{
		URL: a.client.StreamURLForDevice(a.deviceID),
		OnConnected: func() {
			if a.deviceID != "" {
				capabilities := []string{"clipboard"}
				if reporter, ok := a.ctxProvider.(contextProviderReporter); ok {
					capabilities = append(capabilities, "privacy-detector:"+reporter.Layer())
				}
				if _, err := a.client.RegisterDevice(ctx, protocol.RegisterDeviceRequest{
					DeviceID: a.deviceID, Name: a.nodeName, Platform: runtime.GOOS,
					Capabilities: capabilities, ReplaceCapabilities: true,
				}); err != nil {
					slog.Warn("device registration failed", "component", "tg-clipd", "error", err)
				}
			}
			if !a.bootstrapped.Load() {
				a.bootstrap(ctx)
			}
		},
		OnUpdate: func(item protocol.ClipItem) {
			a.applyRemote(item)
		},
	}

	go ws.Run(ctx)

	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()
	var pollEvents <-chan time.Time = ticker.C
	var watchEvents <-chan struct{}
	if watcher, ok := a.monitor.clip.(clipboard.Watcher); ok {
		events, err := watcher.Watch(ctx)
		if err != nil {
			slog.Warn("event-driven clipboard watch unavailable; using polling", "component", "tg-clipd", "error", err)
		} else {
			watchEvents = events
			pollEvents = nil
			slog.Info("event-driven clipboard watch active", "component", "tg-clipd")
		}
	}

	slog.Info("tg-clipd started", "component", "tg-clipd", "hub_url", a.hubURL, "node_name", a.nodeName, "poll_interval", a.pollInterval)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollEvents:
			a.pollClipboard(ctx)
		case _, ok := <-watchEvents:
			if !ok {
				watchEvents = nil
				pollEvents = ticker.C
				slog.Warn("clipboard watch stopped; reverting to polling", "component", "tg-clipd")
				continue
			}
			a.pollClipboard(ctx)
		}
	}
}

func (a *Agent) pollClipboard(ctx context.Context) {
	if a.isPaused() {
		return
	}
	if !a.bootstrapped.Load() {
		return
	}
	result, ct := a.monitor.Poll()
	if result == PollNewContent {
		if blocked := a.handlePrivacy(ct); blocked {
			return
		}
		if err := a.sendToHub(ctx, ct); err != nil {
			slog.Error("failed to send clip to hub, will retry", "component", "tg-clipd", "error", err)
		} else {
			a.monitor.MarkSent()
		}
	}
}

func (a *Agent) bootstrap(ctx context.Context) {
	backoff := 500 * time.Millisecond
	const maxRetries = 5

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ok := a.tryBootstrap(ctx)
		if ok {
			return
		}
		if ctx.Err() != nil {
			a.bootstrapped.Store(true)
			return
		}
		slog.Warn("bootstrap retry", "component", "tg-clipd_bootstrap", "attempt", attempt, "retry_delay", backoff)
		select {
		case <-ctx.Done():
			a.bootstrapped.Store(true)
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}

	slog.Error("bootstrap retries exhausted; proceeding without hub state", "component", "tg-clipd_bootstrap")
	a.bootstrapped.Store(true)
}

// tryBootstrap attempts a single bootstrap fetch. Returns true on success.
func (a *Agent) tryBootstrap(ctx context.Context) bool {
	item, err := a.client.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		slog.Info("bootstrap found no current clip", "component", "tg-clipd_bootstrap")
		a.bootstrapped.Store(true)
		return true
	}
	if err != nil {
		slog.Warn("bootstrap fetch failed", "component", "tg-clipd_bootstrap", "error", err)
		return false
	}

	a.applyRemote(*item)
	slog.Info("bootstrap applied hub clip", "component", "tg-clipd_bootstrap", "sequence", item.Seq, "source", item.Source, "mime_type", item.MimeType)
	a.bootstrapped.Store(true)
	return true
}

func (a *Agent) applyRemote(item protocol.ClipItem) {
	if a.isPaused() {
		return
	}
	if item.Source == a.nodeName {
		slog.Debug("ignoring own update", "component", "tg-clipd", "sequence", item.Seq)
		return
	}

	ct := itemToContent(item)
	if err := a.monitor.ApplyRemote(ct); err != nil {
		slog.Error("failed to apply remote clip", "component", "tg-clipd", "error", err)
	} else {
		slog.Info("applied remote clip", "component", "tg-clipd", "sequence", item.Seq, "source", item.Source, "mime_type", item.MimeType)
	}
}

func (a *Agent) isPaused() bool {
	return a.paused.Load() || a.isPausedByFile()
}

func (a *Agent) sendToHub(ctx context.Context, ct clipboard.Content) error {
	payload := hubclient.PutRequest{MimeType: ct.MimeType, Source: a.nodeName, DeviceID: a.deviceID}
	if ct.IsText() {
		payload.Content = ct.Text()
	} else {
		payload.Data = ct.Data
	}

	if _, err := a.client.Put(ctx, payload); err != nil {
		return err
	}

	slog.Info("sent clip to hub", "component", "tg-clipd", "mime_type", ct.MimeType, "payload_bytes", len(ct.Data))
	return nil
}

func (a *Agent) handlePrivacy(ct clipboard.Content) bool {
	if a.privacy.Empty() {
		return false
	}

	ctx := privacy.Context{}
	if a.ctxProvider != nil && a.privacy.UsesContext() {
		detected, err := a.ctxProvider.CurrentContext()
		if err != nil {
			if a.warnedCtx.CompareAndSwap(false, true) {
				slog.Warn("privacy context unavailable; app/process rules will be best-effort", "err", err)
			}
		} else {
			ctx = detected
		}
	}

	decision := a.privacy.Decide(ctx, ct)
	if !decision.Block {
		return false
	}

	if decision.ClearClipboard {
		if err := a.monitor.ClearLocal(); err != nil {
			a.monitor.MarkHandled()
			slog.Warn("privacy rule blocked clip but failed to clear local clipboard", "rule", decision.Rule, "matched", decision.Matched, "err", err)
		}
	} else {
		a.monitor.MarkHandled()
	}

	slog.Info("blocked local clipboard from sync", "rule", decision.Rule, "matched", decision.Matched, "mime", ct.MimeType)
	return true
}

func (a *Agent) isPausedByFile() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "tg-clipboard", "paused"))
	return err == nil
}

func itemToContent(item protocol.ClipItem) clipboard.Content {
	if item.IsText() {
		return clipboard.Content{MimeType: item.MimeType, Data: []byte(item.Content)}
	}
	return clipboard.Content{MimeType: item.MimeType, Data: item.Data}
}

func resolveContextProvider(cfg Config) contextProvider {
	if cfg.ContextProvider != nil {
		return cfg.ContextProvider
	}
	if cfg.Privacy.UsesContext() {
		return newContextProvider()
	}
	return noopContextProvider{}
}
