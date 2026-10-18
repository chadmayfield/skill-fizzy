//go:build unix && !solaris && !aix

package cli

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// lockHelperEnv names the draft TestLockHelperProcess locks.
const lockHelperEnv = "FZ_TEST_LOCK_HELPER"

// TestLockHelperProcess is not a test. Run as a child with lockHelperEnv
// set, it takes the apply lock of that draft, prints "locked" and waits to
// be killed.
func TestLockHelperProcess(t *testing.T) {
	path := os.Getenv(lockHelperEnv)
	if path == "" {
		t.Skip("helper process only")
	}
	if _, err := lockDrafts([]string{path}); err != nil {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(3)
	}
	os.Stdout.WriteString("locked\n")
	time.Sleep(time.Minute)
	os.Exit(4)
}

// startLockHolder starts a child process that holds the apply lock of path
// and returns it once the lock is taken.
func startLockHolder(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(), lockHelperEnv+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("lock holder: %q, %v", line, err)
	}
	return cmd
}

// A lock left by an apply that was killed does not block the next apply:
// the kernel dropped the flock when the process died.
func TestApplyLockFreedWhenHolderKilled(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	lock := filepath.Join(dir, ".gate.md.fzlock")
	cmd := startLockHolder(t, path)
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("the holder made no lock file: %v", err)
	}
	// A live holder blocks the apply.
	if r := runGo([]string{"draft", "apply", path}); r.code != 2 || !strings.Contains(r.stderr, lock) {
		t.Errorf("live holder: %+v", r)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("SIGKILL should leave the lock file behind: %v", err)
	}
	r := runGo([]string{"draft", "apply", path})
	if r.code != 0 || atomic.LoadInt64(creates) != 1 {
		t.Errorf("apply after the holder died: %+v", r)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock file is still there after a normal run: %v", err)
	}
}

// Two live applies of one draft at the same time: the second refuses with
// exit 2, names the lock and sends nothing. The first finishes and removes
// the lock file.
func TestApplyLockSecondLiveApplyRefused(t *testing.T) {
	inCreate := make(chan struct{})
	release := make(chan struct{})
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		close(inCreate)
		<-release
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	lock := filepath.Join(dir, ".gate.md.fzlock")
	first := make(chan result, 1)
	go func() { first <- runGo([]string{"draft", "apply", path}) }()
	select {
	case <-inCreate:
	case <-time.After(10 * time.Second):
		t.Fatal("the first apply never sent its create")
	}
	second := runGo([]string{"draft", "apply", path})
	close(release)
	r := <-first
	if second.code != 2 || !strings.Contains(second.stderr, lock) {
		t.Errorf("second apply: %+v", second)
	}
	if r.code != 0 || atomic.LoadInt64(creates) != 1 {
		t.Errorf("first apply: %+v, %d creates", r, atomic.LoadInt64(creates))
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock file is still there: %v", err)
	}
}

// A symlink planted at the lock path is refused, never followed.
func TestApplyLockRefusesSymlink(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Symlink(target, filepath.Join(dir, ".gate.md.fzlock")); err != nil {
		t.Fatal(err)
	}
	r := runGo([]string{"draft", "apply", path})
	if r.code != 2 || atomic.LoadInt64(creates) != 0 {
		t.Errorf("symlinked lock: %+v", r)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("fz followed the symlink: %v", err)
	}
}

// A lock file that no process holds, such as one left by a crash, does not
// block the apply, and the apply removes it at the end.
func TestApplyLockStaleFileIgnored(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	lock := filepath.Join(dir, ".gate.md.fzlock")
	writeFile(t, lock, "12345\n")
	r := runGo([]string{"draft", "apply", path})
	if r.code != 0 || atomic.LoadInt64(creates) != 1 {
		t.Errorf("stale lock file: %+v", r)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock file is still there: %v", err)
	}
}
