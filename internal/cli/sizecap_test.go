package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bigFile writes a file of n bytes: front matter, then a long body.
func bigFile(t *testing.T, path string, n int) {
	t.Helper()
	head := "---\nboard: Home\n---\n# Fix the gate\n\n"
	writeFile(t, path, head+strings.Repeat("a", n-len(head)))
}

// A draft or a --description-file over 4 MiB is refused with exit 2
// before fz renders it or sends anything.
func TestFileSizeCap(t *testing.T) {
	srv, calls := acctServer(t, dryRunRoutes)
	_ = srv
	dir := t.TempDir()
	big := filepath.Join(dir, "big.md")
	bigFile(t, big, 4<<20+1)
	ok := filepath.Join(dir, "ok.md")
	bigFile(t, ok, 4<<20)
	for _, argv := range [][]string{
		{"draft", "check", big},
		{"draft", "preview", big},
		{"draft", "apply", big},
		{"card", "create", "Home", "Fix the gate", "--description-file", big},
		{"card", "edit", "87", "--description-file", big},
	} {
		r := runGo(argv)
		if r.code != 2 || !strings.Contains(r.stdout+r.stderr, "fz reads at most 4 MiB") {
			t.Errorf("fz %q: %+v", argv, r)
		}
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("sent %s %s", c.method, c.path)
		}
	}
	// 4 MiB itself is accepted.
	if r := runGo([]string{"draft", "check", ok}); r.code != 0 {
		t.Errorf("4 MiB draft: %+v", r)
	}
	if _, err := os.Stat(big); err != nil {
		t.Fatal(err)
	}
}
