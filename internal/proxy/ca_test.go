package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateCA_MigratesLegacySentinelCA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, ".config", "sentinel")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("create legacy directory: %v", err)
	}
	legacyCA, err := generateCA()
	if err != nil {
		t.Fatalf("generate legacy CA: %v", err)
	}
	if err := saveCAToDisk(legacyCA, filepath.Join(legacyDir, "ca.pem"), filepath.Join(legacyDir, "ca-key.pem")); err != nil {
		t.Fatalf("save legacy CA: %v", err)
	}

	loaded, err := LoadOrGenerateCA()
	if err != nil {
		t.Fatalf("load migrated CA: %v", err)
	}
	if string(loaded.X509.Raw) != string(legacyCA.X509.Raw) {
		t.Fatal("migration generated or selected a different CA certificate")
	}

	ouroborosDir := filepath.Join(home, ".config", "ouroboros")
	for _, name := range []string{"ca.pem", "ca-key.pem"} {
		if _, err := os.Stat(filepath.Join(ouroborosDir, name)); err != nil {
			t.Fatalf("migrated %s missing: %v", name, err)
		}
	}
}
