package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// A sync that stops partway deletes synced-draft:, so the next --update
// sends the title and description even when the draft was reverted to its
// text at the last full sync.
func TestUpdatePartialThenRevertSends(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	if r := syncRun(t, srv, "draft", "apply", "--update", "--force", path); r.code != 0 {
		t.Fatalf("full sync: %+v", r)
	}
	text := syncReadDraft(t, path)
	if !strings.Contains(text, "\nsynced-draft: ") {
		t.Fatalf("no synced-draft line after a full sync:\n%s", text)
	}
	writeFile(t, path, strings.Replace(text, "# Fix the gate", "# Fix the gate NOW", 1))
	srv.mu.Lock()
	srv.afterWrite = map[string]func(*cardState){"PUT /1/cards/87": func(c *cardState) { c.title = "Fix the gate NOW" }}
	srv.mu.Unlock()
	srv.failNext("POST /1/cards/87/steps", 1)
	if r := syncRun(t, srv, "draft", "apply", "--update", path); r.code != 1 {
		t.Fatalf("partial sync: want exit 1, got %+v", r)
	}
	srv.mu.Lock()
	srv.afterWrite = nil
	srv.mu.Unlock()
	text = syncReadDraft(t, path)
	if strings.Contains(text, "\nsynced-draft: ") {
		t.Errorf("partial sync kept synced-draft:\n%s", text)
	}
	writeFile(t, path, strings.Replace(text, "# Fix the gate NOW", "# Fix the gate", 1))
	from := len(srv.writes())
	r := syncRun(t, srv, "draft", "apply", "--update", path)
	if r.code != 0 {
		t.Fatalf("retry: %+v", r)
	}
	if !strings.Contains(r.stdout, "#87 title and description sent\n") || countExact(srv.writes()[from:], "PUT /1/cards/87") != 1 {
		t.Errorf("reverted draft after a partial sync was skipped: writes %q, stdout %q", srv.writes()[from:], r.stdout)
	}
}

// A title and description PUT whose outcome is unknown also deletes
// synced-draft:.
func TestUpdateFailedPutDropsSyncedDraft(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	if r := syncRun(t, srv, "draft", "apply", "--update", "--force", path); r.code != 0 {
		t.Fatalf("full sync: %+v", r)
	}
	text := syncReadDraft(t, path)
	writeFile(t, path, strings.Replace(text, "# Fix the gate", "# Fix the gate NOW", 1))
	srv.failNext("PUT /1/cards/87", 1)
	if r := syncRun(t, srv, "draft", "apply", "--update", path); r.code != 1 {
		t.Fatalf("failed PUT: want exit 1, got %+v", r)
	}
	if text := syncReadDraft(t, path); strings.Contains(text, "\nsynced-draft: ") {
		t.Errorf("failed PUT kept synced-draft:\n%s", text)
	}
}

// countExact counts the calls equal to call.
func countExact(calls []string, call string) int {
	n := 0
	for _, c := range calls {
		if c == call {
			n++
		}
	}
	return n
}
