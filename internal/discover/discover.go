package discover

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thalysguimaraes/tg-clipboard/internal/protocol"
)

const (
	// DefaultHubHostname preserves the historic tailnet hostname.
	DefaultHubHostname  = "tg-clipboard"
	DefaultRolePort     = 9437
	defaultCacheTTL     = 30 * time.Second
	defaultProbeTimeout = 3 * time.Second
)

// Config controls discovery behavior.
type Config struct {
	HubHostname  string
	RolePort     int
	CacheTTL     time.Duration
	ProbeTimeout time.Duration

	readStatus   func(context.Context) (tailnetStatus, error)
	interfaceIPs func() ([]net.IP, error)
	probeURL     func(context.Context, string) (string, error)
	probeRole    func(context.Context, string, int) (string, error)
	now          func() time.Time
}

// Resolver caches tailscale discovery results for a short time so repeated
// calls within a process do not shell out over and over.
type Resolver struct {
	hubHostname  string
	rolePort     int
	cacheTTL     time.Duration
	readStatus   func(context.Context) (tailnetStatus, error)
	interfaceIPs func() ([]net.IP, error)
	probeURL     func(context.Context, string) (string, error)
	probeRole    func(context.Context, string, int) (string, error)
	now          func() time.Time

	mu    sync.Mutex
	cache resolverCache
}

type resolverCache struct {
	status    tailnetStatus
	hubURL    string
	expiresAt time.Time
	valid     bool
}

type tailnetStatus struct {
	Peer map[string]tailnetNode `json:"Peer"`
	Self tailnetNode            `json:"Self"`
}

type tailnetNode struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

// DefaultConfig returns discovery defaults using the current environment.
func DefaultConfig() Config {
	return Config{
		HubHostname:  HubHostnameFromEnv(),
		RolePort:     rolePortFromEnv(),
		CacheTTL:     defaultCacheTTL,
		ProbeTimeout: defaultProbeTimeout,
	}
}

// HubHostnameFromEnv returns the configured tailnet hostname for the hub.
func HubHostnameFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("TG_CLIPBOARD_HOSTNAME")); value != "" {
		return value
	}
	return DefaultHubHostname
}

// NewResolver constructs a resolver with caching, context-aware status lookups,
// and hub probing.
func NewResolver(cfg Config) *Resolver {
	if cfg.HubHostname == "" {
		cfg.HubHostname = DefaultHubHostname
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultProbeTimeout
	}
	if cfg.RolePort <= 0 {
		cfg.RolePort = DefaultRolePort
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.readStatus == nil {
		cfg.readStatus = readTailnetStatus
	}
	if cfg.interfaceIPs == nil {
		cfg.interfaceIPs = localInterfaceIPs
	}

	resolver := &Resolver{
		hubHostname:  cfg.HubHostname,
		rolePort:     cfg.RolePort,
		cacheTTL:     cfg.CacheTTL,
		readStatus:   cfg.readStatus,
		interfaceIPs: cfg.interfaceIPs,
		now:          cfg.now,
	}
	if cfg.probeRole != nil {
		resolver.probeRole = cfg.probeRole
	} else {
		resolver.probeRole = func(ctx context.Context, dns string, port int) (string, error) {
			return probeRoleHubURL(ctx, dns, port, cfg.ProbeTimeout)
		}
	}
	if cfg.probeURL != nil {
		resolver.probeURL = cfg.probeURL
	} else {
		resolver.probeURL = func(ctx context.Context, dns string) (string, error) {
			return probeHubURL(ctx, dns, cfg.ProbeTimeout)
		}
	}

	return resolver
}

// HubURL resolves the hub base URL using the configured tailnet hostname.
func (r *Resolver) HubURL(ctx context.Context) (string, error) {
	status, cachedURL, err := r.status(ctx)
	if err != nil {
		return "", err
	}
	if cachedURL != "" {
		return cachedURL, nil
	}

	var namedErr error
	if dns, err := r.findHubDNS(status); err == nil {
		hubURL, err := r.probeURL(ctx, dns)
		if err == nil {
			r.cacheURL(hubURL)
			return hubURL, nil
		}
		namedErr = err
	} else {
		namedErr = err
	}

	for _, dns := range roleCandidates(status) {
		hubURL, err := r.probeRole(ctx, dns, r.rolePort)
		if err == nil {
			r.cacheURL(hubURL)
			return hubURL, nil
		}
	}

	return "", fmt.Errorf("no reachable hub-role node: %w", namedErr)
}

func (r *Resolver) cacheURL(hubURL string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache.valid && r.now().Before(r.cache.expiresAt) {
		r.cache.hubURL = hubURL
	}
}

// SelfName returns this node's tailscale hostname.
func (r *Resolver) SelfName(ctx context.Context) (string, error) {
	status, _, err := r.status(ctx)
	if err != nil {
		return "", err
	}
	if status.Self.HostName == "" {
		return "", fmt.Errorf("empty hostname in tailscale status")
	}
	return status.Self.HostName, nil
}

// SelfIP returns this node's first Tailscale address, preferring IPv4.
func (r *Resolver) SelfIP(ctx context.Context) (string, error) {
	if addresses, err := r.interfaceIPs(); err == nil {
		if address := pickTailnetIP(addresses); address != "" {
			return address, nil
		}
	}
	status, _, err := r.status(ctx)
	if err != nil {
		return "", err
	}
	for _, address := range status.Self.TailscaleIPs {
		if !strings.Contains(address, ":") {
			return address, nil
		}
	}
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0], nil
	}
	return "", fmt.Errorf("no Tailscale IP in tailscale status")
}

