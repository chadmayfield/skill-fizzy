package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// upload of a missing file or a folder is a usage error that sends nothing.
func TestUploadMissingFileOrFolder(t *testing.T) {
	_, calls := extraServer(t, openCard)
	for _, p := range []string{filepath.Join(t.TempDir(), "nope.jpg"), t.TempDir()} {
		for _, dry := range []bool{false, true} {
			argv := []string{"upload", p}
			if dry {
				argv = append([]string{"--dry-run"}, argv...)
			}
			r := runGo(argv)
			if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, p) {
				t.Errorf("fz %q: %+v, want exit 2", argv, r)
			}
		}
	}
	if c := calls(); len(c) != 0 {
		t.Errorf("sent %+v", c)
	}
}
