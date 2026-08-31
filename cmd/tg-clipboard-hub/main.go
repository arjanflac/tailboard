package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arjanflac/tailboard/internal/discover"
	"github.com/arjanflac/tailboard/internal/hub"
	"github.com/arjanflac/tailboard/internal/service"
	"tailscale.com/tsnet"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install-service" {
		result, err := service.Install("hub", os.Args[2:])
		if err != nil {
			slog.Error("service install failed", "component", "tg-clipboard", "error", err)
			os.Exit(1)
		}
		slog.Info("service installed", "component", "tg-clipboard", "platform", result.Platform, "path", result.Path, "loaded", result.Loaded)
		return
	}
	dev := flag.Bool("dev", false, "development mode: listen on localhost without tsnet")
	addr := flag.String("addr", "localhost:8080", "listen address in dev mode")
	hostname := flag.String("hostname", envString("TG_CLIPBOARD_HOSTNAME", discover.DefaultHubHostname), "tailnet hostname in tsnet mode")
	stateDir := flag.String("state-dir", defaultStateDir(), "tsnet state directory")
	maxHistory := flag.Int("max-history", envInt("TG_CLIPBOARD_MAX_HISTORY", 50), "max history items")
	ttl := flag.Duration("ttl", envDuration("TG_CLIPBOARD_TTL", 24*time.Hour), "item TTL")
	spoolQuota := flag.Int64("spool-quota", envInt64("TG_CLIPBOARD_SPOOL_QUOTA", 10<<30), "maximum transfer spool bytes")
	maxTransferSize := flag.Int64("max-transfer-size", envInt64("TG_CLIPBOARD_MAX_TRANSFER_SIZE", 100<<30), "maximum bytes per transfer")
	transferTTL := flag.Duration("transfer-ttl", envDuration("TG_CLIPBOARD_TRANSFER_TTL", 48*time.Hour), "pending transfer TTL")
	memoryLimit := flag.Int64("memory-limit", envInt64("TG_CLIPBOARD_MEMORY_LIMIT", 48<<20), "soft Go memory limit in bytes (0 to disable)")
	gcPercent := flag.Int("gc-percent", envInt("TG_CLIPBOARD_GC_PERCENT", 25), "Go garbage collection target percentage")
	flag.Parse()
	if *memoryLimit > 0 {
		debug.SetMemoryLimit(*memoryLimit)
	}
	if *gcPercent > 0 {
		debug.SetGCPercent(*gcPercent)
	}

	dbPath := ""
	spoolDir := ""
	if !*dev {
		if err := os.MkdirAll(*stateDir, 0o700); err != nil {
			slog.Error("create state dir failed", "component", "tg-clipboard", "error", err, "state_dir", *stateDir)
			os.Exit(1)
		}
		dbPath = filepath.Join(*stateDir, "clips.db")
		spoolDir = filepath.Join(*stateDir, "spool")
	}

	h, err := hub.New(hub.Config{
		MaxHistory:      *maxHistory,
		TTL:             *ttl,
		DBPath:          dbPath,
		SpoolDir:        spoolDir,
		SpoolQuota:      *spoolQuota,
		MaxTransferSize: *maxTransferSize,
		TransferTTL:     *transferTTL,
	})
	if err != nil {
		slog.Error("hub init failed", "component", "tg-clipboard", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := h.Close(); err != nil {
			slog.Error("hub close failed", "component", "tg-clipboard", "error", err)
		}
	}()

	mux := http.NewServeMux()
	obs := hub.NewObserver()

	var ln net.Listener

	if *dev {
		hub.Register(mux, h, func(r *http.Request) string {
			if name := r.Header.Get("X-Clip-Source"); name != "" {
				return name
			}
			return "dev"
		}, obs)

		ln, err = net.Listen("tcp", *addr)
		if err != nil {
			slog.Error("listen failed", "component", "tg-clipboard", "error", err, "listen_addr", *addr)
			os.Exit(1)
		}
		slog.Info("tg-clipboard dev mode", "component", "tg-clipboard", "listen_addr", *addr)
	} else {
		srv := &tsnet.Server{
			Hostname: *hostname,
			Dir:      *stateDir,
		}
		defer srv.Close()

		// Try TLS first (requires HTTPS enabled in Tailscale admin).
		// Fall back to plain HTTP if HTTPS is not configured.
		ln, err = srv.ListenTLS("tcp", ":443")
		if err != nil {
			slog.Warn("TLS listen failed; falling back to plain HTTP", "component", "tg-clipboard", "error", err)
			ln, err = srv.Listen("tcp", ":80")
			if err != nil {
				slog.Error("tsnet listen failed", "component", "tg-clipboard", "error", err)
				os.Exit(1)
			}
			slog.Info("tg-clipboard listening on tailnet (plain HTTP)", "component", "tg-clipboard", "hostname", *hostname)
		}

		lc, err := srv.LocalClient()
		if err != nil {
			slog.Error("tsnet local client failed", "component", "tg-clipboard", "error", err)
			os.Exit(1)
		}

		hub.Register(mux, h, func(r *http.Request) string {
			who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
			if err != nil {
				return "unknown"
			}
			return who.Node.ComputedName
		}, obs)

		slog.Info("tg-clipboard listening on tailnet", "component", "tg-clipboard", "hostname", *hostname, "state_dir", *stateDir)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(ln)
	}()

	select {
	case sig := <-sigCh:
		obs.BeginShutdown()
		slog.Info("shutdown requested", "component", "tg-clipboard", "signal", sig.String())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "component", "tg-clipboard", "error", err)
			if closeErr := server.Close(); closeErr != nil {
				slog.Error("server close failed", "component", "tg-clipboard", "error", closeErr)
			}
			os.Exit(1)
		}

		if err := <-serveErrCh; err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "component", "tg-clipboard", "error", err)
			os.Exit(1)
		}
	case err := <-serveErrCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "component", "tg-clipboard", "error", err)
			os.Exit(1)
		}
	}
}

func defaultStateDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "tg-clipboard", "tsnet")
	}
	return "/var/lib/tg-clipboard/tsnet"
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envString(key string, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
