//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two hard links to one draft are one draft: check lists it once. apply
// refuses it with exit 2 and sends nothing, because writing card: back by
// rename would split the links.
func TestDraftHardLinks(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	writeFile(t, a, "---\nboard: Home\n---\n# Fix the gate\n")
	b := filepath.Join(dir, "b.md")
	if err := os.Link(a, b); err != nil {
		t.Fatal(err)
	}
	got := syncRun(t, srv, "draft", "check", a, b)
	if got.code != 0 || strings.Count(got.stdout, "✓") != 1 {
		t.Errorf("check: code %d stdout %q", got.code, got.stdout)
	}
	got = syncRun(t, srv, "draft", "apply", a, b)
	if got.code != 2 || !strings.Contains(got.stderr, "hard link") {
		t.Errorf("apply: code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}
