package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// reservedServer serves board Home (b1), which has a custom column named
// Done, and board Garden (b2), which has none. Card 87 sits on Garden. It
// accepts every write and records each request as "METHOD /path".
func reservedServer(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/cards":
			w.Header().Set("Location", srv.URL+"/1/cards/90.json")
			w.WriteHeader(http.StatusCreated)
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"},{"id":"b2","name":"Garden"}]`)
		case r.URL.Path == "/1/boards/b1/columns":
			io.WriteString(w, `[{"id":"c-done","name":"Done"},{"id":"c-next","name":"Next"}]`)
		case r.URL.Path == "/1/boards/b2/columns":
			io.WriteString(w, `[{"id":"c-later","name":"Later"}]`)
		case r.URL.Path == "/1/cards/87":
			io.WriteString(w, `{"number":87,"title":"Fix the gate","board":{"id":"b2","name":"Garden"},"tags":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		var w []string
		for _, c := range calls {
			if !strings.HasPrefix(c, "GET ") {
				w = append(w, c)
			}
		}
		calls = nil
		return w
	}
}

func TestMoveToReservedColumnNameRefuses(t *testing.T) {
	writes := reservedServer(t)
	for _, args := range [][]string{
		{"card", "move", "87", "--board", "Home", "--to", "Done"},
		{"card", "move", "87", "--board", "Home", "--to", "done"},
		{"move", "87", "--board", "Home", "--to", "done"},
		{"card", "create", "Home", "Fix the gate", "--column", "Done"},
		{"create", "Home", "Fix the gate", "--column", "done"},
	} {
		r := runGo(args)
		if r.code != 2 || !strings.Contains(r.stderr, "the board has a column named 'Done'") ||
			!strings.Contains(r.stderr, "use --to with the column id (c-done)") ||
			!strings.Contains(r.stderr, "to close the card, use fz card close") {
			t.Errorf("%q: %+v", args, r)
		}
		if w := writes(); len(w) != 0 {
			t.Errorf("%q sent %q", args, w)
		}
	}
	r := runGo([]string{"card", "move", "87", "--board", "Home", "--to", "c-done"})
	if w := writes(); r.code != 0 || len(w) != 2 || w[1] != "POST /1/cards/87/triage" {
		t.Errorf("by id: %+v %q", r, w)
	}
	// Garden has no Done column, so done still closes the card.
	r = runGo([]string{"card", "move", "87", "--to", "done"})
	if w := writes(); r.code != 0 || len(w) != 1 || w[0] != "POST /1/cards/87/closure" {
		t.Errorf("no reserved column: %+v %q", r, w)
	}
}

func TestDraftApplyToReservedColumnNameRefuses(t *testing.T) {
	writes := reservedServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, "---\nboard: Home\ncolumn: Done\n---\n# Fix the gate\n\n## Goal\nGate latches.\n")
	r := runGo([]string{"draft", "apply", path})
	if r.code != 2 || !strings.Contains(r.stderr, "the board has a column named 'Done'") {
		t.Errorf("got %+v", r)
	}
	if w := writes(); len(w) != 0 {
		t.Errorf("sent %q", w)
	}
}

func TestSyncToReservedColumnNameRefuses(t *testing.T) {
	text := strings.Replace(syncDraftText, "column: Maybe?\n", "column: Done\n", 1)
	srv, fz, cfg, d := syncFixture(t, text)
	srv.mu.Lock()
	srv.columns = `[{"id":"c-done","name":"Done"}]`
	srv.mu.Unlock()
	_, err := syncDraft(fz, d, cfg.InboxPath(), false, false, nil)
	if err == nil || !strings.Contains(err.Error(), "the board has a column named 'Done'") {
		t.Errorf("err = %v", err)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}
