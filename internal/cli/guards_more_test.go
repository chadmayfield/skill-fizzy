package cli

import (
	"net/http"
	"strings"
	"testing"
)

// An unknown word after a group and a bool flag is still an unknown
// command. Only a flag that takes a value skips the next word.
func TestUnknownSubcommandAfterBoolFlag(t *testing.T) {
	for _, argv := range [][]string{
		{"card", "--dry-run", "nope"},
		{"card", "--json", "nope"},
		{"card", "--config", "x.json", "nope"},
	} {
		r := runGo(argv)
		if r.code != 2 || !strings.Contains(r.stderr, `unknown command "nope"`) {
			t.Errorf("fz %q: %+v", argv, r)
		}
	}
}

// board create sends the name without the spaces around it.
func TestBoardCreateTrimsName(t *testing.T) {
	srv, calls := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/1/boards/b9.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	r := runGo([]string{"board", "create", "  Patio  "})
	c := calls()
	if r.code != 0 || r.stdout != "board 'Patio' created: b9\n" || len(c) != 1 || c[0].body != `{"board":{"name":"Patio"}}` {
		t.Errorf("got %+v, calls %+v", r, c)
	}
}
