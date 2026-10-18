package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	actNoteUnread = `{"id":"N1","read":false,"read_at":null,"created_at":"2026-01-02T00:00:00.000Z","title":"Oil the hinge","body":"Assigned to you","creator":` + acctUser + `,"card":{"id":"K1","title":"Oil the hinge","status":"published","url":"u"},"url":"u"}`
	actNoteRead   = `{"id":"N2","read":true,"read_at":"2026-01-02T00:00:00.000Z","created_at":"2026-01-01T00:00:00.000Z","title":"Fix the gate","body":"","creator":` + acctUser + `,"card":{"id":"K2","title":"Fix the gate","status":"published","url":"u"},"url":"u"}`
	actBoards     = `[{"id":"B1","name":"Home"}]`
	actFeed       = `[` +
		`{"id":"V1","action":"card_title_changed","created_at":"2026-01-03T00:00:00.000Z","description":"d","particulars":{"old_title":"Gate","new_title":"Gate latch"},"url":"u","eventable_type":"Card","eventable":{"id":"K1","number":87,"title":"Gate latch"},"board":{"id":"B1","name":"Home"},"creator":` + acctUser + `},` +
		`{"id":"V2","action":"comment_created","created_at":"2026-01-02T00:00:00.000Z","description":"d","particulars":{},"url":"u","eventable_type":"Comment","eventable":{"id":"C1"},"board":{"id":"B1","name":"Home"},"creator":` + acctUser + `},` +
		`{"id":"V3","action":"card_triaged","created_at":"2026-01-01T00:00:00.000Z","description":"d","particulars":{"column":"Next"},"url":"u","eventable_type":"Card","eventable":{"id":"K1","number":87},"board":{"id":"B1","name":"Home"},"creator":` + acctUser + `},` +
		`{"id":"V4","action":"card_assigned","created_at":"2026-01-01T00:00:00.000Z","description":"d","particulars":{"assignee_ids":["U1","U2"]},"url":"u","eventable_type":"Card","eventable":{"id":"K1","number":87},"board":{"id":"B1","name":"Home"},"creator":` + acctUser + `}]`
)

func TestActivityCommandsExist(t *testing.T) {
	hasCommands(t, "fz activity list", "fz notification list", "fz notification read", "fz notification unread", "fz notification settings")
}

func TestNotificationList(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/notifications": "[" + actNoteUnread + "," + actNoteRead + "]"})
	r := runGo([]string{"notification", "list"})
	want := "N1 [unread] 2026-01-02T00:00:00.000Z  Oil the hinge: Assigned to you\nN2 [read] 2026-01-01T00:00:00.000Z  Fix the gate\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "notification", "list"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "[\n  {\n    \"id\": \"N1\",") {
		t.Fatalf("json %+v", r)
	}
}

func TestNotificationListEmpty(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/notifications": "[]"})
	if r := runGo([]string{"notification", "list"}); r.code != 0 || r.stdout != "(no notifications)\n" {
		t.Fatalf("got %+v", r)
	}
}

func TestNotificationReadUnreadAll(t *testing.T) {
	_, calls := acctServer(t, map[string]string{
		"POST /1/notifications/N1/reading":   "",
		"DELETE /1/notifications/N1/reading": "",
		"POST /1/notifications/bulk_reading": "",
	})
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"notification", "read", "N1"}, "notification N1 marked read\n"},
		{[]string{"notification", "unread", "N1"}, "notification N1 marked unread\n"},
		{[]string{"notification", "read", "--all"}, "all notifications marked read\n"},
	}
	for _, c := range cases {
		if r := runGo(c.args); r.code != 0 || r.stdout != c.want {
			t.Errorf("%q: got %+v", c.args, r)
		}
	}
	want := "POST /1/notifications/N1/reading\nDELETE /1/notifications/N1/reading\nPOST /1/notifications/bulk_reading"
	if got := callList(calls()); got != want {
		t.Errorf("calls =\n%s", got)
	}
}

