package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func reviewServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/boards":
			fmt.Fprint(w, `[{"id":"B1","name":"Garage","auto_postpone_period_in_days":90}]`)
		case "/1/boards/B1/columns":
			fmt.Fprint(w, `[{"id":"C1","name":"Now","color":{"name":"Blue","value":"var(--color-card-default)"}}]`)
		case "/1/boards/B1/columns/C1/cards":
			fmt.Fprint(w, `[{"number":87,"title":"Order the hinge","closed":false}]`)
		case "/1/cards":
			if r.URL.Query().Get("indexed_by") == "golden" {
				fmt.Fprint(w, `[{"number":87,"title":"Order the hinge","closed":false}]`)
				return
			}
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestReviewCommand(t *testing.T) {
	parityEnv(t, reviewServer(t))
	got := runGo([]string{"review", "garage"})
	want := "== Garage (postpone 90d)\nNow (1):\n  #87 Order the hinge\nMaybe? 0\n" +
		"Golden (1): #87 Order the hinge\nPostponing soon (0): none\nStalled (0): none\n"
	if got.stdout != want || got.code != 0 {
		t.Errorf("got %+v\nwant out %q", got, want)
	}
	if all := runGo([]string{"review"}); all != got {
		t.Errorf("no argument should review every board:\n%+v", all)
	}
}

func TestReviewJSON(t *testing.T) {
	parityEnv(t, reviewServer(t))
	got := runGo([]string{"--json", "review", "Garage"})
	if got.code != 0 {
		t.Fatalf("%+v", got)
	}
	last := -1
	for _, k := range []string{`"board"`, `"postpone_days"`, `"lanes"`, `"maybe"`, `"golden"`, `"postponing_soon"`, `"stalled"`} {
		i := strings.Index(got.stdout, k)
		if i <= last {
			t.Fatalf("key %s missing or out of order:\n%s", k, got.stdout)
		}
		last = i
	}
	if !strings.Contains(got.stdout, `"name": "Now"`) || !strings.Contains(got.stdout, `"number": 87`) {
		t.Errorf("lane or card missing:\n%s", got.stdout)
	}
}

func TestReviewUnknownBoard(t *testing.T) {
	parityEnv(t, reviewServer(t))
	if got := runGo([]string{"review", "Nope"}); got.code == 0 || !strings.Contains(got.stderr, "no board named 'Nope'") {
		t.Errorf("got %+v", got)
	}
}

func TestReviewHelp(t *testing.T) {
	for _, c := range NewRoot().Commands() {
		if c.Name() == "review" {
			if c.Short == "" || c.Long == "" || c.Example == "" {
				t.Error("review needs Short, Long and Example")
			}
			return
		}
	}
	t.Error("review command not registered")
}
