package tailnet

import (
	"context"
	"log/slog"
	"net"
	"time"
)

// Serve waits for Tailscale and restarts the listener when its address changes.
// It never falls back to a public or LAN interface while Tailscale is offline.
func Serve(ctx context.Context, serve func(context.Context, string) error) {
	supervise(ctx, 2*time.Second, IPv4, serve)
}

func supervise(ctx context.Context, interval time.Duration, discover func() (string, error), serve func(context.Context, string) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		ip, err := discover()
		if err == nil {
			listenerContext, cancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- serve(listenerContext, net.JoinHostPort(ip, "9437")) }()
			running := true
			for running {
				select {
				case <-ctx.Done():
					cancel()
					<-done
					return
				case err := <-done:
					if err != nil {
						slog.Warn("Tailboard listener stopped; will retry", "error", err)
					}
					running = false
				case <-ticker.C:
					current, err := discover()
					if err != nil || current != ip {
						cancel()
						<-done
						running = false
					}
				}
			}
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