func localInterfaceIPs() ([]net.IP, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		switch value := address.(type) {
		case *net.IPNet:
			ips = append(ips, value.IP)
		case *net.IPAddr:
			ips = append(ips, value.IP)
		}
	}
	return ips, nil
}

func pickTailnetIP(addresses []net.IP) string {
	var ipv6 string
	for _, address := range addresses {
		if address == nil {
			continue
		}
		if ipv4 := address.To4(); ipv4 != nil {
			if ipv4[0] == 100 && ipv4[1]&0xc0 == 0x40 {
				return ipv4.String()
			}
			continue
		}
		if len(address) == net.IPv6len &&
			address[0] == 0xfd && address[1] == 0x7a &&
			address[2] == 0x11 && address[3] == 0x5c &&
			address[4] == 0xa1 && address[5] == 0xe0 {
			ipv6 = address.String()
		}
	}
	return ipv6
}

// HubURL preserves the legacy convenience helper.
func HubURL() (string, error) {
	return NewResolver(DefaultConfig()).HubURL(context.Background())
}

// SelfName preserves the legacy convenience helper.
func SelfName() (string, error) {
	return NewResolver(DefaultConfig()).SelfName(context.Background())
}

func (r *Resolver) status(ctx context.Context) (tailnetStatus, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	if r.cache.valid && r.now().Before(r.cache.expiresAt) {
		status := r.cache.status
		hubURL := r.cache.hubURL
		r.mu.Unlock()
		return status, hubURL, nil
	}
	r.mu.Unlock()

	status, err := r.readStatus(ctx)
	if err != nil {
		return tailnetStatus{}, "", err
	}

	r.mu.Lock()
	r.cache = resolverCache{
		status:    status,
		expiresAt: r.now().Add(r.cacheTTL),
		valid:     true,
	}
	r.mu.Unlock()

	return status, "", nil
}

func (r *Resolver) findHubDNS(status tailnetStatus) (string, error) {
	if status.Self.HostName == r.hubHostname {
		return trimDNS(status.Self.DNSName), nil
	}

	for _, peer := range status.Peer {
		if peer.HostName == r.hubHostname {
			return trimDNS(peer.DNSName), nil
		}
	}

	return "", fmt.Errorf("no %q node found on tailnet", r.hubHostname)
}

func roleCandidates(status tailnetStatus) []string {
	seen := make(map[string]struct{}, len(status.Peer)+1)
	candidates := make([]string, 0, len(status.Peer)+1)
	add := func(node tailnetNode) {
		dns := trimDNS(node.DNSName)
		if dns == "" {
			dns = node.HostName
		}
		if dns == "" {
			return
		}
		if _, ok := seen[dns]; ok {
			return
		}
		seen[dns] = struct{}{}
		candidates = append(candidates, dns)
	}
	add(status.Self)
	for _, peer := range status.Peer {
		add(peer)
	}
	sort.Strings(candidates)
	return candidates
}

func readTailnetStatus(ctx context.Context) (tailnetStatus, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return tailnetStatus{}, fmt.Errorf("tailscale status: %w", err)
	}

	var status tailnetStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return tailnetStatus{}, fmt.Errorf("parse tailscale status: %w", err)
	}

	return status, nil
}

func probeHubURL(ctx context.Context, dns string, timeout time.Duration) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	httpsURL := "https://" + dns
	if err := probeStatusEndpoint(ctx, client, httpsURL, timeout); err == nil {
		return httpsURL, nil
	}

	httpURL := "http://" + dns
	if err := probeStatusEndpoint(ctx, client, httpURL, timeout); err == nil {
		return httpURL, nil
	}

	return "", fmt.Errorf("hub endpoint unavailable at %s", dns)
}

func probeRoleHubURL(ctx context.Context, dns string, port int, timeout time.Duration) (string, error) {
	baseURL := "http://" + netJoinHostPort(dns, port)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint, err := url.JoinPath(baseURL, "api", "capabilities")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("role probe returned %s", resp.Status)
	}
	var capabilities protocol.Capabilities
	if err := json.NewDecoder(resp.Body).Decode(&capabilities); err != nil {
		return "", err
	}
	if !capabilities.Features["hub_role"] {
		return "", fmt.Errorf("node does not advertise hub role")
	}
	return baseURL, nil
}

func netJoinHostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return host + ":" + strconv.Itoa(port)
}

func probeStatusEndpoint(ctx context.Context, client *http.Client, baseURL string, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint, err := url.JoinPath(baseURL, "api", "status")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func trimDNS(dns string) string {
	return strings.TrimSuffix(dns, ".")
}

func rolePortFromEnv() int {
	value := strings.TrimSpace(os.Getenv("TG_CLIPBOARD_ROLE_PORT"))
	if value == "" {
		return DefaultRolePort
	}
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return DefaultRolePort
	}
	return port
}
