// Package docscheck holds tests that keep the Markdown docs honest.
package docscheck

import (
	"os"
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

func TestNoEmDashes(t *testing.T) {
	for _, rel := range []string{"README.md", "skill/SKILL.md", "SECURITY.md", "CLAUDE.md", "AGENTS.md", "docs/USERGUIDE.md"} {
		if strings.ContainsRune(read(t, rel), '—') {
			t.Errorf("%s contains an em dash (U+2014)", rel)
		}
	}
}

// CLAUDE.md only imports AGENTS.md, so the two can never drift apart.
func TestClaudeMDImportsAgents(t *testing.T) {
	if got := read(t, "CLAUDE.md"); got != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q, want %q", got, "@AGENTS.md\n")
	}
}
