package memory

import (
	"testing"
	"time"
)

func TestMemoryAddAndRecent(t *testing.T) {
	s := NewMemoryStoreWithWorkspace(t.TempDir(), 3)
	s.AddLong("L1")
	s.AddShort("two")
	s.AddShort("one")

	res := s.Recent(10)
	if len(res) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res))
	}
	if res[0].Text != "one" || res[1].Text != "two" || res[2].Text != "L1" {
		t.Fatalf("unexpected recent order: %v", res)
	}
}

func TestShortLimit(t *testing.T) {
	s := NewMemoryStoreWithWorkspace(t.TempDir(), 2)
	s.AddShort("c")
	time.Sleep(5 * time.Millisecond)
	s.AddShort("b")
	time.Sleep(5 * time.Millisecond)
	s.AddShort("a")

	res := s.Recent(10)
	if len(res) != 2 {
		t.Fatalf("expected 2 items due to limit, got %d", len(res))
	}
	if res[0].Text != "a" || res[1].Text != "b" {
		t.Fatalf("unexpected recent after limit: %v", res)
	}
}

func TestQueryByKeyword(t *testing.T) {
	s := NewMemoryStoreWithWorkspace(t.TempDir(), 10)
	s.AddLong("apple pie recipe")
	s.AddShort("Remember the apple")

	res := s.QueryByKeyword("apple", 10)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Text != "Remember the apple" || res[1].Text != "apple pie recipe" {
		t.Fatalf("unexpected query order: %v", res)
	}
}
