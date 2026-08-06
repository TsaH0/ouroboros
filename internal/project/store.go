package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ouroboros/internal/scope"
)

// Store saves and loads named scope rule sets (projects) as JSON files
// in a dedicated directory (default: ~/.config/ouroboros/projects/).
type Store struct {
	dir string
}

// NewStore creates a project store rooted at dir.
// If dir is empty it defaults to ~/.config/ouroboros/projects.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}
		dir = filepath.Join(home, ".config", "ouroboros", "projects")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create projects dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the directory backing the store.
func (s *Store) Dir() string { return s.dir }

// Save writes the given rules as <name>.json inside the store directory.
// If name is empty, the current active project name is used; if none,
// "default" is assumed.
func (s *Store) Save(name string, rules []scope.Rule) error {
	if name == "" {
		name = "default"
	}
	if err := validateName(name); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	path := s.path(name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write project file: %w", err)
	}
	return nil
}

// Load reads <name>.json and returns the scope rules.
func (s *Store) Load(name string) ([]scope.Rule, error) {
	if name == "" {
		name = "default"
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	path := s.path(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project %q: %w", name, err)
	}
	var rules []scope.Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse project %q: %w", name, err)
	}
	return rules, nil
}

// List returns all saved project names (file stems, sorted).
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// validateName prevents path traversal and weird filenames.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("project name is empty")
	}
	if strings.ContainsAny(name, `/\..`) {
		return fmt.Errorf("project name %q must not contain / or dots", name)
	}
	return nil
}