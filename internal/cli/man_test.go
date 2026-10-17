package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runMan(t *testing.T, dir string) string {
	t.Helper()
	var out, errw bytes.Buffer
	code := run(NewRoot(), []string{"--config", "/nonexistent.json", "man", "--dir", dir}, &out, &errw)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errw.String())
	}
	return out.String()
}

func readPages(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "man1", "*.1"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no pages in %s (%v)", dir, err)
	}
	pages := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pages[filepath.Base(f)] = string(b)
	}
	return pages
}

func TestManCommandExists(t *testing.T) {
	var found *cobra.Command
	for _, c := range NewRoot().Commands() {
		if c.Name() == "man" {
			found = c
		}
	}
	if found == nil {
		t.Fatal("fz man missing")
	}
	if found.Short == "" || found.Long == "" || found.Example == "" {
		t.Error("fz man needs Short, Long and Example")
	}
}

func TestManInstall(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")
	dir := t.TempDir()
	runMan(t, dir)
	pages := readPages(t, dir)
	for _, name := range []string{"fz.1", "fz-card-list.1", "fz-draft-apply.1", "fz-man.1"} {
		if _, ok := pages[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	// Every available command has a page that names it.
	walk(NewRoot(), func(c *cobra.Command) {
		if !c.IsAvailableCommand() && c.Parent() != nil {
			return
		}
		name := strings.ReplaceAll(c.CommandPath(), " ", "-")
		page, ok := pages[name+".1"]
		if !ok {
			t.Errorf("no page for %q", c.CommandPath())
			return
		}
		if !strings.Contains(page, name) {
			t.Errorf("%s.1 does not name its command", name)
		}
	})
	if th := thLine(pages["fz.1"]); th != `.TH "FZ" "1" "" "fz dev" "User Commands"` {
		t.Errorf("fz.1 header: %q", th)
	}
}

func TestManRootSections(t *testing.T) {
	dir := t.TempDir()
	runMan(t, dir)
	root := readPages(t, dir)["fz.1"]
	for _, h := range []string{"DRAFTS", "FILES", "ENVIRONMENT", "EXIT STATUS"} {
		if !strings.Contains(root, ".SH \""+h+"\"") && !strings.Contains(root, ".SH "+h) {
			t.Errorf("fz.1 has no %s section", h)
		}
	}
	for _, want := range []string{"FIZZY_TOKEN_CMD", "FIZZY_RULES_CARD", "actions.log", "without --yes"} {
		if !strings.Contains(root, want) {
			t.Errorf("fz.1 lacks %q", want)
		}
	}
}

func TestManHelpLong(t *testing.T) {
	long := NewRoot().Long
	for _, h := range []string{"DRAFTS", "FILES", "ENVIRONMENT", "EXIT STATUS"} {
		if !strings.Contains(long, h) {
			t.Errorf("root Long lacks %s", h)
		}
	}
	if strings.Contains(long, "\n# ") {
		t.Error("root Long holds Markdown heading marks")
	}
}

func TestManDeterministic(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")
	a, b := t.TempDir(), t.TempDir()
	runMan(t, a)
	runMan(t, b)
	pa, pb := readPages(t, a), readPages(t, b)
	if len(pa) != len(pb) {
		t.Fatalf("%d pages then %d", len(pa), len(pb))
	}
	for name, page := range pa {
		if pb[name] != page {
			t.Errorf("%s differs between runs", name)
		}
		if strings.Contains(page, "Auto generated") || strings.Contains(page, "HISTORY") {
			t.Errorf("%s has an auto-generated tag", name)
		}
	}
}

func TestManSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1760000000")
	dir := t.TempDir()
	runMan(t, dir)
	if first := thLine(readPages(t, dir)["fz.1"]); !strings.Contains(first, "Oct 2025") {
		t.Errorf("header %q lacks the epoch date", first)
	}
}

func TestManNoPrivateText(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")
	dir := t.TempDir()
	runMan(t, dir)
	home, _ := os.UserHomeDir()
	for name, page := range readPages(t, dir) {
		for _, bad := range []string{"redacted", "redacted", "redacted", "github.com", "basecamp", home} {
			if bad != "" && strings.Contains(page, bad) {
				t.Errorf("%s contains %q", name, bad)
			}
		}
		// SEE ALSO may only point at other fz pages.
		if _, after, ok := strings.Cut(page, "SEE ALSO"); ok {
			if strings.Contains(after, "http") || strings.Contains(after, "@") {
				t.Errorf("%s: SEE ALSO holds a link or address", name)
			}
		}
		if strings.ContainsRune(page, emDash) {
			t.Errorf("%s has an em dash", name)
		}
	}
}

func TestManDryRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never")
	var out, errw bytes.Buffer
	if code := run(NewRoot(), []string{"--dry-run", "man", "--dir", dir}, &out, &errw); code != 0 {
		t.Fatalf("exit %d: %s", code, errw.String())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("dry run created the directory")
	}
}

// thLine returns the .TH line of a page.
func thLine(page string) string {
	for _, l := range strings.Split(page, "\n") {
		if strings.HasPrefix(l, ".TH ") {
			return l
		}
	}
	return ""
}
