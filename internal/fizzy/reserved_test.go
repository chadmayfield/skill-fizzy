package fizzy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A custom column named like a move keyword (done, maybe, maybe? or not now,
// in any case) makes --to with that name ambiguous, so MoveCard refuses and
// sends nothing. The id still reaches the column, and other keywords still
// work.
func TestMoveCardReservedColumnName(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		switch {
		case r.Method != http.MethodGet:
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/columns"):
			io.WriteString(w, `[{"id":"c1","name":"Done"},{"id":"c2","name":"Maybe?"},{"id":"c3","name":"Not now"},{"id":"c4","name":"Next"}]`)
		default:
			io.WriteString(w, `{"number":87,"board":{"id":"b1","name":"Home"}}`)
		}
	})
	for _, to := range []string{"done", "Done", "DONE", "maybe", "Maybe?", "not now", "Not Now", "not-now"} {
		for _, board := range []string{"b1", ""} {
			s.calls = nil
			err := f.MoveCard(87, to, board)
			if !IsRefusal(err) {
				t.Errorf("MoveCard(%q, %q) = %v, want a Refusal", to, board, err)
				continue
			}
			if !strings.Contains(err.Error(), "--to with the column id") || !strings.Contains(err.Error(), "c") {
				t.Errorf("message: %v", err)
			}
			for _, c := range s.calls {
				if c.Method != http.MethodGet {
					t.Errorf("%q sent %s %s", to, c.Method, c.Path)
				}
			}
		}
	}
	s.calls = nil
	if err := f.MoveCard(87, "c1", "b1"); err != nil {
		t.Fatalf("by id: %v", err)
	}
	if last := s.calls[len(s.calls)-1]; last.Method != http.MethodPost || last.Path != "/1/cards/87/triage" || !strings.Contains(string(last.Body), `"c1"`) {
		t.Errorf("by id sent %s %s %s", last.Method, last.Path, last.Body)
	}
}

// With no custom column of that name, the keywords behave as before.
func TestMoveCardKeywordsWithoutReservedColumn(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		switch {
		case r.Method != http.MethodGet:
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/columns"):
			io.WriteString(w, `[{"id":"c4","name":"Next"},{"id":"c5","name":"Done soon"}]`)
		default:
			io.WriteString(w, `{"number":87,"board":{"id":"b1","name":"Home"}}`)
		}
	})
	for to, want := range map[string]string{"done": "POST /1/cards/87/closure", "Maybe?": "DELETE /1/cards/87/triage", "not now": "POST /1/cards/87/not_now"} {
		s.calls = nil
		if err := f.MoveCard(87, to, ""); err != nil {
			t.Fatalf("%q: %v", to, err)
		}
		last := s.calls[len(s.calls)-1]
		if got := last.Method + " " + last.Path; got != want {
			t.Errorf("%q sent %s, want %s", to, got, want)
		}
	}
}

// CheckMoveTarget lets a caller refuse before it writes anything else.
func TestCheckMoveTarget(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		io.WriteString(w, `[{"id":"c1","name":"done"}]`)
	})
	if err := f.CheckMoveTarget("b1", "Done"); !IsRefusal(err) {
		t.Errorf("Done: %v", err)
	}
	if err := f.CheckMoveTarget("b1", "maybe"); err != nil {
		t.Errorf("maybe: %v", err)
	}
	s.calls = nil
	for _, to := range []string{"Next", ""} {
		if err := f.CheckMoveTarget("b1", to); err != nil {
			t.Errorf("%q: %v", to, err)
		}
	}
	if len(s.calls) != 0 {
		t.Errorf("a target that is not a keyword read %d times", len(s.calls))
	}
}
