package repocheck

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// processNote matches comment text that records how the code was written
// rather than what it does: review ids, ruling and mutant numbers, batch
// names and session links. Comments describe the code; history lives in git.
var processNote = regexp.MustCompile(`(?i)\b(opus|fable|sonnet|haiku)-[a-z]+-\d+|\bR\d{2,3}\b|\bmutants?\b|\bruling\b|\bbatch \d|\bthe (brief|reviewer)\b|claude\.ai/code`)

// hashComment matches a whole-line # comment in shell, Python, Makefile and
// YAML files.
var hashComment = regexp.MustCompile(`^\s*#(.*)$`)

func TestCommentsDescribeCode(t *testing.T) {
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var bad []string
	for _, rel := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if rel == "internal/repocheck/comments_test.go" {
			continue
		}
		for _, c := range comments(t, filepath.Join(root, rel), rel) {
			if processNote.MatchString(c.text) {
				bad = append(bad, rel+":"+c.line+": "+strings.TrimSpace(c.text))
			}
		}
	}
	if len(bad) > 0 {
		t.Errorf("comments that describe the work process, not the code:\n  %s", strings.Join(bad, "\n  "))
	}
}

func TestProcessNotePattern(t *testing.T) {
	for _, s := range []string{"see opus-integrity-7", "R123 says so", "kills mutant 5", "per the ruling", "from batch 4", "as the reviewer asked", "see claude.ai/code/x"} {
		if !processNote.MatchString(s) {
			t.Errorf("processNote misses %q", s)
		}
	}
	for _, s := range []string{"MatchUser finds a user by id, name or email.", "Retry after a 503.", "Reads every page."} {
		if processNote.MatchString(s) {
			t.Errorf("processNote matches plain comment %q", s)
		}
	}
}

type comment struct{ line, text string }

// comments returns the comments in one tracked file. Go files are parsed, so
// strings never count; shell, Python, Makefile and YAML use # lines.
func comments(t *testing.T, path, rel string) []comment {
	t.Helper()
	var cs []comment
	switch {
	case strings.HasSuffix(rel, ".go"):
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, g := range f.Comments {
			for _, c := range g.List {
				cs = append(cs, comment{strconv.Itoa(fset.Position(c.Pos()).Line), c.Text})
			}
		}
	case strings.HasSuffix(rel, ".py"), strings.HasSuffix(rel, ".sh"), strings.HasSuffix(rel, ".yml"),
		strings.HasSuffix(rel, ".yaml"), filepath.Base(rel) == "Makefile":
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for i, l := range strings.Split(string(data), "\n") {
			if m := hashComment.FindStringSubmatch(l); m != nil {
				cs = append(cs, comment{strconv.Itoa(i + 1), m[1]})
			}
		}
	}
	return cs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not inside a git work tree")
	}
	return strings.TrimSpace(string(top))
}
