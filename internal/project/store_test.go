package project

import (
	"os"
	"path/filepath"
	"testing"

	"ouroboros/internal/scope"
)

func TestStoreSaveLoadList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	rules := []scope.Rule{
		{Kind: scope.RuleKindHost, Pattern: "*.example.com", MatchMode: scope.MatchModeWildcard, Action: scope.ActionInclude, Enabled: true, Priority: 10},
		{Kind: scope.RuleKindPath, Pattern: "/admin", MatchMode: scope.MatchModeLiteral, Action: scope.ActionExclude, Enabled: true, Priority: 5},
	}

	if err := s.Save("test-project", rules); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load("test-project")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d rules, want 2", len(loaded))
	}
	if loaded[0].Pattern != "*.example.com" {
		t.Fatalf("rule[0] pattern = %q", loaded[0].Pattern)
	}

	names, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 1 || names[0] != "test-project" {
		t.Fatalf("names = %v, want [test-project]", names)
	}

	// Save a second project.
	if err := s.Save("alpha", rules); err != nil {
		t.Fatalf("Save alpha: %v", err)
	}
	names, _ = s.List()
	if len(names) != 2 {
		t.Fatalf("names = %v, want 2", names)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	_, err := s.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error loading nonexistent project")
	}
}

func TestStoreValidateName(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)

	if err := s.Save("../escaped", nil); err == nil {
		t.Fatal("expected error for path traversal name")
	}
	if err := s.Save("a.b", nil); err == nil {
		t.Fatal("expected error for name with dots")
	}
}

func TestStoreDefaultDir(t *testing.T) {
	// Just verify NewStore("") works without error on a real system.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	s, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	expected := filepath.Join(tmpHome, ".config", "ouroboros", "projects")
	if s.Dir() != expected {
		t.Fatalf("dir = %q, want %q", s.Dir(), expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
}