// Package netutil finds the PC's LAN address and tells LAN clients from the rest.
package netutil

import (
	"net"
	"net/http"
	"sort"
	"strings"
)

var private = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7", "fe80::/10"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

func IsLAN(ip net.IP) bool {
	for _, n := range private {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func RemoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func IsLoopback(r *http.Request) bool {
	ip := RemoteIP(r)
	return ip != nil && ip.IsLoopback()
}

// LANAddrs returns private IPv4 addresses of up interfaces, physical first.
func LANAddrs() []string {
	type cand struct {
		ip    string
		score int
	}
	var cs []cand
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || !IsLAN(ipn.IP) || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			score := 0
			n := strings.ToLower(i.Name)
			if strings.HasPrefix(ipn.IP.String(), "192.168.") {
				score += 2
			}
			if i.Flags&net.FlagPointToPoint != 0 || strings.Contains(n, "vpn") || strings.HasPrefix(n, "wg") || strings.HasPrefix(n, "tun") ||
				strings.Contains(n, "vethernet") || strings.Contains(n, "virtualbox") || strings.Contains(n, "vmware") || strings.HasPrefix(n, "docker") || strings.HasPrefix(n, "br-") {
				score -= 5
			}
			cs = append(cs, cand{ipn.IP.String(), score})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ip
	}
	return out
}

// LANAddrFor picks the local address on the same subnet as a device, so a TV
// gets a stream URL it can reach.
func LANAddrFor(device string) string {
	dip := net.ParseIP(device)
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && dip != nil && ipn.Contains(dip) {
				return ipn.IP.String()
			}
		}
	}
	if l := LANAddrs(); len(l) > 0 {
		return l[0]
	}
	return "127.0.0.1"
}
