package tailnet

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestOfflineStartupDisconnectAndAddressChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var address atomic.Value
	address.Store("")
	starts := make(chan string, 8)
	stops := make(chan struct{}, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervise(ctx, 5*time.Millisecond, func() (string, error) {
			ip := address.Load().(string)
			if ip == "" {
				return "", errors.New("offline")
			}
			return ip, nil
		}, func(ctx context.Context, ip string) error {
			starts <- ip
			<-ctx.Done()
			stops <- struct{}{}
			return nil
		})
	}()
	select {
	case ip := <-starts:
		t.Fatalf("listened while offline: %s", ip)
	case <-time.After(30 * time.Millisecond):
	}
	awaitStart := func(want string) {
		t.Helper()
		select {
		case got := <-starts:
			if got != want {
				t.Fatalf("listener = %s, want %s", got, want)
			}
		case <-ctx.Done():
			t.Fatal("listener did not start")
		}
	}
	awaitStop := func() {
		t.Helper()
		select {
		case <-stops:
		case <-ctx.Done():
			t.Fatal("listener did not stop")
		}
	}
	address.Store("100.64.0.1")
	awaitStart("100.64.0.1:9437")
	address.Store("")
	awaitStop()
	address.Store("100.64.0.1")
	awaitStart("100.64.0.1:9437")
	address.Store("100.64.0.2")
	awaitStop()
	awaitStart("100.64.0.2:9437")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestListenerFailureRetriesWithoutExiting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attempts := 0
	supervise(ctx, 5*time.Millisecond, func() (string, error) {
		return "100.64.0.1", nil
	}, func(context.Context, string) error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return errors.New("address temporarily unavailable")
	})
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
