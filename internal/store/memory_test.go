package store

import (
	"context"
	"sync"
	"testing"

	"github.com/TsaH0/ouroboros/internal/model"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	f := &model.Flow{ID: "test-1", Host: "example.com"}
	if err := s.SaveFlow(ctx, f); err != nil {
		t.Fatalf("SaveFlow: %v", err)
	}

	got, err := s.GetFlow(ctx, "test-1")
	if err != nil {
		t.Fatalf("GetFlow: %v", err)
	}
	if got == nil {
		t.Fatal("GetFlow returned nil")
	}
	if got.Host != "example.com" {
		t.Fatalf("expected host example.com, got %s", got.Host)
	}
}

func TestMemoryStore_GetMissing(t *testing.T) {
	s := NewMemoryStore()
	got, err := s.GetFlow(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetFlow: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing flow")
	}
}

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		f := &model.Flow{ID: string(rune('A' + i))}
		if err := s.SaveFlow(ctx, f); err != nil {
			t.Fatalf("SaveFlow: %v", err)
		}
	}

	flows, err := s.ListFlows(ctx)
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 5 {
		t.Fatalf("expected 5 flows, got %d", len(flows))
	}
}

func TestMemoryStore_ConcurrentSafety(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			f := &model.Flow{ID: string(rune(n))}
			_ = s.SaveFlow(ctx, f)
		}(i)
	}
	wg.Wait()

	flows, err := s.ListFlows(ctx)
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 100 {
		t.Fatalf("expected 100 flows, got %d", len(flows))
	}
}
