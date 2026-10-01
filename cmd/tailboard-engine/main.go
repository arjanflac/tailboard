package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/arjanflac/tailboard/internal/clipboard"
	"github.com/arjanflac/tailboard/internal/engine"
	"github.com/arjanflac/tailboard/internal/relay"
	"github.com/arjanflac/tailboard/internal/tailnet"
)

var version = "dev"

func main() {
	listen := flag.String("listen", "", "listen address (default: this Mac's Tailscale IPv4 on port 9437)")
	name := flag.String("name", "", "Mac name shown on Android")
	poll := flag.Duration("poll", 100*time.Millisecond, "clipboard polling interval")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	debug.SetMemoryLimit(32 << 20)
	debug.SetGCPercent(25)
	if *poll <= 0 {
		slog.Error("clipboard polling interval must be positive")
		os.Exit(2)
	}
	if *name == "" {
		*name = computerName()
	}

	board, err := clipboard.New()
	if err != nil {
		slog.Error("could not open the Mac clipboard", "error", err)
		os.Exit(1)
	}
	engine := engine.New(board, *name)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		ticker := time.NewTicker(*poll)
		defer ticker.Stop()
		engine.Poll()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				engine.Poll()
			}
		}
	}()

	if *listen == "" {
		slog.Info("Tailboard waiting for Tailscale", "name", *name)
		tailnet.Serve(ctx, func(ctx context.Context, address string) error {
			slog.Info("Tailboard listening", "listen", address)
			return relay.Listen(ctx, address, engine.Relay())
		})
		return
	}
	slog.Info("Tailboard started", "listen", *listen, "name", *name)
	if err := relay.Listen(ctx, *listen, engine.Relay()); err != nil {
		slog.Error("Tailboard stopped", "error", err)
		os.Exit(1)
	}
}

func computerName() string {
	output, err := exec.Command("/usr/sbin/scutil", "--get", "ComputerName").Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return strings.TrimSpace(string(output))
	}
	name, _ := os.Hostname()
	return name
}
