package board

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrLimit    = errors.New("board limit exceeded")
	ErrNotFound = errors.New("item not found")
	ErrConflict = errors.New("item changed on another device")
	ErrImage    = errors.New("only valid PNG, JPEG and GIF images are accepted")
	ErrGeometry = errors.New("invalid card geometry")
	ErrText     = errors.New("text must be nonempty valid UTF-8")
)

type Limits struct {
	MaxItems  int           `json:"maxItems"`
	MaxMemory int64         `json:"maxMemoryBytes"`
	MaxText   int64         `json:"maxTextBytes"`
	MaxImage  int64         `json:"maxImageBytes"`
	TTL       time.Duration `json:"-"`
}

func DefaultLimits() Limits { return Limits{100, 200 << 20, 1 << 20, 10 << 20, time.Hour} }

type Geometry struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (g Geometry) Valid() bool {
	return finite(g.X) && finite(g.Y) && math.Abs(g.X) <= 1e7 && math.Abs(g.Y) <= 1e7 &&
		finite(g.Width) && finite(g.Height) && g.Width >= 200 && g.Width <= 4096 && g.Height >= 140 && g.Height <= 4096
}
func finite(n float64) bool { return !math.IsInf(n, 0) && !math.IsNaN(n) }

type Item struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageID  string `json:"imageId,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Geometry
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revision  uint64    `json:"revision"`
	Size      int64     `json:"size"`
}

type Viewport struct {
	X                 float64   `json:"x"`
	Y                 float64   `json:"y"`
	Zoom              float64   `json:"zoom"`
	LastInteractionAt int64     `json:"lastInteractionAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type Snapshot struct {
	Items             []Item    `json:"items"`
	Viewport          Viewport  `json:"viewport"`
	LastCreatedItemID string    `json:"lastCreatedItemId"`
	LastCreatedAt     time.Time `json:"lastCreatedAt"`
	MemoryBytes       int64     `json:"memoryBytes"`
	Devices           int       `json:"devices"`
	Limits            Limits    `json:"limits"`
	ServerTime        time.Time `json:"serverTime"`
}

type Event struct {
	Type        string `json:"type"`
	Item        *Item  `json:"item,omitempty"`
	ID          string `json:"id,omitempty"`
	Devices     int    `json:"devices,omitempty"`
	MemoryBytes int64  `json:"memoryBytes"`
}

type Client struct{ Events chan Event }
type storedImage struct {
	data []byte
	mime string
}
type Store struct {
	mu       sync.Mutex
	items    map[string]Item
	images   map[string]storedImage
	clients  map[*Client]struct{}
	limits   Limits
	memory   int64
	viewport Viewport
	lastID   string
	lastAt   time.Time
	now      func() time.Time
}

func New(l Limits) *Store {
	return &Store{items: make(map[string]Item), images: make(map[string]storedImage), clients: make(map[*Client]struct{}), limits: l, viewport: Viewport{Zoom: 1}, now: time.Now}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (s *Store) AddText(text string, g Geometry) (Item, error) {
	if !utf8.ValidString(text) || len(text) == 0 {
		return Item{}, ErrText
	}
	if int64(len(text)) > s.limits.MaxText {
		return Item{}, ErrLimit
	}
	return s.add(Item{Type: "text", Text: text, Geometry: g, Size: int64(len(text))}, nil)
}

func (s *Store) AddImage(data []byte, mime string, g Geometry) (Item, error) {
	if int64(len(data)) > s.limits.MaxImage {
		return Item{}, ErrLimit
	}
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" {
		return Item{}, ErrImage
	}
	if http.DetectContentType(data) != mime {
		return Item{}, ErrImage
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return Item{}, ErrImage
	}
	// ヘッダーだけ正しい破損画像を各ブラウザへ配信しない。
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return Item{}, ErrImage
	}
	return s.add(Item{Type: "image", ImageID: newID(), MimeType: mime, Geometry: g, Size: int64(len(data))}, data)
}

