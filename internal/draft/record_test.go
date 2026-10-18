package draft

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRecordFieldsKeepsBytes checks that RecordFields changes only the lines
// it writes: line endings, delimiter lines with trailing spaces, a lone CR in
// the body and other separators stay as they were.
func TestRecordFieldsKeepsBytes(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{
			"crlf",
			"---\r\nboard: Home\r\ncard: 5\r\nnote: keep\r\n---\r\n# Fix the gate\r\n\r\nGate latches.  \r\na\rb\r\n",
			"---\r\nboard: Home\r\nnote: keep\r\ncard: 9\r\n---\r\n# Fix the gate\r\n\r\nGate latches.  \r\na\rb\r\n",
		},
		{
			"delimiters with trailing spaces",
			"---  \nboard: Home\n---  \n# Fix the gate\n",
			"---  \nboard: Home\ncard: 9\n---  \n# Fix the gate\n",
		},
		{
			"lone cr in the body of an lf file",
			"---\nboard: Home\n---\n# Fix the gate\n\nOil\rthe hinge.\n",
			"---\nboard: Home\ncard: 9\n---\n# Fix the gate\n\nOil\rthe hinge.\n",
		},
		{
			"form feed inside a front matter line",
			"---\nboard: Home\fnote: y\n---\n# Fix the gate\n",
			"---\nboard: Home\fnote: y\ncard: 9\n---\n# Fix the gate\n",
		},
		{
			"bom",
			"\xef\xbb\xbf---\nboard: Home\n---\n# Fix the gate\n",
			"\xef\xbb\xbf---\nboard: Home\ncard: 9\n---\n# Fix the gate\n",
		},
		{
			"bom and crlf",
			"\xef\xbb\xbf---\r\nboard: Home\r\n---\r\n# Fix the gate\r\n",
			"\xef\xbb\xbf---\r\nboard: Home\r\ncard: 9\r\n---\r\n# Fix the gate\r\n",
		},
		{
			"comments and quoting kept",
			"---\nboard:   \"Home\"   # c\n# a comment\ncard: 5\n---\n# Fix the gate\n",
			"---\nboard:   \"Home\"   # c\n# a comment\ncard: 9\n---\n# Fix the gate\n",
		},
		{
			// The key must match with its colon.
			"keys that start with card",
			"---\nboard: Home\ncards: gate, hinge\ncardinal: north\ncard: 5\n---\n# Fix the gate\n",
			"---\nboard: Home\ncards: gate, hinge\ncardinal: north\ncard: 9\n---\n# Fix the gate\n",
		},
		{
			// Blank lines in the front matter are kept,
			// and the new line goes after the last line with content.
			"blank line before the closing delimiter",
			"---\nboard: Home\n\n---\n# Fix the gate\n",
			"---\nboard: Home\ncard: 9\n\n---\n# Fix the gate\n",
		},
		{
			"blank line before the closing delimiter, bom and crlf",
			"\xef\xbb\xbf---\r\nboard: Home\r\n\r\n---\r\n# Fix the gate\r\n",
			"\xef\xbb\xbf---\r\nboard: Home\r\ncard: 9\r\n\r\n---\r\n# Fix the gate\r\n",
		},
		{
			"blank line between keys",
			"---\nboard: Home\n\nnote: keep\n---\n# Fix the gate\n",
			"---\nboard: Home\n\nnote: keep\ncard: 9\n---\n# Fix the gate\n",
		},
		{
			"whitespace line before the closing delimiter",
			"---\nboard: Home\n   \n---\n# Fix the gate\n",
			"---\nboard: Home\ncard: 9\n   \n---\n# Fix the gate\n",
		},
		{
			"old card line and a blank line",
			"---\nboard: Home\ncard: 5\n\n\n---\n# Fix the gate\n",
			"---\nboard: Home\ncard: 9\n\n\n---\n# Fix the gate\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeFile(t, t.TempDir(), "gate.md", tc.in)
			if _, err := Parse(p); err != nil {
				t.Fatalf("parse before: %v", err)
			}
			if err := RecordCard(p, 9); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Fatalf("got  %q\nwant %q", raw, tc.want)
			}
			d, err := Parse(p)
			if err != nil {
				t.Fatalf("parse after: %v", err)
			}
			if d.Card != 9 || d.Board != "Home" || d.Title != "Fix the gate" {
				t.Fatalf("parsed card %d board %q title %q", d.Card, d.Board, d.Title)
			}
		})
	}
}

func TestParseStripsBOM(t *testing.T) {
	p := writeFile(t, t.TempDir(), "gate.md", "\xef\xbb\xbf---\nboard: Home\n---\n# Fix the gate\n\nGate latches.\n")
	d, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Board != "Home" || d.Title != "Fix the gate" || d.BodyMD != "Gate latches." {
		t.Fatalf("board %q title %q body %q", d.Board, d.Title, d.BodyMD)
	}
}

// TestRecordFieldsReplacesTheFile checks that the write goes to a new file
// that is renamed over the draft, so a crash cannot leave it cut short, and
// that it keeps the file mode and leaves no temp file behind.
func TestRecordFieldsReplacesTheFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "gate.md", "---\nboard: Home\n---\n# Fix the gate\n")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordCard(p, 9); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("the draft was rewritten in place, not replaced by a rename")
	}
	if after.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", after.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("files left in the folder: %q", names)
	}
}

func TestRecordFieldsWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "real.md", "---\nboard: Home\n---\n# Fix the gate\n")
	link := filepath.Join(dir, "gate.md")
	if err := os.Symlink("real.md", link); err != nil {
		t.Fatal(err)
	}
	if err := RecordCard(link, 9); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink was replaced: %v %v", st, err)
	}
	raw, _ := os.ReadFile(target)
	if want := "---\nboard: Home\ncard: 9\n---\n# Fix the gate\n"; string(raw) != want {
		t.Fatalf("target = %q, want %q", raw, want)
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	ok := writeFile(t, dir, "gate.md", "---\nboard: Home\n---\n# Fix the gate\n")
	if err := CheckWritable(ok); err != nil {
		t.Errorf("writable draft: %v", err)
	}
	ro := writeFile(t, dir, "hinge.md", "---\nboard: Home\n---\n# Oil the hinge\n")
	if err := os.Chmod(ro, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := CheckWritable(ro); err == nil {
		t.Error("a read-only draft passed")
	}
	sub := filepath.Join(dir, "locked")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inLocked := writeFile(t, sub, "latch.md", "---\nboard: Home\n---\n# Fix the latch\n")
	if err := os.Chmod(sub, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sub, 0o755) })
	if err := CheckWritable(inLocked); err == nil {
		t.Error("a draft in a read-only folder passed")
	}
	if err := CheckWritable(sub); err == nil {
		t.Error("a folder passed as a draft")
	}
}
