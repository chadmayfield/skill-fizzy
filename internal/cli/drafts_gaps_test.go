package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --update on a read-only draft is refused before any write, because fz
// could not record synced: afterwards. (Mutant 23 in opus-integrity.)
func TestApplyUpdateRefusesUnwritableDraft(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 1 || !strings.Contains(got.stderr, "cannot write card:") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}
