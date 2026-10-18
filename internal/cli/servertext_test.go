package cli

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// Server text with terminal escapes, as anyone who can edit a card on a
// shared account could write it.
const (
	nastyTitle = `Fix the gate\u001b[2K\r#87    [Now] Harmless title`
	nastyCard  = `{"number":87,"title":"` + nastyTitle + `","closed":false,` +
		`"board":{"id":"b1","name":"Home\u001b[31m"},"column":{"id":"c1","name":"Now\u001b]0;x\u0007"},"tags":["buy\r"],` +
		`"url":"https://fizzy.example.com/1/cards/87\u001b[8m",` +
		`"description":"Line one\nclip \u001b]52;c;Zm9v\u0007board\r\nend",` +
		`"steps":[{"id":"03abc123def456ghi789jkl0mn","content":"Order\u001b[1A\u001b[2K the hinge","completed":false}]}`
)

// serverTextServer serves nastyCard and escape-laden boards, columns and
// an upload reply.
func serverTextServer(t *testing.T) {
	t.Helper()
	var srvURL string
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/1/cards/87" || (r.URL.Path == "/1/cards" && r.Method == http.MethodGet):
			body := nastyCard
			if r.URL.Path == "/1/cards" {
				body = "[" + nastyCard + "]"
			}
			io.WriteString(w, body)
		case r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home\u001b[2J"}]`)
		case r.URL.Path == "/1/boards/b1/columns":
			io.WriteString(w, `[{"id":"c1","name":"Now\u001b[5m"}]`)
		case r.URL.Path == "/1/rails/active_storage/direct_uploads":
			io.WriteString(w, `{"signed_id":"B","attachable_sgid":"att\u001b[2Jsgid","direct_upload":{"url":"`+srvURL+`/put"}}`)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"Not found"}`)
		}
	})
	srvURL = srv.URL
	parityEnv(t, srv)
}

// No server text reaches the terminal with an escape character, a
// carriage return or a bell, in text mode. A title cannot start a new line.
func TestServerTextIsCleaned(t *testing.T) {
	serverTextServer(t)
	img := filepath.Join(t.TempDir(), "hinge.jpg")
	writeFile(t, img, "jpg")
	for _, argv := range [][]string{
		{"card", "show", "87"},
		{"card", "list"},
		{"card", "delete", "87"},
		{"step", "list", "87"},
		{"step", "delete", "87", "03abc123def456ghi789jkl0mn"},
		{"board", "list"},
		{"column", "list", "Home\x1b[2J"},
		{"column", "list", "Nope"},
		{"upload", img},
	} {
		r := runGo(argv)
		for _, out := range []string{r.stdout, r.stderr} {
			if strings.ContainsAny(out, "\x1b\r\x07") {
				t.Errorf("fz %q printed %q", argv, out)
			}
		}
	}
	r := runGo([]string{"card", "show", "87"})
	want := "#87    [Now] Fix the gate#87    [Now] Harmless title #buy\n" +
		"board: Home   url: https://fizzy.example.com/1/cards/87\n\n" +
		"Line one\nclip board\nend\n" +
		"  [ ] Order the hinge\n"
	if r.stdout != want {
		t.Errorf("card show:\n%q\nwant\n%q", r.stdout, want)
	}
}

// --json prints the server's text as it came, escaped as JSON.
func TestServerTextJSONUnchanged(t *testing.T) {
	serverTextServer(t)
	r := runGo([]string{"--json", "card", "show", "87"})
	if r.code != 0 || !strings.Contains(r.stdout, `Fix the gate\u001b[2K\r#87`) || !strings.Contains(r.stdout, `\u001b]52;c;Zm9v\u0007`) {
		t.Errorf("got %+v", r)
	}
}
