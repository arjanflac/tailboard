package discover

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHubHostnameFromEnv(t *testing.T) {
	t.Setenv("TG_CLIPBOARD_HOSTNAME", "")
	if got := HubHostnameFromEnv(); got != DefaultHubHostname {
		t.Fatalf("expected default hostname %q, got %q", DefaultHubHostname, got)
	}

	t.Setenv("TG_CLIPBOARD_HOSTNAME", "custom-hub")
	if got := HubHostnameFromEnv(); got != "custom-hub" {
		t.Fatalf("expected env hostname override, got %q", got)
	}
}

func TestResolverCachesStatusAcrossCalls(t *testing.T) {
	var (
		now        = time.Unix(100, 0)
		readCalls  int
		probeCalls int
	)

	resolver := NewResolver(Config{
		HubHostname: "custom-hub",
		CacheTTL:    time.Minute,
		readStatus: func(context.Context) (tailnetStatus, error) {
			readCalls++
			return tailnetStatus{
				Self: tailnetNode{HostName: "laptop", DNSName: "laptop.tailnet.ts.net."},
				Peer: map[string]tailnetNode{
					"peer": {HostName: "custom-hub", DNSName: "custom-hub.tailnet.ts.net."},
				},
			}, nil
		},
		probeURL: func(context.Context, string) (string, error) {
			probeCalls++
			return "https://custom-hub.tailnet.ts.net", nil
		},
		now: func() time.Time { return now },
	})

	hubURL, err := resolver.HubURL(context.Background())
	if err != nil {
		t.Fatalf("HubURL() error = %v", err)
	}
	if hubURL != "https://custom-hub.tailnet.ts.net" {
		t.Fatalf("unexpected hub URL %q", hubURL)
	}

	selfName, err := resolver.SelfName(context.Background())
	if err != nil {
		t.Fatalf("SelfName() error = %v", err)
	}
	if selfName != "laptop" {
		t.Fatalf("unexpected self name %q", selfName)
	}

	hubURL, err = resolver.HubURL(context.Background())
	if err != nil {
		t.Fatalf("HubURL() second call error = %v", err)
	}
	if hubURL != "https://custom-hub.tailnet.ts.net" {
		t.Fatalf("unexpected cached hub URL %q", hubURL)
	}

	if readCalls != 1 {
		t.Fatalf("expected one tailscale status read, got %d", readCalls)
	}
	if probeCalls != 1 {
		t.Fatalf("expected one hub probe, got %d", probeCalls)
	}
}

func TestResolverHubURLMissingDiscoveryData(t *testing.T) {
	resolver := NewResolver(Config{
		HubHostname: "custom-hub",
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{
				Self: tailnetNode{HostName: "laptop", DNSName: "laptop.tailnet.ts.net."},
			}, nil
		},
		probeURL: func(context.Context, string) (string, error) {
			t.Fatal("probeURL should not run when no matching hub exists")
			return "", nil
		},
		probeRole: func(context.Context, string, int) (string, error) {
			return "", fmt.Errorf("not a hub role")
		},
	})

	_, err := resolver.HubURL(context.Background())
	if err == nil {
		t.Fatal("expected missing hub error")
	}
	if !strings.Contains(err.Error(), `no "custom-hub" node found on tailnet`) {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestResolverFindsEmbeddedHubByRole(t *testing.T) {
	var probes []string
	resolver := NewResolver(Config{
		HubHostname: "tg-clipboard",
		RolePort:    9437,
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{
				Self: tailnetNode{HostName: "laptop", DNSName: "laptop.tail.ts.net."},
				Peer: map[string]tailnetNode{
					"desktop": {HostName: "desktop", DNSName: "desktop.tail.ts.net."},
				},
			}, nil
		},
		probeRole: func(_ context.Context, dns string, port int) (string, error) {
			probes = append(probes, dns)
			if dns == "desktop.tail.ts.net" && port == 9437 {
				return "http://desktop.tail.ts.net:9437", nil
			}
			return "", fmt.Errorf("not a hub role")
		},
	})

	got, err := resolver.HubURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://desktop.tail.ts.net:9437" {
		t.Fatalf("unexpected role URL %q", got)
	}
	if len(probes) != 1 || probes[0] != "desktop.tail.ts.net" {
		t.Fatalf("expected deterministic role probing, got %v", probes)
	}
}

func TestRolePortFromEnv(t *testing.T) {
	t.Setenv("TG_CLIPBOARD_ROLE_PORT", "10437")
	if got := DefaultConfig().RolePort; got != 10437 {
		t.Fatalf("expected configured role port, got %d", got)
	}
	t.Setenv("TG_CLIPBOARD_ROLE_PORT", "invalid")
	if got := DefaultConfig().RolePort; got != DefaultRolePort {
		t.Fatalf("expected default role port, got %d", got)
	}
}

func TestResolverSelfNameMissingHostname(t *testing.T) {
	resolver := NewResolver(Config{
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{}, nil
		},
	})

	_, err := resolver.SelfName(context.Background())
	if err == nil {
		t.Fatal("expected missing hostname error")
	}
	if !strings.Contains(err.Error(), "empty hostname") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestResolverSelfIPPrefersIPv4(t *testing.T) {
	resolver := NewResolver(Config{
		interfaceIPs: func() ([]net.IP, error) { return nil, nil },
		readStatus: func(context.Context) (tailnetStatus, error) {
			return tailnetStatus{Self: tailnetNode{TailscaleIPs: []string{"fd7a:115c:a1e0::1", "100.64.0.7"}}}, nil
		},
	})
	got, err := resolver.SelfIP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "100.64.0.7" {
		t.Fatalf("expected IPv4 address, got %q", got)
	}
}

func TestResolverSelfIPUsesLocalTailnetInterfaceWithoutStatusCommand(t *testing.T) {
	statusReads := 0
	resolver := NewResolver(Config{
		interfaceIPs: func() ([]net.IP, error) {
			return []net.IP{
				net.ParseIP("192.168.1.4"),
				net.ParseIP("fd7a:115c:a1e0::7"),
				net.ParseIP("100.116.168.32"),
			}, nil
		},
		readStatus: func(context.Context) (tailnetStatus, error) {
			statusReads++
			return tailnetStatus{}, errors.New("status should not be read")
		},
	})
	got, err := resolver.SelfIP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "100.116.168.32" {
		t.Fatalf("expected local Tailscale IPv4 address, got %q", got)
	}
	if statusReads != 0 {
		t.Fatalf("expected no status command, got %d reads", statusReads)
	}
}

func TestReadTailnetStatusCommandFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readTailnetStatus(ctx)
	if err == nil {
		t.Fatal("expected command failure")
	}
}

func TestDefaultConfigUsesEnvOverride(t *testing.T) {
	t.Setenv("TG_CLIPBOARD_HOSTNAME", "ci-hub")
	cfg := DefaultConfig()
	if cfg.HubHostname != "ci-hub" {
		t.Fatalf("expected env hostname in config, got %q", cfg.HubHostname)
	}
	if cfg.CacheTTL <= 0 {
		t.Fatalf("expected positive cache TTL, got %s", cfg.CacheTTL)
	}
	if cfg.ProbeTimeout <= 0 {
		t.Fatalf("expected positive probe timeout, got %s", cfg.ProbeTimeout)
	}
}

func TestHubURLLegacyHelperUsesEnv(t *testing.T) {
	t.Setenv("TG_CLIPBOARD_HOSTNAME", "legacy-hub")

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer t.Setenv("PATH", origPath)

	if _, err := HubURL(); err == nil {
		t.Fatal("expected helper to attempt tailscale lookup")
	}
}
