package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"homeclipboardcanvas/internal/board"
)

var errToken = errors.New("token must be a base64url encoded 32-byte secret; use generate-token")

type Options struct {
	Token           string
	DevHTTP         bool
	AllowedNetworks []netip.Prefix
	Limits          board.Limits
	Assets          fs.FS
	Logger          *slog.Logger
}

type App struct {
	Store    *board.Store
	opts     Options
	sessions *sessions
	rate     limiter
	uploads  chan struct{}
}

func New(o Options) (*App, error) {
	sessions, err := newSessions(o.Token, o.DevHTTP)
	if err != nil {
		return nil, err
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &App{Store: board.New(o.Limits), opts: o, sessions: sessions, uploads: make(chan struct{}, 2)}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("POST /api/session", a.login)
	mux.HandleFunc("GET /api/board", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.Store.Snapshot()) })
	mux.HandleFunc("DELETE /api/board", func(w http.ResponseWriter, r *http.Request) { a.Store.Clear(); w.WriteHeader(204) })
	mux.HandleFunc("POST /api/items", a.createText)
	mux.HandleFunc("POST /api/images", a.createImage)
	mux.HandleFunc("GET /api/images/{id}", a.getImage)
	mux.HandleFunc("PATCH /api/items/{id}", a.update)
	mux.HandleFunc("DELETE /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.Delete(r.PathValue("id")); err != nil {
			a.storeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/viewport", a.viewport)
	mux.HandleFunc("GET /api/ws", a.socket)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "endpoint not found") })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		http.FileServer(http.FS(a.opts.Assets)).ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Permissions-Policy", "clipboard-read=(self), clipboard-write=(self), camera=(), microphone=(), geolocation=()")
		if !AllowedAddress(r.RemoteAddr, a.opts.AllowedNetworks) {
			fail(w, 403, "LAN access only")
			return
		}
		if a.opts.DevHTTP {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			ip, _ := netip.ParseAddr(host)
			if !ip.IsLoopback() {
				fail(w, 403, "development is loopback only")
				return
			}
		}
		if !a.opts.DevHTTP && r.TLS == nil {
			fail(w, 403, "HTTPS required")
			return
		}
		if r.URL.Query().Has("token") && (r.URL.Path != "/" || r.Method != "GET") {
			fail(w, 400, "authenticate using POST /api/session")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Method != "GET" || r.URL.Path == "/api/ws" {
				if !sameOrigin(r) {
					fail(w, 403, "same origin required")
					return
				}
			}
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if !a.rate.allow(host, 80, 4) {
				w.Header().Set("Retry-After", "1")
				fail(w, 429, "too many requests")
				return
			}
			if r.URL.Path != "/api/session" && !a.sessions.valid(r) {
				fail(w, 401, "authentication required")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, max int64, target any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		fail(w, 415, "application/json required")
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			fail(w, 413, "request too large")
		} else {
			fail(w, 400, "unable to read request")
		}
		return false
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		fail(w, 400, "invalid JSON")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, 400, "one JSON object required")
		return false
	}
	return true
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !a.rate.allow("login:"+host, 8, 1.0/10) {
		w.Header().Set("Retry-After", "10")
		fail(w, 429, "too many login attempts")
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if !decode(w, r, 1024, &payload) {
		return
	}
	if !a.sessions.tokenValid(payload.Token) {
		fail(w, 401, "invalid token")
		return
	}
	a.sessions.issue(w)
	w.WriteHeader(204)
}
func (a *App) createText(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploads <- struct{}{}:
		defer func() { <-a.uploads }()
	default:
		fail(w, 429, "card processing busy; retry shortly")
		return
	}
	var payload struct {
		Text string `json:"text"`
		board.Geometry
	}
	// JSONエスケープの増加を理由に、上限内の元テキストを拒否しない。
	if !decode(w, r, a.opts.Limits.MaxText*6+8192, &payload) {
		return
	}
	item, err := a.Store.AddText(payload.Text, payload.Geometry)
	if err != nil {
		a.storeError(w, err)
		return
	}
	a.opts.Logger.Info("item created", "type", "text", "size", item.Size)
	writeJSON(w, 201, item)
}
func (a *App) createImage(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploads <- struct{}{}:
		defer func() { <-a.uploads }()
	default:
		fail(w, 429, "image processing busy; retry shortly")
		return
	}
	mime := r.Header.Get("Content-Type")
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" {
		fail(w, 415, "only PNG, JPEG and GIF images accepted")
		return
	}
	values := r.URL.Query()
	nums := [4]float64{}
	for i, key := range []string{"x", "y", "width", "height"} {
		n, err := strconv.ParseFloat(values.Get(key), 64)
		if err != nil {
			fail(w, 400, "invalid image geometry")
			return
		}
		nums[i] = n
	}
	g := board.Geometry{X: nums[0], Y: nums[1], Width: nums[2], Height: nums[3]}
	if !g.Valid() {
		fail(w, 400, "invalid image geometry")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.opts.Limits.MaxImage))
	if err != nil {
		fail(w, 413, "image exceeds size limit")
		return
	}
	item, err := a.Store.AddImage(data, mime, g)
	if err != nil {
		a.storeError(w, err)
		return
	}
	a.opts.Logger.Info("item created", "type", "image", "size", item.Size)
	writeJSON(w, 201, item)
}
func (a *App) getImage(w http.ResponseWriter, r *http.Request) {
	data, mime, ok := a.Store.Image(r.PathValue("id"))
	if !ok {
		fail(w, 404, "image not found")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}
func (a *App) update(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		board.Geometry
		Revision uint64 `json:"revision"`
	}
	if !decode(w, r, 2048, &payload) {
		return
	}
	item, err := a.Store.Update(r.PathValue("id"), payload.Geometry, payload.Revision)
	if err != nil {
		if errors.Is(err, board.ErrConflict) {
			writeJSON(w, 409, map[string]any{"error": "item changed on another device", "item": item})
			return
		}
		a.storeError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (a *App) viewport(w http.ResponseWriter, r *http.Request) {
	var v board.Viewport
	if !decode(w, r, 2048, &v) {
		return
	}
	a.Store.SetViewport(v)
	w.WriteHeader(204)
}
func (a *App) storeError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, board.ErrLimit) {
		status = 413
	}
	if errors.Is(err, board.ErrNotFound) {
		status = 404
	}
	if errors.Is(err, board.ErrImage) {
		status = 415
	}
	fail(w, status, err.Error())
}

func (a *App) socket(w http.ResponseWriter, r *http.Request) {
	c, snapshot := a.Store.Subscribe()
	if c == nil {
		fail(w, 429, "maximum connected devices reached")
		return
	}
	defer a.Store.Unsubscribe(c)
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1024)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	ctx = conn.CloseRead(ctx)
	write := func(value any) error {
		timeout, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		return wsjson.Write(timeout, conn, value)
	}
	if err := write(map[string]any{"type": "board.snapshot", "snapshot": snapshot}); err != nil {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-c.Events:
			if !ok {
				conn.Close(websocket.StatusTryAgainLater, "reconnect for board snapshot")
				return
			}
			if err := write(event); err != nil {
				return
			}
		case <-ping.C:
			timeout, stop := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(timeout)
			stop()
			if err != nil || !a.sessions.valid(r) {
				return
			}
		}
	}
}

func (a *App) ExpireLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.Store.Clear()
			return
		case <-ticker.C:
			a.Store.Expire()
		}
	}
}
