package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Gate and hinge samples with the live key names.
const (
	acctUser     = `{"id":"U1","name":"Alex","role":"owner","active":true,"email_address":"alex@example.com","created_at":"2026-01-01T00:00:00.000Z","url":"https://fizzy.example.com/1/users/U1","avatar_url":"a"}`
	acctUser2    = `{"id":"U2","name":"Sam","role":"member","active":false,"email_address":"sam@example.com","created_at":"2026-01-01T00:00:00.000Z","url":"u","avatar_url":"a"}`
	acctIdentity = `{"id":"I1","accounts":[{"id":"A2","name":"Other","slug":"/2","created_at":"x","user":` + acctUser2 + `},{"id":"A1","name":"Gate Works","slug":"/1","created_at":"x","user":` + acctUser + `}]}`
	acctSettings = `{"id":"A1","name":"Gate Works","cards_count":5,"created_at":"2026-01-01T00:00:00.000Z","auto_postpone_period_in_days":30}`
)

// acctServer answers "METHOD /path" routes with a JSON body and records
// every call; the path in a recorded call includes the query. An empty body
// answers 204 and a missing route 404. A body of "Location: /path" answers
// 201 with that path on the server as its Location.
func acctServer(t *testing.T, routes map[string]string) (*httptest.Server, func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []seenCall
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seenCall{r.Method, r.URL.RequestURI(), string(b)})
		mu.Unlock()
		body, ok := routes[r.Method+" "+r.URL.Path]
		switch {
		case !ok:
			w.WriteHeader(404)
		case body == "":
			w.WriteHeader(204)
		case strings.HasPrefix(body, "Location: "):
			w.Header().Set("Location", srv.URL+strings.TrimPrefix(body, "Location: "))
			w.WriteHeader(201)
		default:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, body)
		}
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	return srv, func() []seenCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenCall(nil), calls...)
	}
}

// callList formats calls as "METHOD path body" lines.
func callList(cs []seenCall) string {
	lines := make([]string, len(cs))
	for i, c := range cs {
		lines[i] = strings.TrimSpace(c.method + " " + c.path + " " + c.body)
	}
	return strings.Join(lines, "\n")
}

// hasCommands fails the test for each path that is not a runnable command.
func hasCommands(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		cmd, _, err := NewRoot().Find(strings.Fields(p)[1:])
		if err != nil || cmd.CommandPath() != p || !cmd.Runnable() {
			t.Errorf("missing command %q", p)
		}
	}
}

func TestAccountCommandsExist(t *testing.T) {
	hasCommands(t, "fz account show", "fz account postpone", "fz export account", "fz export mine", "fz export status")
}

func TestAccountShow(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/account/settings": acctSettings})
	r := runGo([]string{"account", "show"})
	want := "Gate Works  A1\ncards: 5\nauto-postpone: 30 days\ncreated: 2026-01-01T00:00:00.000Z\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "account", "show"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "{\n  \"id\": \"A1\",\n  \"name\": \"Gate Works\",") {
		t.Fatalf("json %+v", r)
	}
}

func TestAccountPostponeNeedsYes(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/account/settings": acctSettings, "PUT /1/account/entropy": acctSettings})
	r := runGo([]string{"account", "postpone", "90"})
	if r.code != 2 || r.stdout != "" ||
		r.stderr != "about to change the auto-postpone period of Gate Works from 30 to 90 days\nrefusing without --yes\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/account/settings" {
		t.Errorf("calls = %q", got)
	}
}

func TestAccountPostponeYes(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/account/settings": acctSettings, "PUT /1/account/entropy": acctSettings})
	r := runGo([]string{"account", "postpone", "90", "--yes"})
	if r.code != 0 || r.stdout != "account auto-postpone set to 90 days\n" {
		t.Fatalf("got %+v", r)
	}
	want := "GET /1/account/settings\nPUT /1/account/entropy {\"entropy\":{\"auto_postpone_period_in_days\":90}}"
	if got := callList(calls()); got != want {
		t.Errorf("calls = %q", got)
	}
}

func TestAccountPostponeDryRun(t *testing.T) {
	srv, calls := acctServer(t, map[string]string{"GET /1/account/settings": acctSettings})
	r := runGo([]string{"--dry-run", "--json", "account", "postpone", "90", "--yes"})
	if r.code != 0 || r.stdout != "{\n  \"auto_postpone_period_in_days\": 90,\n  \"dry_run\": true\n}\n" {
		t.Fatalf("got %+v", r)
	}
	if !strings.Contains(r.stderr, "[dry-run] PUT "+srv.URL+`/1/account/entropy {"entropy": {"auto_postpone_period_in_days": 90}}`) {
		t.Errorf("stderr = %q", r.stderr)
	}
	if got := callList(calls()); got != "GET /1/account/settings" {
		t.Errorf("calls = %q", got)
	}
}

func TestAccountPostponeBadDays(t *testing.T) {
	_, calls := acctServer(t, map[string]string{})
	for _, d := range []string{"x", "0", "-3", "14"} {
		r := runGo([]string{"account", "postpone", "--yes", "--", d})
		if r.code != 2 || !strings.HasPrefix(r.stderr, "fz: ") {
			t.Errorf("%s: got %+v", d, r)
		}
	}
	r := runGo([]string{"account", "postpone", "14", "--yes"})
	if r.stderr != "fz: auto-postpone must be one of (3, 7, 11, 30, 90, 365)\n" {
		t.Errorf("14: stderr = %q", r.stderr)
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestExportAccountAndMine(t *testing.T) {
	pending := `{"id":"E1","status":"pending","created_at":"2026-01-01T00:00:00Z"}`
	_, calls := acctServer(t, map[string]string{
		"POST /1/account/exports":         pending,
		"GET /my/identity":                acctIdentity,
		"POST /1/users/U1/data_exports":   pending,
		"GET /1/account/exports/E1":       `{"id":"E1","status":"completed","created_at":"2026-01-01T00:00:00Z","download_url":"https://fizzy.example.com/e.zip"}`,
		"GET /1/users/U1/data_exports/E1": pending,
	})
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"export", "account"}, "E1 pending 2026-01-01T00:00:00Z\n"},
		{[]string{"export", "mine"}, "E1 pending 2026-01-01T00:00:00Z\n"},
		{[]string{"export", "status", "E1"}, "E1 completed 2026-01-01T00:00:00Z https://fizzy.example.com/e.zip\n"},
		{[]string{"export", "status", "E1", "--mine"}, "E1 pending 2026-01-01T00:00:00Z\n"},
	}
	for _, c := range cases {
		if r := runGo(c.args); r.code != 0 || r.stdout != c.want {
			t.Errorf("%q: got %+v", c.args, r)
		}
	}
	want := "POST /1/account/exports\nGET /my/identity\nPOST /1/users/U1/data_exports\nGET /1/account/exports/E1\nGET /my/identity\nGET /1/users/U1/data_exports/E1"
	if got := callList(calls()); got != want {
		t.Errorf("calls =\n%s", got)
	}
}

func TestExportDryRun(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /my/identity": acctIdentity})
	r := runGo([]string{"--dry-run", "export", "mine"})
	if r.code != 0 || r.stdout != "dry-run: nothing sent; would start your export\n" || !strings.Contains(r.stderr, "[dry-run] POST ") {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /my/identity" {
		t.Errorf("calls = %q", got)
	}
}
