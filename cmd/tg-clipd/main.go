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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/arjanflac/tailboard/internal/agent"
	"github.com/arjanflac/tailboard/internal/deviceid"
	"github.com/arjanflac/tailboard/internal/discover"
	"github.com/arjanflac/tailboard/internal/embeddedhub"
	"github.com/arjanflac/tailboard/internal/hubclient"
	"github.com/arjanflac/tailboard/internal/privacy"
	"github.com/arjanflac/tailboard/internal/service"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

type agentRunner interface {
	Run(context.Context) error
}

var newAgent = func(cfg agent.Config) (agentRunner, error) {
	return agent.New(cfg)
}

// defaultNodeName resolves what this machine is called on device tiles.
// The OS "pretty name" ("MacBook Pro de Thalys") wins over the Tailscale
// hostname and the DNS hostname: the roster shows devices as people-like
// names, not machine identifiers.
func defaultNodeName(ctx context.Context, resolver *discover.Resolver) string {
	if name := prettyHostname(); name != "" {
		return name
	}
	if name, err := resolver.SelfName(ctx); err == nil && name != "" {
		return name
	}
	h, _ := os.Hostname()
	return h
}

// prettyHostname returns the user-facing machine name where the OS has
// one (macOS ComputerName, Linux PRETTY_HOSTNAME), or "".
func prettyHostname() string {
	switch runtime.GOOS {
	case "darwin":
		// Absolute path: launchd agents often run without /usr/sbin on PATH.
		out, err := exec.Command("/usr/sbin/scutil", "--get", "ComputerName").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	case "linux":
		out, err := exec.Command("hostnamectl", "--pretty").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	default:
		return ""
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "install-service" {
		result, err := service.Install("engine", args[1:])
		if err != nil {
			return err
		}
		slog.Info("service installed", "component", "tg-clipd", "platform", result.Platform, "path", result.Path, "loaded", result.Loaded)
		return nil
	}
	fs := flag.NewFlagSet("Tailboard Engine", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	hubURL := fs.String("hub", "", "hub URL (auto-discovered from tailnet if empty)")
	nodeName := fs.String("node", "", "this node's name (auto-discovered from tailscale if empty)")
	pollMs := fs.Int("poll", 500, "clipboard poll interval in milliseconds")
	ignoreApps := fs.String("ignore-apps", envString("TG_CLIPBOARD_IGNORE_APPS", ""), "comma-separated app names or bundle IDs to keep local")
	ignoreProcesses := fs.String("ignore-processes", envString("TG_CLIPBOARD_IGNORE_PROCESSES", ""), "comma-separated process names to keep local")
	filterSensitive := fs.String("filter-sensitive", envString("TG_CLIPBOARD_FILTER_SENSITIVE", ""), "comma-separated sensitive classes to block (secret,password-manager,otp)")
	clearOnBlock := fs.Bool("clear-on-block", envBool("TG_CLIPBOARD_CLEAR_ON_BLOCK", false), "clear the local clipboard when a privacy rule blocks sync")
	privacyPreset := fs.String("privacy-preset", envString("TG_CLIPBOARD_PRIVACY_PRESET", "off"), "privacy bundle: strict, balanced, or off")
	stateDir := fs.String("state-dir", defaultStateDir(), "directory for persistent agent state")
	transferPolicy := fs.String("transfers", envString("TG_CLIPBOARD_TRANSFERS", "ask"), "incoming transfer policy: ask, accept, or off")
	transferAllow := fs.String("transfer-allow", envString("TG_CLIPBOARD_TRANSFER_ALLOW", ""), "comma-separated device IDs allowed for auto-accept")
	downloadDir := fs.String("download-dir", envString("TG_CLIPBOARD_DOWNLOAD_DIR", ""), "incoming transfer destination (default: ~/Downloads)")
	embedHub := fs.Bool("embed-hub", envBool("TG_CLIPBOARD_EMBED_HUB", false), "carry the persistent hub role in this tg-clipd process")
	embedHubAddr := fs.String("embed-hub-addr", envString("TG_CLIPBOARD_EMBED_HUB_ADDR", embeddedhub.DefaultAddress), "listen address for the embedded hub role")
	embedSpoolQuota := fs.Int64("embed-spool-quota", envInt64("TG_CLIPBOARD_EMBED_SPOOL_QUOTA", 10<<30), "embedded hub transfer spool quota")
	embedMaxTransfer := fs.Int64("embed-max-transfer-size", envInt64("TG_CLIPBOARD_EMBED_MAX_TRANSFER_SIZE", 100<<30), "embedded hub maximum transfer size")
	embedTransferTTL := fs.Duration("embed-transfer-ttl", envDuration("TG_CLIPBOARD_EMBED_TRANSFER_TTL", 48*time.Hour), "embedded hub pending transfer TTL")
	controlAddr := fs.String("control-addr", envString("TG_CLIPBOARD_CONTROL_ADDR", "127.0.0.1:9438"), "loopback address for the desktop control surface (off to disable)")
	openControl := fs.Bool("tray", envBool("TG_CLIPBOARD_TRAY", false), "open the desktop device and transfer companion")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.EqualFold(*controlAddr, "off") {
		*controlAddr = ""
	}

	if *hubURL == "" {
		*hubURL = os.Getenv("TG_CLIPBOARD_HUB")
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
		slog.Info("embedded hub role active", "component", "tg-clipd", "listen_addr", listenAddress, "local_url", carriedHub.URL)
		if *hubURL == "" {
			*hubURL = carriedHub.URL
		}
	}
	if *hubURL == "" {
		url, err := resolver.HubURL(ctx)
		if err != nil {
			slog.Warn("hub auto-discovery failed; falling back to localhost", "component", "tg-clipd", "error", err)
			*hubURL = "http://localhost:8080"
		} else {
			slog.Info("discovered hub", "component", "tg-clipd", "hub_url", url)
			*hubURL = url
		}
	}

	if *nodeName == "" {
		*nodeName = defaultNodeName(ctx, resolver)
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
		return filepath.Join(dir, "tg-clipboard")
	}
	return filepath.Join(os.TempDir(), "tg-clipboard")
}

func runMain(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, args); err != nil && err != context.Canceled {
		var initErr *agent.ClipboardInitError
		if errors.As(err, &initErr) {
			slog.Error("clipboard init failed", "component", "tg-clipd", "error", initErr.Err)
		} else {
			slog.Error("tg-clipd exited", "component", "tg-clipd", "error", err)
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
