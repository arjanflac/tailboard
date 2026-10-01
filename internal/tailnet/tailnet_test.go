package tailnet

import (
	"net"
	"testing"
)

func TestDiscoveryExcludesLANAndAmbiguousVPNs(t *testing.T) {
	up := net.FlagUp | net.FlagPointToPoint
	for _, tc := range []struct {
		name       string
		interfaces []net.Interface
		addresses  map[string]string
		want       string
	}{
		{"LAN CGNAT is not Tailscale", []net.Interface{{Name: "en0", Flags: net.FlagUp}}, map[string]string{"en0": "100.64.1.2/24"}, ""},
		{"active tunnel", []net.Interface{{Name: "en0", Flags: net.FlagUp}, {Name: "utun4", Flags: up}}, map[string]string{"en0": "100.64.1.2/24", "utun4": "100.64.1.3/32"}, "100.64.1.3"},
		{"disconnected tunnel", []net.Interface{{Name: "utun4", Flags: net.FlagPointToPoint}}, map[string]string{"utun4": "100.64.1.3/32"}, ""},
		{"unrelated VPN", []net.Interface{{Name: "utun4", Flags: up}}, map[string]string{"utun4": "10.0.0.2/32"}, ""},
		{"ambiguous VPNs", []net.Interface{{Name: "utun4", Flags: up}, {Name: "utun5", Flags: up}}, map[string]string{"utun4": "100.64.1.3/32", "utun5": "100.64.1.4/32"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ipv4FromInterfaces(tc.interfaces, func(iface net.Interface) ([]net.Addr, error) {
				_, address, err := net.ParseCIDR(tc.addresses[iface.Name])
				return []net.Addr{address}, err
			})
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("address = %q, error = %v; want %q", got, err, tc.want)
			}
		})
	}
}
