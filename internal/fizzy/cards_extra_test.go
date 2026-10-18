package fizzy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const usersJSON = `[
 {"id":"u1","name":"Gate Keeper","role":"owner","active":true,"email_address":"keeper@fizzy.example.com","created_at":"2026-01-01T00:00:00Z","url":"u","avatar_url":"a"},
 {"id":"u2","name":"Hinge Smith","role":"member","active":true,"email_address":"hinge@fizzy.example.com","created_at":"2026-01-01T00:00:00Z","url":"u","avatar_url":"a"},
 {"id":"u3","name":"Twin","role":"member","active":true,"email_address":"twin1@fizzy.example.com","created_at":"2026-01-01T00:00:00Z","url":"u","avatar_url":"a"},
 {"id":"u4","name":"Twin","role":"member","active":true,"email_address":"twin2@fizzy.example.com","created_at":"2026-01-01T00:00:00Z","url":"u","avatar_url":"a"}
]`

// cardServer answers GET /1/users with usersJSON, GET /1/cards/87 with card,
// and every other request with 204.
func cardServer(card string) func(w http.ResponseWriter, r *http.Request, b []byte) {
	return func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1/users":
			io.WriteString(w, usersJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/1/cards/87":
			io.WriteString(w, card)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func TestAssignSendsOnlyWhenNeeded(t *testing.T) {
	card := `{"number":87,"assignees":[{"id":"u1","name":"Gate Keeper"}],"has_more_assignees":false}`
	f, s, _, _ := newFizzy(t, false, cardServer(card))

	m, changed, err := f.Assign(87, "Gate Keeper")
	if err != nil || changed || m.ID != "u1" {
		t.Fatalf("assign present: %+v %v %v", m, changed, err)
	}
	for _, c := range s.calls {
		if c.Method == http.MethodPost {
			t.Fatalf("sent %s %s for a user already assigned", c.Method, c.Path)
		}
	}

	s.calls = nil
	m, changed, err = f.Assign(87, "u2")
	if err != nil || !changed || m.ID != "u2" {
		t.Fatalf("assign absent: %+v %v %v", m, changed, err)
	}
	last := s.calls[len(s.calls)-1]
	if last.Method != http.MethodPost || last.Path != "/1/cards/87/assignments" || string(last.Body) != `{"assignee_id":"u2"}` {
		t.Errorf("assign call %s %s %s", last.Method, last.Path, last.Body)
	}

	s.calls = nil
	if _, changed, err = f.Unassign(87, "u2"); err != nil || changed {
		t.Fatalf("unassign absent: %v %v", changed, err)
	}
	if len(s.calls) != 2 {
		t.Errorf("unassign absent sent %d calls, want 2 GETs", len(s.calls))
	}

	s.calls = nil
	if _, changed, err = f.Unassign(87, "keeper@fizzy.example.com"); err != nil || !changed {
		t.Fatalf("unassign present: %v %v", changed, err)
	}
	last = s.calls[len(s.calls)-1]
	if last.Method != http.MethodPost || string(last.Body) != `{"assignee_id":"u1"}` {
		t.Errorf("unassign call %s %s %s", last.Method, last.Path, last.Body)
	}
}

// The card JSON lists at most five assignees. When it says there are more
// and the user is not among the five, fz cannot tell, so it sends nothing.
func TestAssignRefusesWhenListIsCut(t *testing.T) {
	card := `{"number":87,"assignees":[{"id":"u1"}],"has_more_assignees":true}`
	f, s, _, _ := newFizzy(t, false, cardServer(card))
	for _, op := range []func(int, string) (UserMatch, bool, error){f.Assign, f.Unassign} {
		_, _, err := op(87, "u2")
		if err == nil || !strings.Contains(err.Error(), "more than 5 assignees") {
			t.Errorf("err = %v", err)
		}
	}
	// A user among the five is still known.
	if _, changed, err := f.Assign(87, "u1"); err != nil || changed {
		t.Errorf("listed user: %v %v", changed, err)
	}
	for _, c := range s.calls {
		if c.Method != http.MethodGet {
			t.Errorf("sent %s %s", c.Method, c.Path)
		}
	}
}

func TestAssignDryRun(t *testing.T) {
	f, s, out, _ := newFizzy(t, true, cardServer(`{"assignees":[],"has_more_assignees":false}`))
	if _, changed, err := f.Assign(87, "u2"); err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	for _, c := range s.calls {
		if c.Method != http.MethodGet {
			t.Errorf("dry run sent %s %s", c.Method, c.Path)
		}
	}
	if !strings.Contains(out.String(), `[dry-run] POST `) || !strings.Contains(out.String(), `/1/cards/87/assignments {"assignee_id": "u2"}`) {
		t.Errorf("dry-run line %q", out.String())
	}
}

func TestWatchPinImageCalls(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, cardServer("{}"))
	steps := []struct {
		run          func(int) error
		method, path string
	}{
		{f.Watch, "POST", "/1/cards/87/watch"},
		{f.Unwatch, "DELETE", "/1/cards/87/watch"},
		{f.Pin, "POST", "/1/cards/87/pin"},
		{f.Unpin, "DELETE", "/1/cards/87/pin"},
		{f.RemoveImage, "DELETE", "/1/cards/87/image"},
	}
	for i, st := range steps {
		if err := st.run(87); err != nil {
			t.Fatal(err)
		}
		c := s.calls[i]
		if c.Method != st.method || c.Path != st.path || len(c.Body) != 0 {
			t.Errorf("step %d: %s %s %q", i, c.Method, c.Path, c.Body)
		}
	}
}

func TestPins(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		io.WriteString(w, `[{"number":87,"title":"Fix the gate","tags":["buy"]},{"number":88,"title":"Oil the hinge"}]`)
	})
	cards, err := f.Pins()
	if err != nil || len(cards) != 2 || cards[1].Number != 88 {
		t.Fatalf("%+v %v", cards, err)
	}
	if s.calls[0].Path != "/1/my/pins" || len(s.calls) != 1 {
		t.Errorf("calls %+v", s.calls)
	}
	if string(cards[0].Raw()) != `{"number":87,"title":"Fix the gate","tags":["buy"]}` {
		t.Errorf("raw %s", cards[0].Raw())
	}
}

