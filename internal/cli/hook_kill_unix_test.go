//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

	start := time.Now()
	r := runHook(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	// The cleanup fits inside the limit; the hook never runs past it.
	if d := time.Since(start); d > hookTimeout+100*time.Millisecond {
		t.Errorf("took %v, want at most %v", d, hookTimeout)
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
	script := regexp.MustCompile(`(?m)^TIMEOUT = [0-9.]+`).ReplaceAllString(string(src), "TIMEOUT = 1")
	if script == string(src) {
		t.Fatal("no TIMEOUT line to shorten in the script")
	}
	writeFile(t, f.script, script)

	start := time.Now()
	r := runPythonHook(t, f, f.root, `{"cwd":"`+filepath.Join(f.root, "gatehouse")+`"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	// TIMEOUT bounds the whole script, not each fz call.
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("took %v, want about 1 s", d)
	}
	time.Sleep(time.Second)
	checkAllDead(t, pidFile)
}
