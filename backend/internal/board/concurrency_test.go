package board

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrentCreatesRespectCountAndMemoryLimit(t *testing.T) {
	l := DefaultLimits()
	l.MaxItems = 32
	s := New(l)
	var tasks sync.WaitGroup
	for i := 0; i < 100; i++ {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			if _, err := s.AddText("concurrent", Geometry{Width: 320, Height: 240}); err != nil && err != ErrLimit {
				t.Errorf("create: %v", err)
			}
		}()
	}
	tasks.Wait()
	state := s.Snapshot()
	if len(state.Items) != 32 || state.MemoryBytes > l.MaxMemory {
		t.Fatal("concurrent operations exceeded a limit")
	}
}

func TestSlowClientIsClosedAndReconnectReceivesCompleteSnapshot(t *testing.T) {
	s := New(DefaultLimits())
	c, _ := s.Subscribe()
	item, err := s.AddText("latest", Geometry{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		item, err = s.Update(item.ID, Geometry{X: float64(i), Width: 320, Height: 240}, item.Revision)
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.Snapshot().Devices != 0 {
		t.Fatal("slow connection retained")
	}
	for range c.Events {
	}
	connected, snapshot := s.Subscribe()
	defer s.Unsubscribe(connected)
	if len(snapshot.Items) != 1 || snapshot.Items[0].Revision != 151 || snapshot.Items[0].X != 149 {
		t.Fatal("reconnect did not receive latest state")
	}
}

func TestImageExpiryReleasesBytesAndNotifiesClient(t *testing.T) {
	s := New(DefaultLimits())
	now := time.Now()
	s.now = func() time.Time { return now }
	c, _ := s.Subscribe()
	defer s.Unsubscribe(c)
	item, err := s.AddImage(pngBytes(t), "image/png", Geometry{Width: 320, Height: 240})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	s.Expire()
	if _, _, exists := s.Image(item.ImageID); exists || s.Snapshot().MemoryBytes != 0 {
		t.Fatal("expiry retained image")
	}
	for i := 0; i < 3; i++ {
		event := <-c.Events
		if event.Type == "item.expired" {
			if event.ID != item.ID {
				t.Fatal("wrong expiry notification")
			}
			return
		}
	}
	t.Fatal("expiry event missing")
}
