package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/client"
)

// A card create whose Location names card 0 is an error, not a dry run.
func TestCreateLocationCardZero(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/cards":
			w.Header().Set("Location", "/1/cards/0")
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	parityEnv(t, srv)
	r := runGo([]string{"card", "create", "Home", "Fix the gate"})
	if r.code != 1 || strings.Contains(r.stdout, "dry-run") {
		t.Errorf("got %+v", r)
	}
}

// Text written to stderr goes through the terminal cleaner too. draft
// apply prints its failure there itself, and the draft's file name, which
// can hold any byte but / and NUL, reaches the terminal without escapes.
func TestStderrIsCleaned(t *testing.T) {
	srv, _ := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "{\"error\":\"\x1b[31mbad\x07 hinge\"}")
	})
	parityEnv(t, srv)
	path := filepath.Join(t.TempDir(), "gate\x1b[31m\x07.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n")
	r := runGo([]string{"draft", "apply", path})
	if r.code != 1 || !strings.Contains(r.stderr, "gate.md") || strings.ContainsAny(r.stderr, "\x1b\x07") {
		t.Errorf("got %q", r.stderr)
	}
}

// card create --column maybe makes the card where it lands and sends no
// move after it.
func TestCreateColumnMaybeSendsNoMove(t *testing.T) {
	routes := map[string]string{}
	for k, v := range dryRunRoutes {
		routes[k] = v
	}
	routes["POST /1/boards/b1/cards"] = "Location: /1/cards/87.json"
	_, calls := acctServer(t, routes)
	r := runGo([]string{"card", "create", "Home", "Fix the gate", "--column", "maybe"})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	for _, c := range calls() {
		if c.method != "GET" && c.path != "/1/boards/b1/cards" {
			t.Errorf("sent %s %s", c.method, c.path)
		}
	}
}

// Only a refused dial proves a create never left fz. A read or write
// error on the connection may come after the server got the request.
func TestCreateNotSentOps(t *testing.T) {
	inner := errors.New("boom")
	for op, want := range map[string]bool{"dial": true, "write": false, "read": false} {
		err := fmt.Errorf("POST: %w", &net.OpError{Op: op, Net: "tcp", Err: inner})
		if got := createNotSent(err); got != want {
			t.Errorf("%s: createNotSent = %v, want %v", op, got, want)
		}
	}
	for status, want := range map[int]bool{302: false, 404: true, 422: true, 500: false} {
		if got := createNotSent(&client.APIError{Status: status}); got != want {
			t.Errorf("HTTP %d: createNotSent = %v, want %v", status, got, want)
		}
	}
}

// An update whose board move went through and whose title and description
// PUT then failed is a partial sync: synced: is recorded from the card as
// it is, and the message says how to finish it.
func TestSyncBoardMoveThenPutFails(t *testing.T) {
	text := strings.Replace(syncDraftText, "board: Home\n", "board: Garage\n", 1)
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, text))
	srv.failNext("PUT /1/cards/87", 1)
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 1 || !strings.Contains(got.stderr, "the update stopped partway") || !strings.Contains(got.stderr, "Finish it with") {
		t.Errorf("got %+v", got)
	}
}
