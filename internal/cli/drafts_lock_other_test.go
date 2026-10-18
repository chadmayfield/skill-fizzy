//go:build !unix || solaris || aix

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Without flock, an apply holds .<name>.fzlock beside each draft it may
// write, created with O_EXCL, and removes it at the end. A second apply while the lock
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
