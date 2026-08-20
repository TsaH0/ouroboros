// Package skill installs Ouroboros Advisor skill for local AI agents.
package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*
var assets embed.FS

// Install writes skill files to dir. Existing files are replaced by current version.
func Install(dir string) error {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".agents", "skills", "ouroboros-advisor")
	}
	return fs.WalkDir(assets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := assets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		rel, err := filepath.Rel("assets", path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
		}
		mode := os.FileMode(0600)
		if filepath.Base(rel) == "query.sh" {
			mode = 0700
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		return nil
	})
}
