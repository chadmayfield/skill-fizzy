//go:build unix

package draft

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The rewrite keeps the draft's mode and group. A user in more than one
// group can give a file another of them, so the test checks that.
func TestRecordFieldsKeepsModeAndGroup(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "gate.md", "---\nboard: Home\n---\n# Fix the gate\n")
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	gid := int(st.Sys().(*syscall.Stat_t).Gid)
	groups, _ := os.Getgroups()
	other := -1
	for _, g := range groups {
		if g != gid {
			other = g
			break
		}
	}
	if other >= 0 {
		if err := os.Chown(p, -1, other); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordCard(p, 9); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", after.Mode().Perm())
	}
	if other < 0 {
		t.Log("the user is in one group only; the group is not checked")
		return
	}
	if g := int(after.Sys().(*syscall.Stat_t).Gid); g != other {
		t.Errorf("group = %d, want %d", g, other)
	}
}

// A draft with two hard links is refused before anything is written: the
// rename would leave the other link with the old text.
func TestRecordFieldsRefusesHardLinks(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "gate.md", "---\nboard: Home\n---\n# Fix the gate\n")
	if err := os.Link(p, filepath.Join(dir, "hinge.md")); err != nil {
		t.Fatal(err)
	}
	if err := CheckWritable(p); !errors.Is(err, ErrLinked) {
		t.Errorf("CheckWritable: %v", err)
	}
	if err := RecordCard(p, 9); !errors.Is(err, ErrLinked) {
		t.Errorf("RecordCard: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "hinge.md"))
	if string(raw) != "---\nboard: Home\n---\n# Fix the gate\n" {
		t.Errorf("hinge.md = %q", raw)
	}
}
