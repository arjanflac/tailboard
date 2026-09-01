package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/arjanflac/tailboard/internal/clipboard"
	"github.com/arjanflac/tailboard/internal/deviceid"
	"github.com/arjanflac/tailboard/internal/discover"
	"github.com/arjanflac/tailboard/internal/hubclient"
	"github.com/arjanflac/tailboard/internal/protocol"
)

// version is injected via ldflags in reproducible release builds.
var version = "dev"

var hub *hubclient.Client
var localDeviceID string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Allow --hub flag anywhere.
	hubURL := os.Getenv("TG_CLIPBOARD_HUB")
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
		if strings.HasPrefix(arg, "--hub=") {
			hubURL = strings.TrimPrefix(arg, "--hub=")
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	if hubURL == "" {
		resolver := discover.NewResolver(discover.DefaultConfig())
		if url, err := resolver.HubURL(ctx); err == nil {
			hubURL = url
		} else {
			hubURL = "http://localhost:8080"
		}
	}

	var err error
	hub, err = hubclient.New(hubclient.Config{BaseURL: hubURL})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	localDeviceID, err = deviceid.LoadOrCreate(filepath.Join(defaultStateDir(), "device-id"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	hostName := os.Getenv("TG_CLIPBOARD_NODE")
	if hostName == "" {
		resolver := discover.NewResolver(discover.DefaultConfig())
		if tailnetName, resolveErr := resolver.SelfName(ctx); resolveErr == nil {
			hostName = tailnetName
		}
	}
	if hostName == "" {
		hostName, _ = os.Hostname()
	}
	_, _ = hub.RegisterDevice(ctx, protocol.RegisterDeviceRequest{
		DeviceID: localDeviceID, Name: hostName, Platform: runtime.GOOS,
		Capabilities: []string{"clipboard"}, ReplaceCapabilities: true,
	})

	switch args[0] {
	case "get":
		err = cmdGet(ctx, args[1:])
	case "put":
		err = cmdPut(ctx, args[1:])
	case "status":
		err = cmdStatus(ctx)
	case "devices":
		err = cmdDevices(ctx)
	case "clear":
		err = cmdClear(ctx, args[1:])
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: tg-clip <command> [args]

Commands:
  get                        Print current text
  put [text]                 Send text to clipboard (reads stdin if no args)
  status                     Show hub status
  devices                    List registered devices and online state
  clear [--local]            Clear relay state (and optionally this machine's clipboard)

Flags:
  --hub URL        Hub URL (default: auto-discovered, $TG_CLIPBOARD_HUB, or localhost)

Environment:
  TG_CLIPBOARD_HUB        Explicit hub URL override
  TG_CLIPBOARD_HOSTNAME   Tailnet hostname used for auto-discovery (default: tg-clipboard)
`)
}

func defaultStateDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "tg-clipboard")
	}
	return filepath.Join(os.TempDir(), "tg-clipboard")
}

func cmdDevices(ctx context.Context) error {
	devices, err := hub.Devices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("(no registered devices)")
		return nil
	}
	for _, device := range devices {
		state := "offline"
		if device.Online {
			state = "online"
		}
		fmt.Printf("%-24s %-10s %-8s %s\n", device.Name, device.Platform, state, device.DeviceID)
	}
	return nil
}

func cmdGet(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("get does not accept arguments")
	}

	item, err := hub.Current(ctx)
	if errors.Is(err, hubclient.ErrNoCurrentClip) {
		fmt.Fprintln(os.Stderr, "(clipboard empty)")
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Print(item.Content)
	return nil
}

func cmdPut(ctx context.Context, args []string) error {
	var content string
	if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		content = string(data)
	}
	if content == "" {
		return fmt.Errorf("no content provided")
	}

	item, err := hub.Put(ctx, hubclient.PutRequest{Content: content})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "stored (seq=%d, %d bytes)\n", item.Seq, len(item.Content))
	return nil
}

func cmdClear(ctx context.Context, args []string) error {
	clearLocal := false
	for _, arg := range args {
		if arg == "--local" {
			clearLocal = true
		}
	}

	if err := hub.Clear(ctx); err != nil {
		return err
	}

	if clearLocal {
		localClipboard, err := clipboard.New()
		if err != nil {
			return err
		}
		if err := localClipboard.Clear(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "cleared relay state and the local system clipboard")
		return nil
	}

	fmt.Fprintln(os.Stderr, "cleared relay state")
	return nil
}

func cmdStatus(ctx context.Context) error {
	status, err := hub.Status(ctx)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(status))
	for key := range status {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fmt.Printf("%-24s %v\n", key+":", status[key])
	}
	if devices, err := hub.Devices(ctx); err == nil {
		for _, device := range devices {
			if device.DeviceID != localDeviceID {
				continue
			}
			for _, capability := range device.Capabilities {
				if strings.HasPrefix(capability, "privacy-detector:") {
					fmt.Printf("%-24s %s\n", "privacy_detector:", strings.TrimPrefix(capability, "privacy-detector:"))
				}
			}
		}
	}
	return nil
}
