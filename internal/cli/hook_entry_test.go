package cli

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// runHookIn runs the Go hook for the gatehouse folder with the given map.
func runHookWithMap(t *testing.T, f hookFixture, repos, dir string) result {
	t.Helper()
	writeFile(t, f.mapPath, repos)
	return runHook(`{"cwd":"` + filepath.Join(f.root, dir) + `"}`)
}

// An entry with an empty or blank board counts as unmapped: it prints
// nothing and sends nothing.
func TestHookEmptyBoardIsUnmapped(t *testing.T) {
	for _, board := range []string{`""`, `"  "`} {
		f := newHookFixture(t)
		r := runHookWithMap(t, f, `{"gatehouse": {"board": `+board+`}}`, "gatehouse")
		if r != (result{"", "", 0}) {
			t.Errorf("board %s: %+v", board, r)
		}
		if n := atomic.LoadInt64(f.hits); n != 0 {
			t.Errorf("board %s: sent %d requests", board, n)
		}
	}
}

// A card written as a float, such as 87.0, is refused rather than read as
// the whole board. 0.0 means the whole board, as 0 does.
func TestHookFloatCard(t *testing.T) {
	f := newHookFixture(t)
	for _, card := range []string{"87.0", "88.5", "8.8e1"} {
		if r := runHookWithMap(t, f, `{"gatehouse": {"board": "Home", "card": `+card+`}}`, "gatehouse"); r != (result{"", "", 0}) {
			t.Errorf("card %s: %+v", card, r)
		}
	}
	r := runHookWithMap(t, f, `{"gatehouse": {"board": "Home", "card": 0.0}}`, "gatehouse")
	if !strings.HasPrefix(r.stdout, "Fizzy: this repo maps to the Home board.\n") {
		t.Errorf("card 0.0: %+v", r)
	}
}

// A card string from the map is cleaned before it is printed.
func TestHookCleansCardString(t *testing.T) {
	f := newHookFixture(t)
	r := runHookWithMap(t, f, `{"gatehouse": {"board": "Home", "card": "x\u001b[2J\u0007\n7"}}`, "gatehouse")
	want := "Fizzy: this repo's card is on the Home board:\n  #x7 (not open)\n"
	if body, _ := splitNote(r.stdout); body != want || r.code != 0 {
		t.Errorf("got %+v, want %q", r, want)
	}
}

// In a worktree of a bare repo, the git common dir does not end in /.git,
// so its parent folder is not taken as the repo name.
func TestHookBareRepoWorktree(t *testing.T) {
	f := newHookFixture(t)
	bare := filepath.Join(f.root, "porch", "gate.git")
	for _, args := range [][]string{
		{"clone", "-q", "--bare", filepath.Join(f.root, "gatehouse"), bare},
		{"-C", bare, "worktree", "add", "-q", filepath.Join(f.root, "attic", "wt")},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// porch maps to card 120. The worktree's own path holds no mapped name.
	r := runHook(`{"cwd":"` + filepath.Join(f.root, "attic", "wt") + `"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
}

// Results that arrive after the deadline are dropped.
func TestHookDropsLateResults(t *testing.T) {
	f := newHookFixture(t)
	root := NewRoot()
	root.SetIn(strings.NewReader(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`))
	cmd, _, err := root.Find([]string{"hook", "session-start"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := sessionStartText(ctx, newContext(cmd)); got != "" {
		t.Errorf("after the deadline: %q", got)
	}
}

// The help says what the exit status is: 0 on any failure at run time, and
// 2 for a usage error, which a bad flag still is.
func TestHookExitStatusHelp(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"hook", "session-start"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{cmd.Long, cmd.Parent().Long} {
		if !strings.Contains(c, "exits 0 on any failure at run time; a usage error exits 2") {
			t.Errorf("help %q", c)
		}
	}
	if r := runHook("", "--nope"); r.code != 2 {
		t.Errorf("bad flag: %+v", r)
	}
}
