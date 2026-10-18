// Package docscheck holds tests that keep the Markdown docs honest.
package docscheck

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const root = "../.."

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// Every `templates/...` path in the skill must exist next to SKILL.md, so the
// skill works when it is linked into ~/.claude/skills on its own.
func TestSkillTemplatesExist(t *testing.T) {
	text := read(t, "skill/SKILL.md")
	paths := regexp.MustCompile("`(templates/[^`]+)`").FindAllStringSubmatch(text, -1)
	if len(paths) == 0 {
		t.Fatal("SKILL.md mentions no templates/ path")
	}
	for _, m := range paths {
		if _, err := os.Stat(filepath.Join(root, "skill", m[1])); err != nil {
			t.Errorf("SKILL.md names %s, missing under skill/: %v", m[1], err)
		}
	}
}

// The skill must work for an agent that has only the fz binary on PATH.
func TestSkillDoesNotNeedTheRepo(t *testing.T) {
	text := read(t, "skill/SKILL.md")
	for _, bad := range []string{"bin/fz", "fizzy_skill", "pip install", "python"} {
		if strings.Contains(strings.ToLower(text), bad) {
			t.Errorf("SKILL.md mentions %q", bad)
		}
	}
}

// emDash is U+2014, written as an escape so this file passes its own check.
const emDash = '\u2014'

// Every tracked text file is free of em dashes. A file with a NUL byte is
// binary and skipped.
func TestNoEmDashes(t *testing.T) {
	files := trackedFiles(t)
	if len(files) == 0 {
		t.Fatal("git ls-files listed nothing")
	}
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue // deleted in the work tree
		}
		if bytes.IndexByte(b, 0) >= 0 {
			continue
		}
		if bytes.ContainsRune(b, emDash) {
			t.Errorf("%s contains an em dash (U+2014)", rel)
		}
	}
}

// trackedFiles lists the files git tracks, relative to the repo root.
func trackedFiles(t *testing.T) []string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

// CLAUDE.md only imports AGENTS.md, so the two can never drift apart.
func TestClaudeMDImportsAgents(t *testing.T) {
	if got := read(t, "CLAUDE.md"); got != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q, want %q", got, "@AGENTS.md\n")
	}
}