func TestNotificationReadNeedsIDOrAll(t *testing.T) {
	_, calls := acctServer(t, map[string]string{})
	for _, argv := range [][]string{{"notification", "read"}, {"notification", "read", "N1", "--all"}} {
		if r := runGo(argv); r.code != 2 || !strings.HasPrefix(r.stderr, "fz: ") {
			t.Errorf("%q: got %+v", argv, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestNotificationReadDryRun(t *testing.T) {
	srv, calls := acctServer(t, map[string]string{})
	r := runGo([]string{"--dry-run", "notification", "read", "--all"})
	if r.code != 0 || r.stderr != "[dry-run] POST "+srv.URL+"/1/notifications/bulk_reading \n" {
		t.Fatalf("got %+v", r)
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestNotificationSettings(t *testing.T) {
	_, calls := acctServer(t, map[string]string{
		"GET /1/notifications/settings": `{"bundle_email_frequency":"every_few_hours"}`,
		"PUT /1/notifications/settings": "",
	})
	if r := runGo([]string{"notification", "settings"}); r.code != 0 || r.stdout != "email: every_few_hours\n" {
		t.Fatalf("show %+v", r)
	}
	if r := runGo([]string{"notification", "settings", "--email", "daily"}); r.code != 0 || r.stdout != "email set to daily\n" {
		t.Fatalf("set %+v", r)
	}
	want := "GET /1/notifications/settings\nPUT /1/notifications/settings {\"user_settings\":{\"bundle_email_frequency\":\"daily\"}}"
	if got := callList(calls()); got != want {
		t.Errorf("calls =\n%s", got)
	}
}

func TestNotificationSettingsBadEmail(t *testing.T) {
	_, calls := acctServer(t, map[string]string{})
	r := runGo([]string{"notification", "settings", "--email", "hourly"})
	if r.code != 2 || r.stderr != "fz: argument --email: invalid choice: 'hourly' (choose from 'never', 'every_few_hours', 'daily', 'weekly')\n" {
		t.Fatalf("got %+v", r)
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestActivityList(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/activities": actFeed})
	r := runGo([]string{"activity", "list"})
	want := "2026-01-03T00:00:00.000Z  card_title_changed  Card #87  title: Gate to Gate latch\n" +
		"2026-01-02T00:00:00.000Z  comment_created  Comment\n" +
		"2026-01-01T00:00:00.000Z  card_triaged  Card #87  column: Next\n" +
		"2026-01-01T00:00:00.000Z  card_assigned  Card #87  assignee_ids: U1, U2\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/activities" {
		t.Errorf("calls = %q", got)
	}
}

func TestActivityListFilters(t *testing.T) {
	_, calls := acctServer(t, map[string]string{
		"GET /1/activities": "[]", "GET /1/boards": actBoards,
		"GET /1/users": "[" + acctUser + "," + acctUser2 + "]",
	})
	for _, who := range []string{"U1", "alex", "ALEX@example.com"} {
		r := runGo([]string{"activity", "list", "--board", "Home", "--creator", who})
		if r.code != 0 || r.stdout != "(no activity)\n" {
			t.Fatalf("%s: got %+v", who, r)
		}
	}
	cs := calls()
	last := cs[len(cs)-1]
	if last.path != "/1/activities?board_ids%5B%5D=B1&creator_ids%5B%5D=U1" {
		t.Errorf("last call = %+v", last)
	}
}

func TestActivityListUnknownCreator(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "]"})
	r := runGo([]string{"activity", "list", "--creator", "Nobody"})
	if r.code != 2 || r.stderr != "fz: no user 'Nobody' (have: Alex)\n" {
		t.Fatalf("got %+v", r)
	}
}

// A deactivated user is not in GET /users, so an id that matches nobody is
// sent as it is.
func TestActivityListCreatorIDPassesThrough(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "]", "GET /1/activities": "[]"})
	r := runGo([]string{"activity", "list", "--creator", "03abc123def456ghi789jkl0mn"})
	if r.code != 0 || r.stdout != "(no activity)\n" {
		t.Fatalf("got %+v", r)
	}
	cs := calls()
	if last := cs[len(cs)-1]; last.path != "/1/activities?creator_ids%5B%5D=03abc123def456ghi789jkl0mn" {
		t.Errorf("last call = %+v", last)
	}
}

func TestActivityListLimit(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		if page != "3" {
			n := map[string]string{"1": "2", "2": "3"}[page]
			w.Header().Set("Link", "<"+srv.URL+"/1/activities?page="+n+">; rel=\"next\"")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, strings.Replace(actFeed, `"id":"V1"`, `"id":"P`+page+`"`, 1))
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	cases := []struct {
		args          []string
		lines, wantRq int
	}{
		{[]string{"activity", "list", "--limit", "3"}, 3, 1},
		{[]string{"activity", "list", "--limit", "5"}, 5, 2},
		{[]string{"activity", "list"}, 12, 3},
		{[]string{"activity", "list", "--limit", "0"}, 12, 3},
	}
	for _, c := range cases {
		mu.Lock()
		requests = 0
		mu.Unlock()
		r := runGo(c.args)
		mu.Lock()
		got := requests
		mu.Unlock()
		if n := strings.Count(r.stdout, "\n"); r.code != 0 || n != c.lines || got != c.wantRq {
			t.Errorf("%q: %d lines, %d requests (%+v), want %d and %d", c.args, n, got, r, c.lines, c.wantRq)
		}
	}
	if r := runGo([]string{"activity", "list", "--limit", "-1"}); r.code != 2 {
		t.Errorf("--limit -1: got %+v", r)
	}
	help := runGo([]string{"activity", "list", "--help"})
	if !strings.Contains(help.stdout, "(default 50)") {
		t.Errorf("help does not show the default:\n%s", help.stdout)
	}
}

// Two users share a name, so --creator by that name refuses with exit 2.
func TestActivityListAmbiguousCreator(t *testing.T) {
	twin := strings.Replace(acctUser2, `"Sam"`, `"Alex"`, 1)
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + twin + "]"})
	r := runGo([]string{"activity", "list", "--creator", "alex"})
	if r.code != 2 || r.stderr != "fz: 2 users are named 'alex'; use the user id or email\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/users" {
		t.Errorf("calls = %q", got)
	}
}
