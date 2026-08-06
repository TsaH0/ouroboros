package store

import (
	"context"
	"sync"
	"testing"

	"sentinel/internal/model"
)

func TestInMemoryFlowStore_SaveAndGet(t *testing.T) {
	s := NewInMemoryFlowStore()
	ctx := context.Background()

	f := &model.Flow{ID: "test-1", Host: "example.com"}
	if err := s.Save(ctx, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get(ctx, "test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Host != "example.com" {
		t.Fatalf("expected host example.com, got %s", got.Host)
	}
}

func TestInMemoryFlowStore_GetMissing(t *testing.T) {
	s := NewInMemoryFlowStore()
	got, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing flow")
	}
}

func TestInMemoryFlowStore_List(t *testing.T) {
	s := NewInMemoryFlowStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		f := &model.Flow{ID: string(rune('A' + i))}
		if err := s.Save(ctx, f); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	flows, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(flows) != 5 {
		t.Fatalf("expected 5 flows, got %d", len(flows))
	}
}

func TestInMemoryFlowStore_ConcurrentSafety(t *testing.T) {
	s := NewInMemoryFlowStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			f := &model.Flow{ID: string(rune(n))}
			_ = s.Save(ctx, f)
		}(i)
	}
	wg.Wait()

	flows, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(flows) != 100 {
		t.Fatalf("expected 100 flows, got %d", len(flows))
	}
}
