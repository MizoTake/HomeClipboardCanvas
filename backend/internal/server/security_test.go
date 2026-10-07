package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"homeclipboardcanvas/internal/board"
	"homeclipboardcanvas/web"
)

func TestTLSAuthenticationIssuesSecureCookieAndRejectsHTTP(t *testing.T) {
	app, err := New(Options{Token: testToken, Limits: board.DefaultLimits(), Assets: web.Assets()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewTLSServer(app.Handler())
	defer ts.Close()
	r, _ := http.NewRequest("POST", ts.URL+"/api/session", strings.NewReader(`{"token":"`+testToken+`"}`))
	r.Header.Set("Origin", ts.URL)
	r.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cookies := resp.Cookies()
	if resp.StatusCode != 204 || len(cookies) != 1 || !cookies[0].Secure || cookies[0].Name != "__Host-hcc-session" {
		t.Fatal("TLS authentication cookie invalid")
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
	app, _ := New(Options{Token: testToken, DevHTTP: true, Limits: board.DefaultLimits(), Assets: web.Assets()})
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

func TestSessionsExpireCannotBeForgedAndRestartInvalidatesThem(t *testing.T) {
	s, _ := newSessions(testToken, false)
	w := httptest.NewRecorder()
	s.issue(w)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(w.Result().Cookies()[0])
	if !s.valid(r) {
		t.Fatal("issued cookie invalid")
	}
	other, _ := newSessions(testToken, false)
	if other.valid(r) {
		t.Fatal("restart restored session")
	}
	for _, value := range []string{"0." + s.signature("0"), "9999999999.forged", "invalid"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: s.cookie, Value: value})
		if s.valid(r) {
			t.Fatal("expired or forged session accepted")
		}
	}
}

func TestLoginRateLimitAndWrongToken(t *testing.T) {
	ts, client := newTestServer(t)
	for i := 0; i < 9; i++ {
		r, _ := http.NewRequest("POST", ts.URL+"/api/session", strings.NewReader(`{"token":"incorrect"}`))
		r.Header.Set("Origin", ts.URL)
		r.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := 401
		if i == 8 {
			want = 429
		}
		if resp.StatusCode != want {
			t.Fatalf("attempt %d: %d", i, resp.StatusCode)
		}
	}
}

func TestReadAfterTTLDoesNotExposeItem(t *testing.T) {
	limits := board.DefaultLimits()
	limits.TTL = time.Millisecond
	app, _ := New(Options{Token: testToken, DevHTTP: true, Limits: limits, Assets: web.Assets()})
	app.Store.AddText("expires", board.Geometry{Width: 320, Height: 240})
	time.Sleep(10 * time.Millisecond)
	if len(app.Store.Snapshot().Items) != 0 {
		t.Fatal("read exposed expired item")
	}
}
