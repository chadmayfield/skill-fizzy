package cli

import (
	"path/filepath"
	"testing"
)

// draft preview reads the config file for the inbox, so a config file fz
// cannot use stops it with exit 1. A missing config file is fine.
func TestDraftPreviewReadsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	t.Setenv("FIZZY_CONFIG", filepath.Join(dir, "missing.json"))
	if r := runGo([]string{"draft", "preview", path}); r.code != 0 {
		t.Errorf("no config file: %+v", r)
	}
	bad := filepath.Join(dir, "config.json")
	writeFile(t, bad, "{bad")
	t.Setenv("FIZZY_CONFIG", bad)
	if r := runGo([]string{"draft", "preview", path}); r.code != 1 || r.stdout != "" {
		t.Errorf("broken config file: %+v", r)
	}
}
