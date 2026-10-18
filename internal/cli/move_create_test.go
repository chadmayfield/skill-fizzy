package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// moveCreateServer is a fake Fizzy for card move and card create. Board
// Home (b1) has the columns in homeCols, and board Shop (b2) those in
// shopCols. Card 87 sits on Home. A card create answers with location,
// or with no Location header when location is "". POST taggings answers
// tagStatus. It records every call.
type moveCreateServer struct {
	homeCols, shopCols string
	location           string
	tagStatus          int
	stepLocation       bool

	mu    sync.Mutex
	calls []seenCall
}

func newMoveCreateServer(t *testing.T) *moveCreateServer {
	t.Helper()
	s := &moveCreateServer{
		homeCols:     `[{"id":"c1","name":"Now"},{"id":"c2","name":"Next"}]`,
		shopCols:     `[{"id":"c3","name":"Next"},{"id":"c4","name":"NEXT"},{"id":"c5","name":"Later"}]`,
		location:     "/1/cards/90.json",
		tagStatus:    http.StatusNoContent,
		stepLocation: true,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.calls = append(s.calls, seenCall{r.Method, r.URL.Path, string(b)})
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/cards":
			if s.location != "" {
				w.Header().Set("Location", s.location)
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/taggings"):
			w.WriteHeader(s.tagStatus)
		case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/steps") || strings.HasSuffix(r.URL.Path, "/comments")):
			if s.stepLocation {
				w.Header().Set("Location", r.URL.Path+"/03abc123def456ghi789jkl0mn.json")
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"},{"id":"b2","name":"Shop"}]`)
		case r.URL.Path == "/1/boards/b1/columns":
			io.WriteString(w, s.homeCols)
		case r.URL.Path == "/1/boards/b2/columns":
			io.WriteString(w, s.shopCols)
		case r.URL.Path == "/1/cards/87":
			io.WriteString(w, `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},"tags":[]}`)
		case r.URL.Path == "/1/cards/90":
			// A new card's reply, here with no tags key.
			io.WriteString(w, `{"number":90,"title":"Oil the hinge","board":{"id":"b1","name":"Home"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	return s
}

// writes returns the non-GET calls so far as "METHOD /path".
func (s *moveCreateServer) writes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, c := range s.calls {
		if c.method != http.MethodGet {
			out = append(out, c.method+" "+c.path)
		}
	}
	return out
}

// card move --board B --to X checks X on B before the card changes board.
func TestMoveToOtherBoardChecksTargetFirst(t *testing.T) {
	for _, tc := range []struct {
		to   string
		code int
	}{
		{"Nope", 1},
		{"next", 2}, // Next and NEXT on Shop
	} {
		s := newMoveCreateServer(t)
		r := runGo([]string{"card", "move", "87", "--board", "Shop", "--to", tc.to})
		if r.code != tc.code || r.stdout != "" {
			t.Errorf("--to %s: %+v, want exit %d", tc.to, r, tc.code)
		}
		if w := s.writes(); len(w) != 0 {
			t.Errorf("--to %s sent %q", tc.to, w)
		}
	}
	// A reserved name on the target board is refused the same way.
	s := newMoveCreateServer(t)
	s.shopCols = `[{"id":"c6","name":"Done"}]`
	r := runGo([]string{"card", "move", "87", "--board", "Shop", "--to", "done"})
	if r.code != 2 || len(s.writes()) != 0 {
		t.Errorf("reserved: %+v, writes %q", r, s.writes())
	}
	// A good target moves the board, then the column on the new board.
	s = newMoveCreateServer(t)
	r = runGo([]string{"card", "move", "87", "--board", "Shop", "--to", "Later"})
	want := []string{"PUT /1/cards/87/board", "POST /1/cards/87/triage"}
	if r.code != 0 || strings.Join(s.writes(), ",") != strings.Join(want, ",") {
		t.Errorf("good move: %+v, writes %q", r, s.writes())
	}
	if c := s.calls[len(s.calls)-1]; !strings.Contains(c.body, `"c5"`) {
		t.Errorf("triage body %q, want column c5", c.body)
	}
}

// card move with neither --to nor --board is a usage error.
func TestMoveNeedsToOrBoard(t *testing.T) {
	for _, argv := range [][]string{{"card", "move", "87"}, {"move", "87"}} {
		s := newMoveCreateServer(t)
		r := runGo(argv)
		if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, "--to") {
			t.Errorf("fz %q: %+v, want exit 2", argv, r)
		}
		if len(s.calls) != 0 {
			t.Errorf("fz %q made calls %+v", argv, s.calls)
		}
	}
}
