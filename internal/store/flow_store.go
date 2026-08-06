package store

import (
	"context"
	"sync"

	"ouroboros/internal/model"
)

// FlowStore is the persistence abstraction for HTTP flows.
type FlowStore interface {
	Save(ctx context.Context, flow *model.Flow) error
	Get(ctx context.Context, id string) (*model.Flow, error)
	List(ctx context.Context) ([]*model.Flow, error)
}

// InMemoryFlowStore is a concurrency-safe in-memory implementation of FlowStore.
type InMemoryFlowStore struct {
	mu    sync.RWMutex
	flows []*model.Flow
	byID  map[string]*model.Flow
}

func NewInMemoryFlowStore() *InMemoryFlowStore {
	return &InMemoryFlowStore{
		byID: make(map[string]*model.Flow),
	}
}

func (s *InMemoryFlowStore) Save(_ context.Context, flow *model.Flow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[flow.ID] = flow
	s.flows = append(s.flows, flow)
	return nil
}

func (s *InMemoryFlowStore) Get(_ context.Context, id string) (*model.Flow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.byID[id]
	if !ok {
		return nil, nil
	}
	return f, nil
}

func (s *InMemoryFlowStore) List(_ context.Context) ([]*model.Flow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Return a copy to avoid data races on the slice.
	result := make([]*model.Flow, len(s.flows))
	copy(result, s.flows)
	return result, nil
}
