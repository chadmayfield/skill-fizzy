package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// forged is server text that tries to start a line of its own, with a
// newline, a carriage return and an escape sequence. In JSON.
const forged = `Gate\nFORGED line\r\u001b[31m`

// forgedServer answers every read the text commands make with names,
// titles and bodies that hold forged. It answers a card create with a
// Location, and other writes with 204.
func forgedServer(t *testing.T) {
	t.Helper()
	card := `{"number":87,"title":"` + forged + `","board":{"id":"b1","name":"` + forged + `"},` +
		`"column":{"id":"c1","name":"` + forged + `"},"tags":["` + forged + `"],"image_url":"https://fizzy.example.com/i.png",` +
		`"assignees":[{"id":"U1"}],"has_more_assignees":false,"steps":[]}`
	user := `{"id":"U1","name":"` + forged + `","role":"owner","active":true,"email_address":"` + forged + `","url":"u"}`
	comment := `{"id":"c1","created_at":"2026-01-02T03:04:05Z","body":{"plain_text":"Gate latches.\nFORGED body\n\nFORGED again","html":"<p>x</p>"},"creator":{"name":"` + forged + `"}}`
	hook := `{"id":"H1","name":"` + forged + `","payload_url":"https://hooks.example.com/gate","active":true,"subscribed_actions":["card_closed"],"created_at":"x"}`
	routes := map[string]string{
		"GET /1/boards":                           `[{"id":"b1","name":"` + forged + `"}]`,
		"GET /1/boards/b1":                        `{"id":"b1","name":"` + forged + `","all_access":false,"auto_postpone_period_in_days":30,"creator":{"name":"` + forged + `"},"user_ids":["U1"]}`,
		"GET /1/boards/b1/columns":                `[{"id":"c1","name":"` + forged + `","color":{"name":"` + forged + `","value":"var(--color-card-4)"}}]`,
		"GET /1/boards/b1/columns/c1":             `{"id":"c1","name":"` + forged + `","color":{"name":"` + forged + `","value":"var(--color-card-4)"}}`,
		"GET /1/boards/b1/columns/c1/cards":       "[" + card + "]",
		"GET /1/boards/b1/accesses":               `{"board_id":"b1","all_access":false,"users":[{"id":"U1","name":"` + forged + `","role":"owner","has_access":true,"involvement":"` + forged + `"}]}`,
		"GET /1/boards/b1/webhooks":               "[" + hook + "]",
		"GET /1/boards/b1/webhooks/H1":            hook,
		"GET /1/boards/b1/webhooks/H1/deliveries": `[{"id":"D1","state":"` + forged + `","event":{"action":"` + forged + `"},"response":{"code":500,"error":"` + forged + `"}}]`,
		"GET /1/cards":                            "[" + card + "]",
		"GET /1/cards/87":                         card,
		"GET /1/my/pins":                          "[" + card + "]",
		"GET /1/cards/87/reactions":               `[{"id":"r1","content":"` + forged + `","reacter":{"id":"U1","name":"` + forged + `"}}]`,
		"GET /1/cards/87/comments":                "[" + comment + "]",
		"GET /1/cards/87/comments/c1":             comment,
		"GET /1/cards/87/comments/c1/reactions":   `[{"id":"r1","content":"` + forged + `","reacter":{"id":"U1","name":"` + forged + `"}}]`,
		"GET /1/users":                            "[" + user + "]",
		"GET /1/users/U1":                         user,
		"GET /1/tags":                             `[{"id":"t1","title":"` + forged + `"}]`,
		"GET /1/notifications":                    `[{"id":"N1","read":false,"created_at":"x","title":"` + forged + `","body":"` + forged + `"}]`,
		"GET /1/activities":                       `[{"id":"V1","action":"` + forged + `","created_at":"x","particulars":{"old_title":"` + forged + `","new_title":"` + forged + `"},"eventable_type":"` + forged + `","eventable":{"number":87}}]`,
		"GET /my/identity":                        `{"id":"I1","accounts":[{"id":"A1","name":"` + forged + `","slug":"/1","user":` + user + `}]}`,
		"GET /1/account/settings":                 `{"id":"A1","name":"` + forged + `","cards_count":5,"created_at":"x","auto_postpone_period_in_days":30}`,
		"GET /1/account/exports/E1":               `{"id":"E1","status":"` + forged + `","created_at":"x","download_url":"https://fizzy.example.com/e"}`,
	}
	var srvURL string
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/cards" {
			w.Header().Set("Location", srvURL+"/1/cards/87.json")
			w.WriteHeader(http.StatusCreated)
			return
		}
		body, ok := routes[r.Method+" "+r.URL.Path]
		switch {
		case ok:
			io.WriteString(w, body)
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srvURL = srv.URL
	parityEnv(t, srv)
}

