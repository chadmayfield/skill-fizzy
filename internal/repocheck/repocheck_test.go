// Package repocheck holds tests about the repository itself.
package repocheck

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// private matches the paths of planning and agent files. None of them may be
// tracked, because the repository will be public.
var private = regexp.MustCompile(`(?i)superpowers|(^|/)plans/|(^|/)specs/|CLAUDE\.local\.md|\.claude/settings\.local\.json|\.claude/worktrees/`)

func TestNoPlanningFilesTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not inside a git work tree")
	}
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = strings.TrimSpace(string(top))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var bad []string
	for _, p := range strings.Split(string(out), "\n") {
		if p != "" && private.MatchString(p) {
			bad = append(bad, p)
		}
	}
	if len(bad) > 0 {
		t.Errorf("planning or agent files are tracked:\n  %s", strings.Join(bad, "\n  "))
	}
}

func TestPrivatePatternCoversEveryListedPath(t *testing.T) {
	for _, p := range []string{
		".superpowers/a.md", "docs/superpowers/a.md", "docs/plans/a.md", "docs/specs/a.md",
		"CLAUDE.local.md", ".claude/settings.local.json", ".claude/worktrees/x/a.go", ".Claude/Worktrees/x/a.go",
	} {
		if !private.MatchString(p) {
			t.Errorf("%q is not flagged", p)
		}
	}
	if private.MatchString("docs/USERGUIDE.md") {
		t.Error("flagged an ordinary file")
	}
}
