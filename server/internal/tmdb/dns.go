package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Some providers answer DNS queries for TMDB with 127.0.0.1 or nothing at
// all. When the system resolver gives no usable address, TMDB hosts are
// resolved over DNS-over-HTTPS instead; the TLS connection itself still goes
// to TMDB with its real certificate.

// dohServers are addressed by IP so that they do not depend on DNS either.
var dohServers = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/resolve", "https://9.9.9.9:5053/dns-query"}

var (
	dohMu    sync.Mutex
	dohCache = map[string]dohEntry{}
)

type dohEntry struct {
	ips []string
	at  time.Time
}

func usable(ips []netip.Addr) bool {
	for _, ip := range ips {
		if !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

// tmdbHost limits the fallback to TMDB itself: a mirror on localhost or in
// the LAN must resolve the normal way.
func tmdbHost(h string) bool {
	return h == "themoviedb.org" || strings.HasSuffix(h, ".themoviedb.org") || h == "tmdb.org" || strings.HasSuffix(h, ".tmdb.org")
}

func newTransport() *http.Transport {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || net.ParseIP(host) != nil || !tmdbHost(host) {
			return d.DialContext(ctx, network, addr)
		}
		ips, _ := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if usable(ips) {
			return d.DialContext(ctx, network, addr)
		}
		alt, err := resolveDoH(ctx, host)
		if err != nil {
			return nil, err
		}
		var last error
		for _, ip := range alt {
			c, err := d.DialContext(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				return c, nil
			}
			last = err
		}
		return nil, last
	}
	return t
}

func resolveDoH(ctx context.Context, host string) ([]string, error) {
	dohMu.Lock()
	e, ok := dohCache[host]
	dohMu.Unlock()
	if ok && time.Since(e.at) < 10*time.Minute {
		return e.ips, nil
	}
	client := &http.Client{Timeout: 8 * time.Second}
	var last error = errors.New("DNS-over-HTTPS недоступен")
	for _, srv := range dohServers {
		ips, err := queryDoH(ctx, client, srv, host)
		if err != nil {
			last = err
			continue
		}
		if len(ips) > 0 {
			dohMu.Lock()
			dohCache[host] = dohEntry{ips: ips, at: time.Now()}
			dohMu.Unlock()
			return ips, nil
		}
	}
	return nil, last
}

func queryDoH(ctx context.Context, client *http.Client, srv, host string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", srv+"?"+url.Values{"name": {host}, "type": {"A"}}.Encode(), nil)
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	var ips []string
	for _, a := range r.Answer {
		if ip, err := netip.ParseAddr(a.Data); a.Type == 1 && err == nil && usable([]netip.Addr{ip}) {
			ips = append(ips, a.Data)
		}
	}
	return ips, nil
}