// testForgedLines checks that no server text in text mode can start a line
// of its own, or carry an escape, a carriage return or a bell. Single-line
// text is joined onto its line. A comment body keeps its newlines, with
// every line indented under the head line. card show prints the description as
// it is, which the parity golden pins.
func testForgedLines(t *testing.T) {
	forgedServer(t)
	for _, argv := range [][]string{
		{"card", "show", "87"},
		{"card", "list"},
		{"card", "pins"},
		{"card", "boosts", "87"},
		{"card", "delete", "87"},
		{"card", "image", "87", "--remove"},
		{"card", "assign", "87", "U1"},
		{"card", "create", "b1", "Fix the gate", "--tag", "buy"},
		{"column", "show", "b1", "c1"},
		{"column", "cards", "b1", "c1"},
		{"column", "list", "b1"},
		{"column", "delete", "b1", "c1"},
		{"--dry-run", "column", "edit", "b1", "c1", "--name", "Later"},
		{"board", "list"},
		{"board", "show", "b1"},
		{"board", "delete", "b1"},
		{"board", "publish", "b1"},
		{"board", "access", "b1"},
		{"board", "access", "b1", "--users", "U1,U2"},
		{"--dry-run", "board", "edit", "b1", "--name", "Garden"},
		{"comment", "list", "87"},
		{"comment", "show", "87", "c1"},
		{"comment", "delete", "87", "c1"},
		{"comment", "reactions", "87", "c1"},
		{"user", "list"},
		{"user", "show", "U1"},
		{"user", "edit", "U1", "--name", "Gate Keeper"},
		{"tag", "list"},
		{"notification", "list"},
		{"activity", "list"},
		{"whoami"},
		{"account", "show"},
		{"account", "postpone", "90"},
		{"export", "status", "E1"},
		{"webhook", "list", "b1"},
		{"webhook", "show", "b1", "H1"},
		{"webhook", "deliveries", "b1", "H1"},
		{"webhook", "delete", "b1", "H1"},
		{"webhook", "edit", "b1", "H1", "--name", "Hinge alerts"},
		{"review", "b1"},
	} {
		r := runGo(argv)
		if r.code != 0 && r.code != 2 {
			t.Errorf("fz %q: exit %d, stderr %q", argv, r.code, r.stderr)
		}
		for _, out := range []string{r.stdout, r.stderr} {
			if strings.ContainsAny(out, "\x1b\r\x07") {
				t.Errorf("fz %q printed an escape: %q", argv, out)
			}
			for _, l := range strings.Split(out, "\n") {
				if strings.HasPrefix(l, "FORGED") {
					t.Errorf("fz %q printed a forged line: %q", argv, out)
					break
				}
			}
		}
	}
}

// A comment body keeps its lines, each one indented by two spaces under
// the head line, the first one too.
func TestCommentBodyIndented(t *testing.T) {
	forgedServer(t)
	r := runGo([]string{"comment", "show", "87", "c1"})
	want := serverDataLine + "\nc1  GateFORGED line  2026-01-02T03:04:05Z\n  Gate latches.\n  FORGED body\n\n  FORGED again\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("got %+v\nwant %q", r, want)
	}
}
