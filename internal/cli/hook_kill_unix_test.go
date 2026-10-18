//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
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

// checkAllDead fails the test if any pid in pidFile still runs, and kills
// what it finds. The file must hold two pids.
func checkAllDead(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	fields := strings.Fields(string(data))
	if err != nil || len(fields) != 2 {
		t.Fatalf("pid file %q: %v", data, err)
	}
	for _, f := range fields {
		pid, err := strconv.Atoi(f)
		if err != nil {
			t.Fatalf("pid %q: %v", f, err)
		}
		if err := syscall.Kill(pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Errorf("pid %d still running 1 s after the hook returned", pid)
		}
	}
}

// The Go hook kills a hung token command, and its children, when it stops
// at its time limit.
func TestHookKillsHungTokenCommand(t *testing.T) {
	f := newHookFixture(t)
	pidFile := filepath.Join(t.TempDir(), "pids")
	t.Setenv("FIZZY_TOKEN", "")
	t.Setenv("FIZZY_TOKEN_CMD", hungTokenCmd(pidFile))
	old := hookTimeout
	hookTimeout = time.Second
	t.Cleanup(func() { hookTimeout = old })

	r := runHook(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	time.Sleep(time.Second)
	checkAllDead(t, pidFile)
}

// The Python hook kills fz, and everything fz started, when an fz call
// passes its time limit.
func TestPythonHookKillsHungFz(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not on PATH")
	}
	f := newHookFixture(t)
	pidFile := filepath.Join(t.TempDir(), "pids")
	// A fake fz that hangs the way fz does on a hung token command.
	writeFile(t, f.fakeFz, "#!/bin/sh\n"+hungTokenCmd(pidFile)+"\n")
	// Run a copy of the script with a 1 second limit per call.
	src, err := os.ReadFile(f.script)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, f.script, strings.Replace(string(src), "TIMEOUT = 60", "TIMEOUT = 1", 1))

	start := time.Now()
	r := runPythonHook(t, f, f.root, `{"cwd":"`+filepath.Join(f.root, "gatehouse")+`"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %v", d)
	}
	time.Sleep(time.Second)
	checkAllDead(t, pidFile)
}
