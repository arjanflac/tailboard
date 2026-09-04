package tailnet

import (
	"fmt"
	"net"
	"net/netip"
)

// IPv4 finds the CGNAT address assigned by the already-installed Tailscale app.
// Reading network interfaces works inside a macOS login item without spawning
// or coupling Tailboard to Tailscale's CLI.
func IPv4() (string, error) {
	prefix := netip.MustParsePrefix("100.64.0.0/10")
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, networkInterface := range interfaces {
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, value := range addresses {
			address, err := netip.ParsePrefix(value.String())
			if err == nil && prefix.Contains(address.Addr()) {
				return address.Addr().String(), nil
			}
		}
	}
	return "", fmt.Errorf("could not read a Tailscale IPv4 address; is Tailscale running?")
}
