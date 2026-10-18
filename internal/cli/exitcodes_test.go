package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A --replace phrase that is not in the description sends nothing, so it
// is a refusal and exits 2, like a phrase that matches twice.
func TestReplaceNotFoundIsRefusal(t *testing.T) {
	srv, calls := editServer(t, `{"number":87,"title":"Fix the gate","description_html":"<p>Oil the hinge.</p>"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "87", "--replace", "latch", "bolt"})
	if r.code != 2 || !strings.Contains(r.stderr, `"latch" not found`) {
		t.Errorf("got %+v", r)
	}
	for _, c := range calls() {
		if c.method != http.MethodGet {
			t.Errorf("sent %s %s", c.method, c.path)
		}
	}
}

// A board whose name matches exactly wins over one that matches only
// ignoring case, as for columns and tags. Two that match only ignoring
// case still exit 2.
func TestBoardExactNameWins(t *testing.T) {
	acctServer(t, map[string]string{
		"GET /1/boards":            `[{"id":"b1","name":"Home"},{"id":"b2","name":"home"},{"id":"b3","name":"Yard"},{"id":"b4","name":"YARD"}]`,
		"GET /1/boards/b2/columns": `[{"id":"c1","name":"Next"}]`,
	})
	r := runGo([]string{"column", "list", "home"})
	if r.code != 0 || !strings.Contains(r.stdout, "Next") {
		t.Errorf("exact name: %+v", r)
	}
	r = runGo([]string{"column", "list", "yard"})
	if r.code != 2 || !strings.Contains(r.stderr, "2 boards are named 'yard'; use the board id") {
		t.Errorf("two inexact names: %+v", r)
	}
}

// card move reads the card to find its board. A reply with no board id
// is refused with a message that says so, and nothing is sent.
func TestMoveCardWithoutBoardID(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/cards/87": `{"number":87,"title":"Fix the gate"}`})
	r := runGo([]string{"card", "move", "87", "--to", "Next"})
	if r.code != 2 || !strings.Contains(r.stderr, "card #87 names no board in Fizzy's reply") {
		t.Errorf("got %+v", r)
	}
	if len(calls()) != 1 {
		t.Errorf("calls: %v", calls())
	}
}

// A usage error exits 2 even when there is no config, because the check
// runs before the config loads.
func TestUsageBeforeConfig(t *testing.T) {
	for _, argv := range [][]string{
		{"card", "move", "87"},
		{"comment", "react", "87", "c1", ""},
		{"comment", "react", "87", "c1", "   "},
		{"card", "tag", "87", "#"},
		{"card", "tag", "87", " # ", "--remove"},
	} {
		r := runGo(argv)
		if r.code != 2 || strings.Contains(r.stderr, "FIZZY_URL") {
			t.Errorf("fz %q: %+v", argv, r)
		}
	}
}

// fz help with a topic that is no command is a usage error, exit 2, as
// fz nope is.
func TestHelpUnknownTopic(t *testing.T) {
	r := runGo([]string{"help", "nope"})
	if r.code != 2 || !strings.Contains(r.stderr, `unknown help topic`) {
		t.Errorf("got %+v", r)
	}
	if r := runGo([]string{"help", "card"}); r.code != 0 || !strings.Contains(r.stdout, "card") {
		t.Errorf("help card: %+v", r)
	}
	if r := runGo([]string{"help"}); r.code != 0 {
		t.Errorf("help: %+v", r)
	}
}

// A reply of the wrong shape says which request it was and that the
// reply was not what fz expected. It never names a Go type.
func TestDecodeErrorText(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/cards/87":
			io.WriteString(w, `[1,2]`)
		case "/1/cards":
			io.WriteString(w, `{"number":87}`)
		case "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":7}]`)
		case "/1/tags":
			io.WriteString(w, `[1`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	parityEnv(t, srv)
	for argv, want := range map[string]string{
		"card show 87": "fz: GET /1/cards/87: the reply was not what fz expected",
		"card list":    "fz: GET /1/cards: the reply was not what fz expected",
		"board list":   "fz: GET /1/boards: the reply was not what fz expected",
		"tag list":     "fz: GET /1/tags: the reply was not what fz expected",
	} {
		r := runGo(strings.Fields(argv))
		if r.code != 1 || !strings.HasPrefix(r.stderr, want) {
			t.Errorf("fz %s: %+v", argv, r)
		}
		for _, bad := range []string{"Go ", "fizzy.", "jsontext", "json:"} {
			if strings.Contains(r.stderr, bad) {
				t.Errorf("fz %s names %q: %q", argv, bad, r.stderr)
			}
		}
	}
}
