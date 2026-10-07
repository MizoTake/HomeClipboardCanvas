package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"homeclipboardcanvas/internal/board"
	"homeclipboardcanvas/web"
)

var testToken = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	app, err := New(Options{Token: testToken, DevHTTP: true, Limits: board.DefaultLimits(), Assets: web.Assets()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	return ts, client
}

func login(t *testing.T, ts *httptest.Server, client *http.Client) {
	t.Helper()
	r, _ := http.NewRequest("POST", ts.URL+"/api/session", strings.NewReader(`{"token":"`+testToken+`"}`))
	r.Header.Set("Origin", ts.URL)
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe session cookie")
	}
}

func TestLANAddressPolicy(t *testing.T) {
	for _, value := range []string{"127.0.0.1:10", "192.168.1.2:443", "10.2.3.4:33", "172.31.0.1:40", "[::1]:80", "[fd00::1]:80", "[::ffff:192.168.2.3]:10"} {
		if !AllowedAddress(value, nil) {
			t.Errorf("LAN denied: %s", value)
		}
	}
	for _, value := range []string{"8.8.8.8:10", "172.32.1.1:10", "100.64.1.1:10", "invalid", "[2001:4860:4860::8888]:10"} {
		if AllowedAddress(value, nil) {
			t.Errorf("external address allowed: %s", value)
		}
	}
}

func TestAuthenticationOriginAndSizeLimit(t *testing.T) {
	ts, client := newTestServer(t)
	resp, _ := client.Get(ts.URL + "/api/board")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated: %d", resp.StatusCode)
	}
	login(t, ts, client)
	resp, _ = client.Get(ts.URL + "/api/board")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("authenticated response or headers invalid")
	}
	r, _ := http.NewRequest("POST", ts.URL+"/api/items", strings.NewReader(`{"text":"bad","width":320,"height":240}`))
	r.Header.Set("Origin", "https://external.invalid")
	resp, _ = client.Do(r)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("origin: %d", resp.StatusCode)
	}
	r, _ = http.NewRequest("POST", ts.URL+"/api/items", strings.NewReader(strings.Repeat("x", (6<<20)+10000)))
	r.Header.Set("Origin", ts.URL)
	r.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(r)
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatalf("request size: %d", resp.StatusCode)
	}
}

func TestWebSocketSnapshotBroadcastDeleteAndClear(t *testing.T) {
	ts, client := newTestServer(t)
	login(t, ts, client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/ws", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": []string{ts.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var event struct {
		Type     string         `json:"type"`
		Snapshot board.Snapshot `json:"snapshot"`
		Item     board.Item     `json:"item"`
	}
	if err := wsjson.Read(ctx, conn, &event); err != nil || event.Type != "board.snapshot" {
		t.Fatalf("snapshot: %v %s", err, event.Type)
	}
	r, _ := http.NewRequest("POST", ts.URL+"/api/items", strings.NewReader(`{"text":"shared","width":320,"height":240}`))
	r.Header.Set("Origin", ts.URL)
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d %s", resp.StatusCode, data)
	}
	var item board.Item
	json.NewDecoder(resp.Body).Decode(&item)
	for {
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "item.created" {
			break
		}
	}
	if event.Item.Text != "shared" {
		t.Fatal("broadcast lost text")
	}
	for _, path := range []string{"/api/items/" + item.ID, "/api/board"} {
		r, _ := http.NewRequest("DELETE", ts.URL+path, nil)
		r.Header.Set("Origin", ts.URL)
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 204 {
			t.Fatalf("delete: %d", resp.StatusCode)
		}
	}
}

func TestWebSocketRejectsUntrustedOrigin(t *testing.T) {
	ts, client := newTestServer(t)
	login(t, ts, client)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/ws", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": []string{"https://evil.invalid"}}})
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatal("untrusted websocket accepted")
	}
}
