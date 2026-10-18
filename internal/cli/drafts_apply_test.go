package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRefusesBadCardValue(t *testing.T) {
	for _, value := range []string{"87abc", "87.0", "+87", "-5", "99999999999999999999"} {
		t.Run(value, func(t *testing.T) {
			srv := newSyncServer(t)
			path := filepath.Join(t.TempDir(), "gate.md")
			text := strings.Replace(syncDraftText, "card: 87\n", "card: "+value+"\n", 1)
			writeFile(t, path, srv.draft(t, text))
			before := syncReadDraft(t, path)
			for _, args := range [][]string{
				{"draft", "apply", path},
				{"draft", "apply", "--update", path},
			} {
				got := syncRun(t, srv, args...)
				if got.code != 2 || !strings.Contains(got.stderr, "front matter line 'card: "+value+"'") {
					t.Errorf("%q: code %d stderr %q", args, got.code, got.stderr)
				}
				if w := srv.writes(); len(w) != 0 {
					t.Errorf("%q: writes reached the server: %q", args, w)
				}
				if syncReadDraft(t, path) != before {
					t.Errorf("%q: the draft changed", args)
				}
			}
		})
	}
}

func TestApplyUpdateAcceptsHashCardAndKeepsTheLine(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, strings.Replace(syncDraftText, "card: 87\n", "card: #87\n", 1)))
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if !syncHas(srv.writes(), "PUT /1/cards/87") {
		t.Errorf("card #87 was not updated: %q", srv.writes())
	}
	for _, w := range srv.writes() {
		if strings.HasPrefix(w, "POST /1/boards/") {
			t.Errorf("a new card was created: %q", srv.writes())
		}
	}
	if text := syncReadDraft(t, path); !strings.Contains(text, "\ncard: #87\n") {
		t.Errorf("the card: line was rewritten:\n%s", text)
	}
}

func TestApplyRefusesBadGoldenValue(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, "---\nboard: Home\ngolden: maybe\n---\n# Oil the hinge\n")
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 2 || !strings.Contains(got.stderr, "front matter line 'golden: maybe'") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

// creates counts the card creates the server saw.
func (s *syncServer) creates() int {
	n := 0
	for _, w := range s.writes() {
		if strings.HasPrefix(w, "POST /1/boards/") && strings.HasSuffix(w, "/cards") {
			n++
		}
	}
	return n
}

func TestApplyNamesOneDraftOnce(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "hinge.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Oil the hinge\n")
	link := filepath.Join(t.TempDir(), "alias.md")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	got := syncRun(t, srv, "draft", "apply", dir+"/", path, dir+"/./hinge.md", link)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if n := srv.creates(); n != 1 {
		t.Errorf("%d creates for one draft: %q", n, srv.writes())
	}
}
