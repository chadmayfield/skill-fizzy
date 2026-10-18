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

// A failure after the board exists says the board was created, with its
// id, names the step that did not finish and gives the commands that
// finish it. It exits 1, so a retry does not make a second board.
func TestBoardCreateLaterStepFails(t *testing.T) {
	for _, c := range []struct {
		name, failPath string
		want           string
	}{
		{"period", "PUT /1/boards/b9",
			"fz: board 'Gate yard' was created (id b9), but setting the auto-postpone period did not finish: PUT /1/boards/b9 -> HTTP 422: {\"error\":\"no\"}; " +
				"to finish, run: fz board edit b9 --postpone 30; fz column create b9 Later; fz column create b9 'Side gate'\n"},
		{"second column", "POST /1/boards/b9/columns#2",
			"fz: board 'Gate yard' was created (id b9), but creating column 'Side gate' did not finish: POST /1/boards/b9/columns -> HTTP 422: {\"error\":\"no\"}; " +
				"to finish, run: fz column create b9 'Side gate'\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var calls []string
			cols := 0
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				key := r.Method + " " + r.URL.Path
				calls = append(calls, key)
				if key == "POST /1/boards/b9/columns" {
					cols++
					if cols == 2 {
						key += "#2"
					}
				}
				switch {
				case key == c.failPath:
					w.WriteHeader(http.StatusUnprocessableEntity)
					w.Write([]byte(`{"error":"no"}`))
				case key == "POST /1/boards":
					w.Header().Set("Location", srv.URL+"/1/boards/b9.json")
					w.WriteHeader(http.StatusCreated)
				case strings.HasPrefix(key, "POST /1/boards/b9/columns"):
					w.Header().Set("Location", srv.URL+"/1/boards/b9/columns/c9.json")
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			t.Cleanup(srv.Close)
			parityEnv(t, srv)
			r := runGo([]string{"board", "create", "Gate yard", "--postpone", "30", "--columns", "Later,Side gate"})
			if r.code != 1 || r.stdout != "" || r.stderr != c.want {
				t.Errorf("got %+v\nwant stderr %q", r, c.want)
			}
		})
	}
}
