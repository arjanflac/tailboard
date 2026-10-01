package tailnet

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// IPv4 finds the CGNAT address assigned by the already-installed Tailscale app.
// Reading network interfaces works inside a macOS login item without spawning
// or coupling Tailboard to Tailscale's CLI.
func IPv4() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	return ipv4FromInterfaces(interfaces, func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() })
}

func ipv4FromInterfaces(interfaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) (string, error) {
	prefix := netip.MustParsePrefix("100.64.0.0/10")
	var found string
	for _, networkInterface := range interfaces {
		// CGNAT addresses also occur on ISP/LAN interfaces. Only use an active
		// macOS tunnel, and fail closed if more than one VPN could be Tailscale.
		if !strings.HasPrefix(networkInterface.Name, "utun") ||
			networkInterface.Flags&(net.FlagUp|net.FlagPointToPoint) != net.FlagUp|net.FlagPointToPoint {
			continue
		}
		addresses, err := addrs(networkInterface)
		if err != nil {
			continue
		}
		for _, value := range addresses {
			address, err := netip.ParsePrefix(value.String())
			if err == nil && prefix.Contains(address.Addr()) {
				ip := address.Addr().String()
				if found != "" && found != ip {
					return "", fmt.Errorf("multiple CGNAT VPN addresses; use an explicit Tailscale listen address")
				}
				found = ip
			}
		}
	}
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("could not read a Tailscale IPv4 address; is Tailscale running?")
}
