package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)


// A closed card is never moved, even when the draft names a column.
func TestSyncLeavesClosedCardColumn(t *testing.T) {
	text := strings.Replace(syncDraftText, "column: Maybe?\n", "column: Not now\n", 1)
	srv := newSyncServer(t)
	srv.edit(func(c *cardState) { c.closed = true })
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, text))
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	for _, w := range srv.writes() {
		if strings.HasSuffix(w, "/not_now") || strings.HasSuffix(w, "/triage") || strings.HasSuffix(w, "/closure") {
			t.Errorf("moved a closed card: %q", w)
		}
	}
	if !strings.Contains(got.stdout, "#87 card is closed; column left alone\n") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

// A draft without golden turns a golden card's mark off.
func TestSyncTurnsGoldenOff(t *testing.T) {
	srv := newSyncServer(t)
	srv.edit(func(c *cardState) { c.golden = true })
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if !syncHas(srv.writes(), "DELETE /1/cards/87/goldness") || !strings.Contains(got.stdout, "#87 golden off\n") {
		t.Errorf("writes %q stdout %q", srv.writes(), got.stdout)
	}
}

// A missing image in a later draft stops the batch before the first
// draft's card is created.
func TestApplyChecksEveryImageFirst(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\nboard: Home\n---\n# Fix the gate\n")
	writeFile(t, filepath.Join(dir, "b.md"), "---\nboard: Home\n---\n# Oil the hinge\n\n![the hinge](missing.jpg)\n")
	got := syncRun(t, srv, "draft", "apply", dir)
	if got.code != 1 || !strings.Contains(got.stderr, "image not found: missing.jpg") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
	if text, _ := os.ReadFile(filepath.Join(dir, "a.md")); strings.Contains(string(text), "card:") {
		t.Errorf("a.md was changed:\n%s", text)
	}
}
