package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
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
	if *listen == "" {
		ip, err := tailnet.IPv4()
		if err != nil {
			slog.Error("Tailboard requires a connected Tailscale app", "error", err)
			os.Exit(1)
		}
		*listen = net.JoinHostPort(ip, "9437")
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
