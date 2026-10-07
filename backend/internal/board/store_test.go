package board

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestTextLifecycleAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	s := New(Limits{MaxItems: 100, MaxMemory: 200 << 20, MaxText: 1 << 20, MaxImage: 10 << 20, TTL: time.Hour})
	s.now = func() time.Time { return now }
	c, snapshot := s.Subscribe()
	defer s.Unsubscribe(c)
	if len(snapshot.Items) != 0 {
		t.Fatal("initial board is not empty")
	}
	if event := <-c.Events; event.Type != "devices.updated" {
		t.Fatal("device event missing")
	}
	item, err := s.AddText("日本語 <script>plain text</script>", Geometry{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	if item.ExpiresAt.Sub(item.CreatedAt) != time.Hour {
		t.Fatal("TTL is not one hour")
	}
	if event := <-c.Events; event.Type != "item.created" {
		t.Fatalf("event: %s", event.Type)
	}
	updated, err := s.Update(item.ID, Geometry{X: 20, Y: 30, Width: 400, Height: 260}, item.Revision)
	if err != nil || updated.X != 20 {
		t.Fatalf("update: %v", err)
	}
	if _, err := s.Update(item.ID, item.Geometry, item.Revision); err != ErrConflict {
		t.Fatalf("stale revision: %v", err)
	}
	now = now.Add(time.Hour)
	s.Expire()
	for {
		event := <-c.Events
		if event.Type == "item.expired" {
			if event.ID != item.ID {
				t.Fatal("wrong item expired")
			}
			break
		}
	}
	if len(s.Snapshot().Items) != 0 || s.Snapshot().MemoryBytes != 0 {
		t.Fatal("expired text remains in memory")
	}
	if err := s.Delete(item.ID); err != ErrNotFound {
		t.Fatalf("deleted item: %v", err)
	}
}

func TestImageDeletionAndClearReleaseBytes(t *testing.T) {
	s := New(DefaultLimits())
	data := pngBytes(t)
	item, err := s.AddImage(data, "image/png", Geometry{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 0
	stored, mime, ok := s.Image(item.ImageID)
	if !ok || mime != "image/png" || stored[0] != 137 {
		t.Fatal("image is not independently stored")
	}
	if err := s.Delete(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := s.Image(item.ImageID); ok {
		t.Fatal("deleted image remains")
	}
	item, _ = s.AddImage(pngBytes(t), "image/png", Geometry{Width: 320, Height: 240})
	s.Clear()
	if _, _, ok := s.Image(item.ImageID); ok || s.Snapshot().MemoryBytes != 0 {
		t.Fatal("clear retains image bytes")
	}
}

func TestLimitsAndMIME(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxItems = 1
	limits.MaxText = 3
	s := New(limits)
	geometry := Geometry{Width: 320, Height: 240}
	if _, err := s.AddText("long", geometry); err != ErrLimit {
		t.Fatalf("text limit: %v", err)
	}
	if _, err := s.AddText("ok", geometry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddText("two", geometry); err != ErrLimit {
		t.Fatalf("count limit: %v", err)
	}
	s.Clear()
	s.limits.MaxMemory = 1
	if _, err := s.AddText("ok", geometry); err != ErrLimit {
		t.Fatalf("memory limit: %v", err)
	}
	s.limits = DefaultLimits()
	if _, err := s.AddImage([]byte("<svg/>"), "image/svg+xml", geometry); err != ErrImage {
		t.Fatalf("svg: %v", err)
	}
	if _, err := s.AddImage(pngBytes(t), "image/jpeg", geometry); err != ErrImage {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := s.AddText("ok", Geometry{Width: -1, Height: 1}); err != ErrGeometry {
		t.Fatalf("geometry: %v", err)
	}
}

func TestViewportUsesLastInteractionAndRestartIsEmpty(t *testing.T) {
	s := New(DefaultLimits())
	now := time.Now().UnixMilli()
	if !s.SetViewport(Viewport{X: 30, Zoom: 1, LastInteractionAt: now}) {
		t.Fatal("first viewport rejected")
	}
	if s.SetViewport(Viewport{X: 99, Zoom: 1, LastInteractionAt: now - 1}) {
		t.Fatal("older viewport won")
	}
	if s.SetViewport(Viewport{Zoom: 1, LastInteractionAt: now + 3600000}) {
		t.Fatal("future viewport accepted")
	}
	if s.Snapshot().Viewport.X != 30 {
		t.Fatal("viewport changed")
	}
	if len(New(DefaultLimits()).Snapshot().Items) != 0 {
		t.Fatal("new store restored data")
	}
}
