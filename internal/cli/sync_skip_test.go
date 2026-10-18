package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countCalls counts the server writes that start with prefix since from.
func countCalls(w []string, from int, prefix string) int {
	n := 0
	for _, c := range w[from:] {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// --update skips the image uploads and the title and description PUT when
// the draft and the card's title and description are as at the last sync.
// A changed image byte or a changed body sends them again. A dry run
// prints the same skip line.
func TestUpdateSkipsUnchangedDescription(t *testing.T) {
	srv := newSyncServer(t)
	srv.uploads = true
	home, drafts := imageEnv(t, srv)
	img := filepath.Join(home, "inbox", "hinge.jpg")
	writeFile(t, img, "jpg bytes")
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\nSwap the hinge.\n\n![a](../hinge.jpg)\n")
	if r := runGo([]string{"draft", "apply", p}); r.code != 0 {
		t.Fatalf("create: %+v", r)
	}
	update := func() (result, int) {
		t.Helper()
		from := len(srv.writes())
		r := runGo([]string{"draft", "apply", "--update", p})
		if r.code != 0 {
			t.Fatalf("update: %+v\n%s", r, syncReadDraft(t, p))
		}
		return r, from
	}
	runGo([]string{"draft", "apply", "--update", p})
	r, from := update()
	w := srv.writes()
	if countCalls(w, from, "POST /1/rails/active_storage/direct_uploads") != 0 || countCalls(w, from, "PUT /1/cards/90") != 0 {
		t.Errorf("unchanged draft sent %q", w[from:])
	}
	if !strings.Contains(r.stdout, "#90 title and description unchanged\n") {
		t.Errorf("unchanged: %+v", r)
	}
	if !strings.Contains(syncReadDraft(t, p), "\nsynced-draft: sha256:") {
		t.Errorf("no synced-draft line:\n%s", syncReadDraft(t, p))
	}
	if d := runGo([]string{"--dry-run", "draft", "apply", "--update", p}); !strings.Contains(d.stdout, "#90 title and description unchanged\n") {
		t.Errorf("dry run: %+v", d)
	}

	// One byte of the image changes: it is uploaded and the description sent.
	writeFile(t, img, "jpg bytez")
	_, from = update()
	w = srv.writes()
	if countCalls(w, from, "POST /1/rails/active_storage/direct_uploads") != 1 || countCalls(w, from, "PUT /1/cards/90") != 1 {
		t.Errorf("changed image sent %q", w[from:])
	}

	// The body changes: the description is sent.
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, strings.Replace(string(b), "Swap the hinge.", "Swap both hinges.", 1))
	_, from = update()
	w = srv.writes()
	if countCalls(w, from, "PUT /1/cards/90") != 1 {
		t.Errorf("changed body sent %q", w[from:])
	}
}