func TestBoosts(t *testing.T) {
	list := `[{"id":"r1","content":"nice","reacter":{"id":"u1","name":"Gate Keeper"},"url":"x"}]`
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method == http.MethodGet {
			io.WriteString(w, list)
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":"r2","content":"ok"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	bs, err := f.Boosts(87)
	if err != nil || len(bs) != 1 || bs[0].ID != "r1" || bs[0].Content != "nice" || bs[0].Reacter.Name != "Gate Keeper" {
		t.Fatalf("%+v %v", bs, err)
	}
	if string(bs[0].Raw()) != `{"id":"r1","content":"nice","reacter":{"id":"u1","name":"Gate Keeper"},"url":"x"}` {
		t.Errorf("raw %s", bs[0].Raw())
	}
	raw, err := f.AddBoost(87, "Great 👍")
	if err != nil || string(raw) != `{"id":"r2","content":"ok"}` {
		t.Fatalf("add: %s %v", raw, err)
	}
	if err := f.RemoveBoost(87, "r1"); err != nil {
		t.Fatal(err)
	}
	if c := s.calls[1]; c.Method != "POST" || c.Path != "/1/cards/87/reactions" || unHTML(c.Body) != `{"reaction":{"content":"Great 👍"}}` {
		t.Errorf("add call %s %s %s", c.Method, c.Path, c.Body)
	}
	if c := s.calls[2]; c.Method != "DELETE" || c.Path != "/1/cards/87/reactions/r1" {
		t.Errorf("remove call %s %s", c.Method, c.Path)
	}
}

// Fizzy stores a boost in a VARCHAR(16) column, which counts code points, so
// 16 emoji fit and 17 ASCII letters do not.
func TestAddBoostLength(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, cardServer("{}"))
	if _, err := f.AddBoost(87, strings.Repeat("a", 17)); err == nil || !strings.Contains(err.Error(), "this one is 17") {
		t.Errorf("17 letters: %v", err)
	}
	if _, err := f.AddBoost(87, ""); err == nil {
		t.Error("empty boost accepted")
	}
	if len(s.calls) != 0 {
		t.Errorf("sent %d calls for a bad boost", len(s.calls))
	}
	if _, err := f.AddBoost(87, strings.Repeat("👍", 16)); err != nil {
		t.Errorf("16 emoji: %v", err)
	}
	if BoostLength("é👍a") != 3 {
		t.Errorf("BoostLength = %d", BoostLength("é👍a"))
	}
}

func TestFindTag(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		io.WriteString(w, `[{"id":"t1","title":"buy"},{"id":"t2","title":"waiting"}]`)
	})
	for _, in := range []string{"waiting", "#Waiting", " WAITING "} {
		tg, err := f.FindTag(in)
		if err != nil || tg.ID != "t2" {
			t.Errorf("FindTag(%q) = %+v %v", in, tg, err)
		}
	}
	if _, err := f.FindTag("nope"); err == nil || !strings.Contains(err.Error(), "no tag 'nope' (have: buy, waiting)") {
		t.Errorf("missing: %v", err)
	}
}
