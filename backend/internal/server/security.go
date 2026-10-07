package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
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

type sessions struct {
	tokenHash [32]byte
	key       []byte
	cookie    string
	secure    bool
}

func newSessions(token string, dev bool) (*sessions, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return nil, errToken
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	name := "__Host-hcc-session"
	if dev {
		name = "hcc-dev-session"
	}
	return &sessions{sha256.Sum256([]byte(token)), key, name, !dev}, nil
}

func (s *sessions) tokenValid(token string) bool {
	digest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(s.tokenHash[:], digest[:]) == 1
}
func (s *sessions) signature(value string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *sessions) issue(w http.ResponseWriter) {
	expires := time.Now().Add(24 * time.Hour)
	value := strconv.FormatInt(expires.Unix(), 10)
	http.SetCookie(w, &http.Cookie{Name: s.cookie, Value: value + "." + s.signature(value), Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: 86400, Expires: expires})
}
func (s *sessions) valid(r *http.Request) bool {
	cookie, err := r.Cookie(s.cookie)
	if err != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	return err == nil && expiry > time.Now().Unix() && hmac.Equal([]byte(parts[1]), []byte(s.signature(parts[0])))
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
