package cli

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Two applies of one draft at the same time make one card.
func TestApplyConcurrentMakesOneCard(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	var wg sync.WaitGroup
	results := make([]result, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runGo([]string{"draft", "apply", path})
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt64(creates); n != 1 {
		t.Errorf("%d creates, want 1: %+v", n, results)
	}
}

// check and preview show a creating: line left by an unknown create
// outcome. check exits 1 for it, so the problem shows before apply.
func TestCheckAndPreviewShowCreating(t *testing.T) {
	srv := newSyncServer(t)
	_, drafts := imageEnv(t, srv)
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\ncreating: 2026-10-18T00:00:00Z\n---\n# Fix the gate\n")
	want := "creating: pending since 2026-10-18T00:00:00Z; resolve before apply\n"
	r := runGo([]string{"draft", "check", p})
	if r.code != 1 || !strings.Contains(r.stdout, want) {
		t.Errorf("check: %+v", r)
	}
	r = runGo([]string{"draft", "preview", p})
	if r.code != 0 || !strings.Contains(r.stdout, want) {
		t.Errorf("preview: %+v", r)
	}
}

// A draft that already has card: is skipped without --update, so its
// images are not checked: a moved image file does not block the new draft
// beside it.
func TestApplySkippedDraftImagesNotChecked(t *testing.T) {
	srv := newSyncServer(t)
	_, drafts := imageEnv(t, srv)
	writeFile(t, filepath.Join(drafts, "a.md"), "---\nboard: Home\ncard: 87\n---\n# Fix the gate\n\n![x](gone.png)\n")
	writeFile(t, filepath.Join(drafts, "b.md"), "---\nboard: Home\n---\n# Oil the hinge\n")
	r := runGo([]string{"draft", "apply", drafts})
	if r.code != 0 || !strings.Contains(r.stdout, "skip a.md: already card #87") || !strings.Contains(r.stdout, "#90 Oil the hinge") {
		t.Errorf("got %+v", r)
	}
}
