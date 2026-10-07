package server

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"homeclipboardcanvas/internal/board"
	"homeclipboardcanvas/web"
)

func TestTLSAllowsLANWithoutCredentialsAndRejectsHTTP(t *testing.T) {
	app := New(Options{Limits: board.DefaultLimits(), Assets: web.Assets()})
	ts := httptest.NewTLSServer(app.Handler())
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(resp.Cookies()) != 0 {
		t.Fatal("TLS LAN access requires credentials or issues a cookie")
	}
	req := httptest.NewRequest("GET", "http://127.0.0.1/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("production accepted plaintext HTTP")
	}
}

func TestIPPolicyCannotBeBypassedWithForwardedHeaders(t *testing.T) {
	app := New(Options{DevHTTP: true, Limits: board.DefaultLimits(), Assets: web.Assets()})
	r := httptest.NewRequest("GET", "http://127.0.0.1/", nil)
	r.RemoteAddr = "8.8.8.8:12345"
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("forwarded header bypassed LAN restriction")
	}
	if !AllowedAddress("100.64.1.1:80", []netip.Prefix{netip.MustParsePrefix("100.64.0.0/10")}) {
		t.Fatal("additional CIDR rejected")
	}
}

func TestLANAccessStillEnforcesAPIRateLimit(t *testing.T) {
	ts, client := newTestServer(t)
	for i := 0; i < 200; i++ {
		resp, err := client.Get(ts.URL + "/api/board")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == 429 {
			if resp.Header.Get("Retry-After") != "1" {
				t.Fatal("missing retry delay")
			}
			return
		}
		if resp.StatusCode != 200 {
			t.Fatalf("LAN request: %d", resp.StatusCode)
		}
	}
	t.Fatal("API rate limit did not reject excess requests")
}

func TestReadAfterTTLDoesNotExposeItem(t *testing.T) {
	limits := board.DefaultLimits()
	limits.TTL = time.Millisecond
	app := New(Options{DevHTTP: true, Limits: limits, Assets: web.Assets()})
	app.Store.AddText("expires", board.Geometry{Width: 320, Height: 240})
	time.Sleep(10 * time.Millisecond)
	if len(app.Store.Snapshot().Items) != 0 {
		t.Fatal("read exposed expired item")
	}
}
