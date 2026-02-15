package agent

import (
	"sync"
	"testing"
)

func TestStore_AddAndGet(t *testing.T) {
	s := NewStore()
	a := NewAgent("test", "feat/x", "main", "/wt", "@1", "%0")
	a.ID = "custom-id"

	s.Add(a)
	got, ok := s.Get("custom-id")
	if !ok {
		t.Fatal("Get returned not-ok for added agent")
	}
	if got != a {
		t.Error("Get returned different agent pointer")
	}
}

func TestStore_Add_AutoID(t *testing.T) {
	s := NewStore()
	a1 := NewAgent("a1", "b1", "main", "/wt1", "@1", "%0")
	a2 := NewAgent("a2", "b2", "main", "/wt2", "@2", "%1")

	s.Add(a1)
	s.Add(a2)

	if a1.ID != "a1" {
		t.Errorf("first auto ID = %q, want %q", a1.ID, "a1")
	}
	if a2.ID != "a2" {
		t.Errorf("second auto ID = %q, want %q", a2.ID, "a2")
	}
}

func TestStore_Add_PresetID(t *testing.T) {
	s := NewStore()
	a := NewAgent("test", "feat/x", "main", "/wt", "@1", "%0")
	a.ID = "preset"

	s.Add(a)
	if a.ID != "preset" {
		t.Errorf("preset ID changed to %q", a.ID)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := NewStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get should return false for missing agent")
	}
}

func TestStore_All(t *testing.T) {
	s := NewStore()
	a1 := NewAgent("a1", "b1", "main", "/wt1", "@1", "%0")
	a2 := NewAgent("a2", "b2", "main", "/wt2", "@2", "%1")
	s.Add(a1)
	s.Add(a2)

	all := s.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d agents, want 2", len(all))
	}
}

func TestStore_All_Empty(t *testing.T) {
	s := NewStore()
	all := s.All()
	if len(all) != 0 {
		t.Errorf("All() returned %d agents, want 0", len(all))
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	s := NewStore()
	a := NewAgent("test", "feat/x", "main", "/wt", "@1", "%0")
	s.Add(a)

	ok := s.UpdateStatus(a.ID, StatusReviewReady)
	if !ok {
		t.Error("UpdateStatus returned false for existing agent")
	}
	if a.GetStatus() != StatusReviewReady {
		t.Errorf("status = %q, want %q", a.GetStatus(), StatusReviewReady)
	}
}

func TestStore_UpdateStatus_NotFound(t *testing.T) {
	s := NewStore()
	ok := s.UpdateStatus("nonexistent", StatusDone)
	if ok {
		t.Error("UpdateStatus should return false for missing agent")
	}
}

func TestStore_Remove(t *testing.T) {
	s := NewStore()
	a := NewAgent("test", "feat/x", "main", "/wt", "@1", "%0")
	s.Add(a)

	s.Remove(a.ID)
	_, ok := s.Get(a.ID)
	if ok {
		t.Error("Get should return false after Remove")
	}
	if len(s.All()) != 0 {
		t.Error("All() should return empty after Remove")
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := NewAgent("test", "b", "main", "/wt", "@1", "%0")
			s.Add(a)
			s.Get(a.ID)
			s.All()
			s.UpdateStatus(a.ID, StatusDone)
			s.Remove(a.ID)
		}()
	}
	wg.Wait()
}
