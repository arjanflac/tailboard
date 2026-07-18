package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thalysguimaraes/cliphub/internal/agent"
	"github.com/thalysguimaraes/cliphub/internal/deviceid"
	"github.com/thalysguimaraes/cliphub/internal/discover"
	"github.com/thalysguimaraes/cliphub/internal/embeddedhub"
	"github.com/thalysguimaraes/cliphub/internal/hubclient"
	"github.com/thalysguimaraes/cliphub/internal/privacy"
	"github.com/thalysguimaraes/cliphub/internal/service"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

type agentRunner interface {
	Run(context.Context) error
}

var newAgent = func(cfg agent.Config) (agentRunner, error) {
	return agent.New(cfg)
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "install-service" {
		result, err := service.Install("clipd", args[1:])
		if err != nil {
			return err
		}
		slog.Info("service installed", "component", "clipd", "platform", result.Platform, "path", result.Path, "loaded", result.Loaded)
		return nil
	}
	fs := flag.NewFlagSet("clipd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	hubURL := fs.String("hub", "", "hub URL (auto-discovered from tailnet if empty)")
	nodeName := fs.String("node", "", "this node's name (auto-discovered from tailscale if empty)")
	pollMs := fs.Int("poll", 500, "clipboard poll interval in milliseconds")
	ignoreApps := fs.String("ignore-apps", envString("CLIPHUB_IGNORE_APPS", ""), "comma-separated app names or bundle IDs to keep local")
	ignoreProcesses := fs.String("ignore-processes", envString("CLIPHUB_IGNORE_PROCESSES", ""), "comma-separated process names to keep local")
	filterSensitive := fs.String("filter-sensitive", envString("CLIPHUB_FILTER_SENSITIVE", ""), "comma-separated sensitive classes to block (secret,password-manager,otp)")
	clearOnBlock := fs.Bool("clear-on-block", envBool("CLIPHUB_CLEAR_ON_BLOCK", false), "clear the local clipboard when a privacy rule blocks sync")
	privacyPreset := fs.String("privacy-preset", envString("CLIPHUB_PRIVACY_PRESET", "off"), "privacy bundle: strict, balanced, or off")
	stateDir := fs.String("state-dir", defaultStateDir(), "directory for persistent agent state")
	transferPolicy := fs.String("transfers", envString("CLIPHUB_TRANSFERS", "ask"), "incoming transfer policy: ask, accept, or off")
	transferAllow := fs.String("transfer-allow", envString("CLIPHUB_TRANSFER_ALLOW", ""), "comma-separated device IDs allowed for auto-accept")
	downloadDir := fs.String("download-dir", envString("CLIPHUB_DOWNLOAD_DIR", ""), "incoming transfer destination (default: ~/Downloads)")
	embedHub := fs.Bool("embed-hub", envBool("CLIPHUB_EMBED_HUB", false), "carry the persistent hub role in this clipd process")
	embedHubAddr := fs.String("embed-hub-addr", envString("CLIPHUB_EMBED_HUB_ADDR", embeddedhub.DefaultAddress), "listen address for the embedded hub role")
	embedSpoolQuota := fs.Int64("embed-spool-quota", envInt64("CLIPHUB_EMBED_SPOOL_QUOTA", 10<<30), "embedded hub transfer spool quota")
	embedMaxTransfer := fs.Int64("embed-max-transfer-size", envInt64("CLIPHUB_EMBED_MAX_TRANSFER_SIZE", 100<<30), "embedded hub maximum transfer size")
	embedTransferTTL := fs.Duration("embed-transfer-ttl", envDuration("CLIPHUB_EMBED_TRANSFER_TTL", 48*time.Hour), "embedded hub pending transfer TTL")
	controlAddr := fs.String("control-addr", envString("CLIPHUB_CONTROL_ADDR", "127.0.0.1:9438"), "loopback address for the desktop control surface (off to disable)")
	openControl := fs.Bool("tray", envBool("CLIPHUB_TRAY", false), "open the desktop device and transfer companion")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.EqualFold(*controlAddr, "off") {
		*controlAddr = ""
	}

	if *hubURL == "" {
		*hubURL = os.Getenv("CLIPHUB_HUB")
	}
	resolver := discover.NewResolver(discover.DefaultConfig())
	var carriedHub *embeddedhub.Server
	if *embedHub {
		listenAddress := *embedHubAddr
		if host, port, err := net.SplitHostPort(listenAddress); err == nil && host == "" {
			tailnetIP, err := resolver.SelfIP(ctx)
			if err != nil {
				return fmt.Errorf("resolve tailnet address for embedded hub: %w", err)
			}
			listenAddress = net.JoinHostPort(tailnetIP, port)
		}
		var err error
		carriedHub, err = embeddedhub.Start(ctx, embeddedhub.Config{
			Address:         listenAddress,
			StateDir:        filepath.Join(*stateDir, "embedded-hub"),
			SpoolQuota:      *embedSpoolQuota,
			MaxTransferSize: *embedMaxTransfer,
			TransferTTL:     *embedTransferTTL,
		})
		if err != nil {
			return err
		}
		defer carriedHub.Shutdown(context.Background())
		slog.Info("embedded hub role active", "component", "clipd", "listen_addr", listenAddress, "local_url", carriedHub.URL)
		if *hubURL == "" {
			*hubURL = carriedHub.URL
		}
	}
	if *hubURL == "" {
		url, err := resolver.HubURL(ctx)
		if err != nil {
			slog.Warn("hub auto-discovery failed; falling back to localhost", "component", "clipd", "error", err)
			*hubURL = "http://localhost:8080"
		} else {
			slog.Info("discovered hub", "component", "clipd", "hub_url", url)
			*hubURL = url
		}
	}

	if *nodeName == "" {
		name, err := resolver.SelfName(ctx)
		if err != nil {
			h, _ := os.Hostname()
			*nodeName = h
		} else {
			*nodeName = name
		}
	}

	client, err := hubclient.New(hubclient.Config{BaseURL: *hubURL})
	if err != nil {
		return err
	}
	stableDeviceID, err := deviceid.LoadOrCreate(filepath.Join(*stateDir, "device-id"))
	if err != nil {
		return err
	}

	sensitiveClasses, err := privacy.ParseSensitiveClasses(*filterSensitive)
	if err != nil {
		return err
	}
	presetConfig, err := privacy.Preset(*privacyPreset)
	if err != nil {
		return err
	}
	privacyConfig := presetConfig.Merge(privacy.NewConfig(
		privacy.ParseCSV(*ignoreApps),
		privacy.ParseCSV(*ignoreProcesses),
		sensitiveClasses,
		*clearOnBlock,
	))

	a, err := newAgent(agent.Config{
		HubURL:         *hubURL,
		Client:         client,
		NodeName:       *nodeName,
		DeviceID:       stableDeviceID,
		PollInterval:   time.Duration(*pollMs) * time.Millisecond,
		Privacy:        privacyConfig,
		TransferPolicy: *transferPolicy,
		TransferAllow:  privacy.ParseCSV(*transferAllow),
		DownloadDir:    *downloadDir,
		ControlAddr:    *controlAddr,
		OpenControl:    *openControl,
	})
	if err != nil {
		return err
	}

	return a.Run(ctx)
}

func defaultStateDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "cliphub")
	}
	return filepath.Join(os.TempDir(), "cliphub")
}

func runMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, args); err != nil && err != context.Canceled {
		var initErr *agent.ClipboardInitError
		if errors.As(err, &initErr) {
			slog.Error("clipboard init failed", "component", "clipd", "error", initErr.Err)
		} else {
			slog.Error("clipd exited", "component", "clipd", "error", err)
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(runMain(os.Args[1:]))
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
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

func envInt64(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}
