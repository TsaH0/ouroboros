// Package skill installs Ouroboros Advisor skill for local AI agents.
package skill

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/SKILL.md
var skillDoc []byte

//go:embed assets/query.sh
var queryScript []byte

// Install writes skill files to dir. Existing files are replaced by current version.
func Install(dir string) error {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".agents", "skills", "ouroboros-advisor")
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0700); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), skillDoc, 0600); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "query.sh"), queryScript, 0700); err != nil {
		return fmt.Errorf("write query.sh: %w", err)
	}
	fmt.Printf("Installed Ouroboros Advisor skill: %s\n", dir)
	fmt.Printf("Use: bash %s/scripts/query.sh overview\n", dir)
	return nil
}
