package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// poisonedName holds an SGR color, an OSC 8 link, a bell, a carriage
// return, a newline, a tab and a bidi override, as any account member could
// set a display name or a tag title.
var poisonedName = "P\x1b[31mQ\x1b]8;;https://fizzy.example.com/\aL\x1b]8;;\a\a\rR\nS\tT" + string(rune(0x202e)) + "U"

// Names and titles from the server inside an error message are cleaned,
// on one line, before they reach stderr.
func TestErrorTextIsCleaned(t *testing.T) {
	users, _ := json.Marshal([]map[string]any{{"id": "u1", "name": poisonedName}})
	tags, _ := json.Marshal([]map[string]any{{"id": "t1", "title": poisonedName}})
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/users":
			w.Write(users)
		case "/1/tags":
			w.Write(tags)
		case "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
		case "/1/cards/87":
			io.WriteString(w, `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},"assignees":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"Not found"}`)
		}
	})
	parityEnv(t, srv)
	for _, argv := range [][]string{
		{"card", "assign", "87", "nobody"},
		{"card", "unassign", "87", "nobody"},
		{"card", "list", "--tag", "nope"},
		{"board", "access", "Home", "--users", "nobody", "--yes"},
	} {
		r := runGo(argv)
		if r.code == 0 || !strings.Contains(r.stderr, "PQLRSTU") {
			t.Errorf("fz %q: want an error naming the cleaned name, got %+v", argv, r)
		}
		if strings.ContainsAny(r.stderr, "\x1b\a\r\t"+string(rune(0x202e))) || strings.Count(r.stderr, "\n") != 1 {
			t.Errorf("fz %q printed %q", argv, r.stderr)
		}
	}
}

// An error made from any text is cleaned at the last step, before it is
// printed, even where no command cleaned it.
func TestRunCleansEveryError(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"boom `+"\\u001b]52;c;Zm9v\\u0007"+` here"}`)
	})
	parityEnv(t, srv)
	r := runGo([]string{"board", "list"})
	if r.code != 1 || strings.ContainsAny(r.stderr, "\x1b\a") {
		t.Errorf("got %+v", r)
	}
}