func (s *Store) add(item Item, data []byte) (Item, error) {
	if !item.Geometry.Valid() {
		return Item{}, ErrGeometry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	if len(s.items) >= s.limits.MaxItems || s.memory+item.Size+1024 > s.limits.MaxMemory {
		return Item{}, ErrLimit
	}
	item.ID = newID()
	item.CreatedAt = s.now().UTC()
	item.UpdatedAt = item.CreatedAt
	item.ExpiresAt = item.CreatedAt.Add(s.limits.TTL)
	item.Revision = 1
	s.items[item.ID] = item
	if data != nil {
		s.images[item.ImageID] = storedImage{bytes.Clone(data), item.MimeType}
	}
	s.memory += item.Size + 1024
	s.lastID, s.lastAt = item.ID, item.CreatedAt
	s.broadcastLocked(Event{Type: "item.created", Item: &item})
	return item, nil
}

func (s *Store) Update(id string, g Geometry, revision uint64) (Item, error) {
	if !g.Valid() {
		return Item{}, ErrGeometry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	item, ok := s.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	if revision != item.Revision {
		return item, ErrConflict
	}
	item.Geometry, item.UpdatedAt, item.Revision = g, s.now().UTC(), item.Revision+1
	s.items[id] = item
	s.broadcastLocked(Event{Type: "item.updated", Item: &item})
	return item, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return ErrNotFound
	}
	s.deleteLocked(id, "item.deleted")
	return nil
}

func (s *Store) deleteLocked(id, event string) {
	item := s.items[id]
	s.memory -= item.Size + 1024
	delete(s.images, item.ImageID)
	delete(s.items, id)
	if id == s.lastID {
		s.lastID, s.lastAt = "", time.Time{}
		for _, candidate := range s.items {
			if candidate.CreatedAt.After(s.lastAt) {
				s.lastID, s.lastAt = candidate.ID, candidate.CreatedAt
			}
		}
	}
	s.broadcastLocked(Event{Type: event, ID: id})
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items, s.images = make(map[string]Item), make(map[string]storedImage)
	s.memory, s.lastID, s.lastAt = 0, "", time.Time{}
	s.broadcastLocked(Event{Type: "board.cleared"})
}

func (s *Store) Expire() { s.mu.Lock(); defer s.mu.Unlock(); s.expireLocked() }
func (s *Store) expireLocked() {
	for id, item := range s.items {
		if !s.now().Before(item.ExpiresAt) {
			s.deleteLocked(id, "item.expired")
		}
	}
}

func (s *Store) Image(id string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	im, ok := s.images[id]
	return im.data, im.mime, ok
}

func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	return s.snapshotLocked()
}
func (s *Store) snapshotLocked() Snapshot {
	items := make([]Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return Snapshot{items, s.viewport, s.lastID, s.lastAt, s.memory, len(s.clients), s.limits, s.now().UTC()}
}

func (s *Store) SetViewport(v Viewport) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !finite(v.X) || !finite(v.Y) || math.Abs(v.X) > 1e7 || math.Abs(v.Y) > 1e7 || !finite(v.Zoom) || v.Zoom < .2 || v.Zoom > 3 || v.LastInteractionAt <= s.viewport.LastInteractionAt || v.LastInteractionAt > s.now().Add(5*time.Second).UnixMilli() {
		return false
	}
	v.UpdatedAt = s.now().UTC()
	s.viewport = v
	return true
}

func (s *Store) Subscribe() (*Client, Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	if len(s.clients) >= 64 {
		return nil, Snapshot{}
	}
	c := &Client{make(chan Event, 128)}
	s.clients[c] = struct{}{}
	s.broadcastLocked(Event{Type: "devices.updated", Devices: len(s.clients)})
	return c, s.snapshotLocked()
}

func (s *Store) Unsubscribe(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		close(c.Events)
	}
	s.broadcastLocked(Event{Type: "devices.updated", Devices: len(s.clients)})
}

func (s *Store) broadcastLocked(e Event) {
	e.MemoryBytes = s.memory
	for c := range s.clients {
		select {
		case c.Events <- e:
		default:
			// 遅い接続を無制限に蓄積せず、再接続時に完全な状態を取得させる。
			delete(s.clients, c)
			close(c.Events)
		}
	}
}
