package server

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

var defaultNetworks = []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7"}

func AllowedAddress(remote string, extra []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, cidr := range defaultNetworks {
		if netip.MustParsePrefix(cidr).Contains(ip) {
			return true
		}
	}
	for _, cidr := range extra {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func sameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return err == nil && origin.Scheme == scheme && strings.EqualFold(origin.Host, r.Host) && origin.User == nil && origin.RawQuery == "" && origin.Path == "" && origin.Fragment == ""
}

type rateEntry struct {
	tokens float64
	at     time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

func (l *limiter) allow(key string, capacity, perSecond float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil {
		l.entries = make(map[string]rateEntry)
	}
	now := time.Now()
	e, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= 2048 {
			for k, v := range l.entries {
				if now.Sub(v.at) > 5*time.Minute {
					delete(l.entries, k)
				}
			}
			if len(l.entries) >= 2048 {
				return false
			}
		}
		e = rateEntry{capacity, now}
	}
	e.tokens = min(capacity, e.tokens+now.Sub(e.at).Seconds()*perSecond)
	e.at = now
	allowed := e.tokens >= 1
	if allowed {
		e.tokens--
	}
	l.entries[key] = e
	return allowed
}
