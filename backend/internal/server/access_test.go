package server

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"homeclipboardcanvas/internal/board"
	"homeclipboardcanvas/web"
)

func TestLANRequestsWithoutCredentialsAreAllowedAndExternalRequestsAreForbidden(t *testing.T) {
	app := New(Options{Limits: board.DefaultLimits(), Assets: web.Assets()})
	for _, tc := range []struct {
		address string
		status  int
	}{
		{"192.168.1.20:443", 200},
		{"10.0.0.20:443", 200},
		{"172.16.0.20:443", 200},
		{"[::ffff:192.168.1.20]:443", 200},
		{"[fd00::20]:443", 200},
		{"8.8.8.8:443", 403},
		{"[2001:4860::20]:443", 403},
	} {
		r := httptest.NewRequest("GET", "https://canvas.home/api/board", nil)
		r.RemoteAddr = tc.address
		r.TLS = &tls.ConnectionState{}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s: got %d, want %d", tc.address, w.Code, tc.status)
		}
		if w.Header().Get("Set-Cookie") != "" {
			t.Fatal("access issued a session cookie")
		}
	}
}
