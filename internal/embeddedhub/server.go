package embeddedhub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thalysguimaraes/tg-clipboard/internal/hub"
)

const DefaultAddress = ":9437"

type Config struct {
	Address         string
	StateDir        string
	MaxHistory      int
	TTL             time.Duration
	SpoolQuota      int64
	MaxTransferSize int64
	TransferTTL     time.Duration
}

// Server is an in-process hub carried by a tg-clipd instance.
type Server struct {
	URL      string
	listener net.Listener
	http     *http.Server
	hub      *hub.Hub
	close    sync.Once
	closeErr error
}

// Start begins an embedded hub on the host's normal network stack. Binding the
// default ":9437" address makes it reachable through the host's Tailscale IP.
func Start(ctx context.Context, cfg Config) (*Server, error) {
	if cfg.Address == "" {
		cfg.Address = DefaultAddress
	}
	if cfg.StateDir == "" {
		return nil, fmt.Errorf("embedded hub state directory is required")
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create embedded hub state directory: %w", err)
	}

	h, err := hub.New(hub.Config{
		MaxHistory:      cfg.MaxHistory,
		TTL:             cfg.TTL,
		DBPath:          filepath.Join(cfg.StateDir, "clips.db"),
		SpoolDir:        filepath.Join(cfg.StateDir, "spool"),
		SpoolQuota:      cfg.SpoolQuota,
		MaxTransferSize: cfg.MaxTransferSize,
		TransferTTL:     cfg.TransferTTL,
	})
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("listen for embedded hub: %w", err)
	}

	mux := http.NewServeMux()
	observer := hub.NewObserver()
	hub.Register(mux, h, func(r *http.Request) string {
		if source := r.Header.Get("X-Clip-Source"); source != "" {
			return source
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil && host != "" {
			return host
		}
		return "embedded"
	}, observer)

	server := &Server{
		URL:      localURL(listener.Addr()),
		listener: listener,
		http: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		hub: h,
	}
	go func() {
		_ = server.http.Serve(listener)
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	return server, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.close.Do(func() {
		httpErr := s.http.Shutdown(ctx)
		hubErr := s.hub.Close()
		s.closeErr = errors.Join(httpErr, hubErr)
	})
	return s.closeErr
}

func localURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
