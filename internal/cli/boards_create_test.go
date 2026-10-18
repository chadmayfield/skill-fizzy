package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A board create reply with no Location leaves fz without the id it needs
// for the columns. fz says so and exits 1, and never reports success with
// the columns skipped.
func TestBoardCreateColumnsWithoutLocation(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	for _, args := range [][]string{
		{"board", "create", "Gate", "--columns", "Later,Now"},
		{"board-create", "Gate", "--columns", "Later,Now"},
	} {
		calls = nil
		r := runGo(args)
		want := "fz: board 'Gate' was created, but Fizzy sent no id for it, so its columns were not created\n"
		if r.code != 1 || r.stdout != "" || r.stderr != want {
			t.Errorf("%q: %+v", args, r)
		}
		if len(calls) != 1 {
			t.Errorf("%q sent %q", args, calls)
		}
	}
	calls = nil
	r := runGo([]string{"board", "create", "Gate", "--postpone", "30", "--columns", "Later"})
	if r.code != 1 || !strings.Contains(r.stderr, "so the auto-postpone period was not set and its columns were not created") {
		t.Errorf("both: %+v", r)
	}
	// With no columns and no period, no id is needed.
	r = runGo([]string{"board", "create", "Gate"})
	if r.code != 0 {
		t.Errorf("plain: %+v", r)
	}
}
