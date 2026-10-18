package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestPeopleCommandsExist(t *testing.T) {
	hasCommands(t, "fz tag list", "fz user list", "fz user show", "fz user edit", "fz user timezone", "fz whoami")
}

// The tag group must not change the fz 0.1 rewrite: "fz tag 87 buy" is
// still card tag, and "fz tag list" reaches the new command.
func TestTagGroupKeepsLegacyRewrite(t *testing.T) {
	if got := Rewrite([]string{"tag", "87", "buy"}); !reflect.DeepEqual(got, []string{"card", "tag", "87", "buy"}) {
		t.Errorf("Rewrite(tag 87 buy) = %q", got)
	}
	if got := Rewrite([]string{"tag", "list"}); !reflect.DeepEqual(got, []string{"tag", "list"}) {
		t.Errorf("Rewrite(tag list) = %q", got)
	}
	for argv, want := range map[string]string{"tag 87 buy": "fz card tag", "tag list": "fz tag list", "--json tag list": "fz tag list"} {
		cmd, _, err := NewRoot().Find(Rewrite(strings.Fields(argv)))
		if err != nil || cmd.CommandPath() != want {
			t.Errorf("%q reached %v (%v), want %q", argv, cmd.CommandPath(), err, want)
		}
	}
}

func TestTagList(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/tags": `[{"id":"T1","title":"gate","created_at":"x","url":"u"},{"id":"T2","title":"hinge","created_at":"x","url":"u"}]`})
	r := runGo([]string{"tag", "list"})
	if r.code != 0 || r.stdout != "T1  #gate\nT2  #hinge\n" {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "tag", "list"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "[\n  {\n    \"id\": \"T1\",") {
		t.Fatalf("json %+v", r)
	}
	if got := callList(calls()); got != "GET /1/tags\nGET /1/tags" {
		t.Errorf("calls = %q", got)
	}
}

func TestTagListEmpty(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/tags": `[]`})
	if r := runGo([]string{"tag", "list"}); r.code != 0 || r.stdout != "(no tags)\n" {
		t.Fatalf("got %+v", r)
	}
}

func TestLegacyTagStillTagsACard(t *testing.T) {
	card := `{"number":87,"title":"Oil the hinge","tags":[]}`
	_, calls := acctServer(t, map[string]string{"GET /1/cards/87": card, "POST /1/cards/87/taggings": ""})
	r := runGo([]string{"tag", "87", "buy"})
	if r.code != 0 || r.stdout != "tag buy added\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/cards/87\nPOST /1/cards/87/taggings {\"tag_title\":\"buy\"}" {
		t.Errorf("calls = %q", got)
	}
}

func TestUserListAndShow(t *testing.T) {
	acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + acctUser2 + "]", "GET /1/users/U1": acctUser})
	r := runGo([]string{"user", "list"})
	if r.code != 0 || r.stdout != "U1  Alex <alex@example.com> owner\nU2  Sam <sam@example.com> member (inactive)\n" {
		t.Fatalf("list %+v", r)
	}
	r = runGo([]string{"user", "show", "U1"})
	if r.code != 0 || r.stdout != "U1  Alex <alex@example.com> owner\nurl: https://fizzy.example.com/1/users/U1\n" {
		t.Fatalf("show %+v", r)
	}
	r = runGo([]string{"--json", "user", "show", "U1"})
	if r.code != 0 || !strings.Contains(r.stdout, `"avatar_url": "a"`) {
		t.Fatalf("json %+v", r)
	}
}

func TestWhoami(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /my/identity": acctIdentity})
	r := runGo([]string{"whoami"})
	want := "  2  Other  Sam <sam@example.com> member\n* 1  Gate Works  Alex <alex@example.com> owner\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /my/identity" {
		t.Errorf("calls = %q", got)
	}
	r = runGo([]string{"--json", "whoami"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "{\n  \"id\": \"I1\",\n  \"accounts\": [") {
		t.Fatalf("json %+v", r)
	}
}

func TestUserEditNeedsYes(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + acctUser2 + "]", "GET /1/users/U1": acctUser, "PUT /1/users/U1": acctUser})
	r := runGo([]string{"user", "edit", "U1", "--name", "Gate Keeper"})
	if r.code != 2 || r.stdout != "" || r.stderr != "about to rename user U1 from Alex to Gate Keeper\nrefusing without --yes\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/users\nGET /1/users/U1" {
		t.Errorf("calls = %q", got)
	}
}

func TestUserEditYes(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + acctUser2 + "]", "GET /1/users/U1": acctUser, "PUT /1/users/U1": acctUser})
	r := runGo([]string{"user", "edit", "U1", "--name", "Gate Keeper", "--yes"})
	if r.code != 0 || r.stdout != "user U1 renamed to Gate Keeper\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != "GET /1/users\nGET /1/users/U1\nPUT /1/users/U1 {\"user\":{\"name\":\"Gate Keeper\"}}" {
		t.Errorf("calls = %q", got)
	}
}

