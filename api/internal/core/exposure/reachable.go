package exposure

import (
	"net"
	"net/netip"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func isExposed(host string) bool {
	return transports.IsNetworkExposed("tcp://" + net.JoinHostPort(host, "1"))
}

// reachable lists where another device can dial a listener bound at bound: for
// a wildcard bind, each of this machine's global unicast IPv4 addresses at the
// bound port; otherwise the bind itself.
func reachable(bound string, interfaces func() ([]net.Addr, error)) []string {
	host, port, err := net.SplitHostPort(bound)
	if err != nil {
		return nil
	}
	if ip, perr := netip.ParseAddr(strings.Trim(host, "[]")); perr != nil || !ip.IsUnspecified() {
		return []string{bound}
	}
	addrs, err := interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(n.IP)
		if !ok || !ip.Unmap().IsGlobalUnicast() || !ip.Unmap().Is4() {
			continue
		}
		out = append(out, net.JoinHostPort(ip.Unmap().String(), port))
	}
	return out
}
