package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Text that would print an empty list or None says what is missing, or
// leaves the field out.
func TestEmptyFieldsInText(t *testing.T) {
	card := `{"number":87,"title":"Fix the gate","board":null}`
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/cards/87":
			io.WriteString(w, card)
		case "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
		case "/1/boards/b1/columns":
			io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	parityEnv(t, srv)
	r := runGo([]string{"card", "move", "87", "--to", "Now", "--board", "Home"})
	if r.code != 1 || r.stderr != "fz: no column 'Now' (no custom columns)\n" {
		t.Errorf("move: %+v", r)
	}
	r = runGo([]string{"card", "show", "87"})
	if r.code != 0 || r.stdout != "#87    [maybe] Fix the gate\n" {
		t.Errorf("show, no board or url: %+v", r)
	}
	card = `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"}}`
	if r = runGo([]string{"card", "show", "87"}); r.stdout != "#87    [maybe] Fix the gate\nboard: Home\n" {
		t.Errorf("show, no url: %+v", r)
	}
	card = `{"number":87,"title":"Fix the gate","url":"https://fizzy.example.com/1/cards/87"}`
	if r = runGo([]string{"card", "show", "87"}); r.stdout != "#87    [maybe] Fix the gate\nurl: https://fizzy.example.com/1/cards/87\n" {
		t.Errorf("show, no board: %+v", r)
	}
	if strings.Contains(r.stdout, "None") {
		t.Errorf("printed None: %q", r.stdout)
	}
}
