//go:build unix

package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// hungTokenCmd is a token command that never prints, with a child of its
// own. Both write their pids to pidFile.
func hungTokenCmd(pidFile string) string {
	return `sh -c 'echo $$ > ` + pidFile + `; sleep 30 & echo $! >> ` + pidFile + `; sleep 30'`
}

// alivePids returns the pids in pidFile that still run. It fails the test
// when the file does not hold two pids.
func alivePids(t *testing.T, pidFile string) []int {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	fields := strings.Fields(string(data))
	if err != nil || len(fields) != 2 {
		t.Fatalf("pid file %q: %v", data, err)
	}
	var alive []int
	for _, f := range fields {
		pid, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("pid %q: %v", f, err)
		}
		if err := syscall.Kill(pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
			alive = append(alive, pid)
		}
	}
	return alive
}

// When the caller's context ends, ResolveTokenContext kills the token
// command and every process it started, not only the first one.
func TestResolveTokenContextKillsTheGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pids")
	c := &Config{TokenCmd: hungTokenCmd(pidFile)}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.ResolveTokenContext(ctx)
	if err == nil || !errors.Is(err, ErrConfig) {
		t.Errorf("err = %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
	time.Sleep(time.Second)
	if alive := alivePids(t, pidFile); len(alive) != 0 {
		for _, pid := range alive {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		t.Errorf("still running 1 s later: %v", alive)
	}
}
