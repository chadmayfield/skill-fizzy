//go:build unix

package cli

import (
	"context"
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

// TestPythonHookBoundsRegularMap limits map size and time spent parsing JSON.
func TestPythonHookBoundsRegularMap(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not on PATH")
	}
	fixture := newHookFixture(t)
	program := `import runpy, sys, time
hook = runpy.run_path(sys.argv[1], run_name="fizzy_hook_test")
namespace = hook["read_map"].__globals__
namespace["DEADLINE"] = time.monotonic() + 0.2
if sys.argv[2] == "slow":
    original = namespace["json"].loads
    def slow_loads(*args, **kwargs):
        time.sleep(5)
        return original(*args, **kwargs)
    namespace["json"].loads = slow_loads
try:
    hook["read_map"]()
except TimeoutError:
    sys.exit(0 if sys.argv[2] == "slow" else 2)
except ValueError:
    sys.exit(0 if sys.argv[2] == "large" else 2)
sys.exit(3)
`
	for _, mode := range []string{"slow", "large"} {
		t.Run(mode, func(t *testing.T) {
			text := hookMap
			if mode == "large" {
				text = strings.Repeat(" ", 1<<20) + text
			}
			writeFile(t, fixture.mapPath, text)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "python3", "-I", "-c", program, fixture.script, mode)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + fixture.home, "FIZZY_REPOS_MAP=" + fixture.mapPath}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("map %s: %v, %s", mode, err, out)
			}
		})
	}
}

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
	checkPidsDead(t, fields)
}

// checkPidsDead fails the test if any of pids still runs, and kills what it
// finds.
func checkPidsDead(t *testing.T, pids []string) {
	t.Helper()
	for _, f := range pids {
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
	// TIMEOUT bounds the script from the start of main(), not each fz
	// call. The extra time is for starting Python on a loaded machine.
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Errorf("took %v, want about 1 s", d)
	}
	// On a loaded machine the limit can pass before the fake fz writes its
	// pids, and the kill then comes before fz starts. Wait up to the limit
	// for the file. If fz never started, the check above, that the hook
	// printed nothing and exited 0, is all there is to test.
	var pids []string
	for deadline := time.Now().Add(time.Second); ; {
		data, _ := os.ReadFile(pidFile)
		pids = strings.Fields(string(data))
		if len(pids) == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(pids) == 0 {
		t.Log("the time limit passed before fz started; checked the output only")
		return
	}
	time.Sleep(time.Second)
	checkPidsDead(t, pids)
}

// The Python hook's limit covers stdin and the map. A stdin left open, or a
// named pipe at the map's path, prints nothing and ends the hook near the
// limit, not later. The fixture's copy has a 60 second limit, so this test
// runs one with 1 second.
func TestPythonHookLimitCoversStdinAndMap(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not on PATH")
	}
	f := newHookFixture(t)
	src, err := os.ReadFile(f.script)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "fizzy-session-hook.py")
	writeFile(t, script, strings.Replace(string(src), "TIMEOUT = 60", "TIMEOUT = 1", 1))
	fifo := filepath.Join(f.root, "repos-fifo.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mapPath string
		openStdin     bool
	}{
		{"stdin left open", f.mapPath, true},
		{"map is a named pipe", fifo, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("python3", "-I", script)
			cmd.Dir = f.root
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + f.home, "FIZZY_REPOS_MAP=" + tc.mapPath, "FIZZY_FZ=" + f.fakeFz}
			if tc.openStdin {
				w, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				defer w.Close()
			} else {
				cmd.Stdin = strings.NewReader(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`)
			}
			var out strings.Builder
			cmd.Stdout = &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil || out.String() != "" {
					t.Errorf("err %v, stdout %q", err, out.String())
				}
				if d := time.Since(start); d > 3*time.Second {
					t.Errorf("took %v, want about 1 s", d)
				}
			case <-time.After(6 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Fatal("the hook ran past its limit")
			}
		})
	}
}
