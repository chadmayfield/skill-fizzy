package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// An apply holds .<name>.fzlock beside each draft it may write, created
// with O_EXCL, and removes it at the end. A second apply while the lock
// exists refuses with exit 2, names the lock, says a stale one is the
// user's to delete, and sends nothing.
func TestApplyLock(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	lock := filepath.Join(dir, ".gate.md.fzlock")
	writeFile(t, lock, "")
	r := runGo([]string{"draft", "apply", path})
	if r.code != 2 || !strings.Contains(r.stderr, lock) || !strings.Contains(r.stderr, "delete") {
		t.Errorf("held lock: %+v", r)
	}
	if n := atomic.LoadInt64(creates); n != 0 {
		t.Errorf("%d creates with the lock held", n)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("fz removed a lock it did not take: %v", err)
	}
	os.Remove(lock)
	r = runGo([]string{"draft", "apply", path})
	if r.code != 0 || atomic.LoadInt64(creates) != 1 {
		t.Errorf("apply: %+v", r)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock is still there: %v", err)
	}
}

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