func TestUserEditDryRun(t *testing.T) {
	srv, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + acctUser2 + "]", "GET /1/users/U1": acctUser})
	r := runGo([]string{"--dry-run", "--json", "user", "edit", "U1", "--name", "Gate Keeper", "--yes"})
	if r.code != 0 || r.stdout != "{\n  \"id\": \"U1\",\n  \"name\": \"Gate Keeper\",\n  \"dry_run\": true\n}\n" {
		t.Fatalf("got %+v", r)
	}
	if !strings.Contains(r.stderr, "[dry-run] PUT "+srv.URL+`/1/users/U1 {"user": {"name": "Gate Keeper"}}`) {
		t.Errorf("stderr = %q", r.stderr)
	}
	if got := callList(calls()); got != "GET /1/users\nGET /1/users/U1" {
		t.Errorf("calls = %q", got)
	}
}

func TestUserEditNeedsName(t *testing.T) {
	_, calls := acctServer(t, map[string]string{})
	for _, argv := range [][]string{{"user", "edit", "U1", "--yes"}, {"user", "edit", "U1", "--name", " ", "--yes"}} {
		if r := runGo(argv); r.code != 2 || !strings.HasPrefix(r.stderr, "fz: ") {
			t.Errorf("%q: got %+v", argv, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestUserTimezone(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"PATCH /1/my/timezone": ""})
	r := runGo([]string{"user", "timezone", "America/Denver"})
	if r.code != 0 || r.stdout != "timezone set to America/Denver\n" {
		t.Fatalf("got %+v", r)
	}
	if got := callList(calls()); got != `PATCH /1/my/timezone {"timezone_name":"America/Denver"}` {
		t.Errorf("calls = %q", got)
	}
}

func TestUserTimezoneRejectsBadNames(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"PATCH /1/my/timezone": ""})
	for _, name := range []string{"Mars/Base", "", "Local", "../etc", "posixrules", "Factory"} {
		r := runGo([]string{"user", "timezone", name})
		if r.code != 2 || !strings.HasPrefix(r.stderr, "fz: ") {
			t.Errorf("%q: got %+v", name, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestUserTimezoneAcceptsExactNames(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"PATCH /1/my/timezone": ""})
	for _, name := range []string{"America/Denver", "Europe/London", "America/Argentina/Buenos_Aires", "UTC"} {
		if r := runGo([]string{"user", "timezone", name}); r.code != 0 {
			t.Errorf("%q: got %+v", name, r)
		}
	}
	if len(calls()) != 4 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

func TestUserTimezoneServerRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(422)
		io.WriteString(w, `{"error":"invalid"}`)
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	r := runGo([]string{"user", "timezone", "America/Denver"})
	if r.code != 1 || r.stderr != "fz: timezone America/Denver was rejected by the server; names are case-sensitive, for example America/Denver\n" {
		t.Fatalf("got %+v", r)
	}
}

// user show and user edit take a USER the way card assign does: an id, a
// name, a name in any case when only one user has it, or an email.
func TestUserShowAndEditMatchUser(t *testing.T) {
	_, calls := acctServer(t, map[string]string{
		"GET /1/users":    "[" + acctUser + "," + acctUser2 + "]",
		"GET /1/users/U1": acctUser,
		"GET /1/users/U2": acctUser2,
		"PUT /1/users/U2": acctUser2,
	})
	for _, who := range []string{"U1", "Alex", "alex", "ALEX@example.com"} {
		r := runGo([]string{"user", "show", who})
		if r.code != 0 || !strings.HasPrefix(r.stdout, "U1  Alex <alex@example.com> owner\n") {
			t.Errorf("show %s: %+v", who, r)
		}
	}
	r := runGo([]string{"user", "edit", "sam@example.com", "--name", "Gate Keeper", "--yes"})
	if r.code != 0 || r.stdout != "user U2 renamed to Gate Keeper\n" || r.stderr != "about to rename user U2 from Sam to Gate Keeper\n" {
		t.Fatalf("edit %+v", r)
	}
	var puts []string
	for _, c := range calls() {
		if c.method != "GET" {
			puts = append(puts, c.method+" "+c.path)
		}
	}
	if len(puts) != 1 || puts[0] != "PUT /1/users/U2" {
		t.Errorf("writes = %q", puts)
	}
}

// A USER that matches nobody exits 1, and one that two users share exits 2.
// Neither sends a write.
func TestUserShowAndEditUnknownOrAmbiguous(t *testing.T) {
	twin := strings.Replace(strings.Replace(acctUser2, `"U2"`, `"U3"`, 1), `"Sam"`, `"alex"`, 1)
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + twin + "]"})
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"user", "show", "Nobody"}, 1},
		{[]string{"user", "edit", "Nobody", "--name", "Gate Keeper", "--yes"}, 1},
		{[]string{"user", "show", "ALEX"}, 2},
		{[]string{"user", "edit", "ALEX", "--name", "Gate Keeper", "--yes"}, 2},
	} {
		if r := runGo(c.args); r.code != c.code || r.stdout != "" {
			t.Errorf("%q: %+v", c.args, r)
		}
	}
	for _, c := range calls() {
		if c.method != "GET" || c.path != "/1/users" {
			t.Errorf("sent %s %s", c.method, c.path)
		}
	}
}

// whoami leaves out the angle brackets when a user has no email, as
// userLine does.
func TestWhoamiWithoutEmail(t *testing.T) {
	acctServer(t, map[string]string{"GET /my/identity": strings.Replace(acctIdentity, `"email_address":"alex@example.com"`, `"email_address":""`, 1)})
	r := runGo([]string{"whoami"})
	want := "  2  Other  Sam <sam@example.com> member\n* 1  Gate Works  Alex owner\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
}
