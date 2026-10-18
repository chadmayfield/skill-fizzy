package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type extraCall struct{ method, path, query, body string }

// extraServer is a small fake Fizzy for the card extras. card is what GET
// /1/cards/87 returns.
func extraServer(t *testing.T, card string) (*httptest.Server, func() []extraCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []extraCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, extraCall{r.Method, r.URL.Path, r.URL.RawQuery, string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reactions") {
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, `{"id":"r2","content":"ok"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/1/users":
			io.WriteString(w, `[{"id":"u1","name":"Gate Keeper","email_address":"keeper@fizzy.example.com"},{"id":"u2","name":"Hinge Smith","email_address":"hinge@fizzy.example.com"}]`)
		case "/1/cards/87":
			io.WriteString(w, card)
		case "/1/my/pins":
			io.WriteString(w, `[{"number":87,"title":"Fix the gate","column":{"id":"c1","name":"Now"},"tags":["buy"]}]`)
		case "/1/cards/87/reactions":
			io.WriteString(w, `[{"id":"r1","content":"nice","reacter":{"id":"u1","name":"Gate Keeper"}}]`)
		case "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
		case "/1/boards/b1/columns":
			io.WriteString(w, `[{"id":"c1","name":"Now"},{"id":"c2","name":"Next"}]`)
		case "/1/tags":
			io.WriteString(w, `[{"id":"t1","title":"buy"},{"id":"t2","title":"waiting"}]`)
		case "/1/cards":
			io.WriteString(w, `[{"number":88,"title":"Oil the hinge"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	return srv, func() []extraCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]extraCall(nil), calls...)
	}
}

func extraWrites(cs []extraCall) []extraCall {
	var out []extraCall
	for _, c := range cs {
		if c.method != http.MethodGet {
			out = append(out, c)
		}
	}
	return out
}

func TestCardExtraCommandsRegistered(t *testing.T) {
	root := NewRoot()
	for _, name := range []string{"assign", "unassign", "watch", "unwatch", "pin", "unpin", "pins", "boost", "boosts", "unboost", "image"} {
		cmd, _, err := root.Find([]string{"card", name})
		if err != nil || cmd.Name() != name || cmd.Parent().Name() != "card" {
			t.Errorf("card %s not found: %v", name, err)
			continue
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Errorf("card %s needs Short, Long and Example", name)
		}
	}
}

func TestCardAssign(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[{"id":"u1"}],"has_more_assignees":false}`)
	r := runGo([]string{"card", "assign", "87", "hinge@fizzy.example.com"})
	if r.code != 0 || r.stdout != "card #87: assigned Hinge Smith (u2, matched by email)\n" {
		t.Fatalf("got %+v", r)
	}
	w := extraWrites(calls())
	if len(w) != 1 || w[0].path != "/1/cards/87/assignments" || w[0].body != `{"assignee_id":"u2"}` {
		t.Errorf("writes %+v", w)
	}
}

func TestCardAssignAlready(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[{"id":"u1"}],"has_more_assignees":false}`)
	r := runGo([]string{"card", "assign", "87", "Gate Keeper"})
	if r.code != 0 || r.stdout != "card #87: Gate Keeper (u1, matched by name) already assigned\n" {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Errorf("writes %+v", w)
	}
	r = runGo([]string{"--json", "card", "unassign", "87", "u2"})
	want := "{\n  \"number\": 87,\n  \"user_id\": \"u2\",\n  \"name\": \"Hinge Smith\",\n  \"matched_by\": \"id\",\n  \"changed\": false\n}\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
}

func TestCardUnassign(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[{"id":"u1"}],"has_more_assignees":false}`)
	r := runGo([]string{"card", "unassign", "87", "u1"})
	if r.code != 0 || r.stdout != "card #87: unassigned Gate Keeper (u1, matched by id)\n" {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].body != `{"assignee_id":"u1"}` {
		t.Errorf("writes %+v", w)
	}
}

func TestCardAssignUnknownUser(t *testing.T) {
	extraServer(t, `{}`)
	r := runGo([]string{"card", "assign", "87", "Nobody"})
	if r.code != 1 || !strings.HasPrefix(r.stderr, "fz: no user 'Nobody'") {
		t.Fatalf("got %+v", r)
	}
}

func TestCardAssignDryRun(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[],"has_more_assignees":false}`)
	r := runGo([]string{"--dry-run", "card", "assign", "87", "u1"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] POST ") || !strings.Contains(r.stderr, `/1/cards/87/assignments {"assignee_id": "u1"}`) {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Errorf("dry run sent %+v", w)
	}
}

func TestCardWatchAndPin(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	for _, c := range []struct {
		args         []string
		out          string
		method, path string
	}{
		{[]string{"card", "watch", "87"}, "card #87 watched\n", "POST", "/1/cards/87/watch"},
		{[]string{"card", "unwatch", "87"}, "card #87 unwatched\n", "DELETE", "/1/cards/87/watch"},
		{[]string{"card", "pin", "87"}, "card #87 pinned\n", "POST", "/1/cards/87/pin"},
		{[]string{"card", "unpin", "87"}, "card #87 unpinned\n", "DELETE", "/1/cards/87/pin"},
		{[]string{"card", "unboost", "87", "r1"}, "boost r1 removed from card #87\n", "DELETE", "/1/cards/87/reactions/r1"},
	} {
		before := len(extraWrites(calls()))
		r := runGo(c.args)
		if r.code != 0 || r.stdout != c.out {
			t.Errorf("%v: got %+v", c.args, r)
			continue
		}
		w := extraWrites(calls())
		if len(w) != before+1 || w[before].method != c.method || w[before].path != c.path {
			t.Errorf("%v: writes %+v", c.args, w[before:])
		}
	}
	r := runGo([]string{"--json", "card", "pin", "87"})
	if r.stdout != "{\n  \"number\": 87,\n  \"pinned\": true\n}\n" {
		t.Errorf("pin json %q", r.stdout)
	}
	r = runGo([]string{"--json", "card", "unwatch", "87"})
	if r.stdout != "{\n  \"number\": 87,\n  \"watching\": false\n}\n" {
		t.Errorf("unwatch json %q", r.stdout)
	}
}

func TestCardPins(t *testing.T) {
	extraServer(t, `{}`)
	r := runGo([]string{"card", "pins"})
	if r.code != 0 || r.stdout != "#87    [Now] Fix the gate #buy\n" {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "card", "pins"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "[\n  {\n    \"number\": 87,") {
		t.Fatalf("got %+v", r)
	}
}

func TestCardBoosts(t *testing.T) {
	extraServer(t, `{}`)
	r := runGo([]string{"card", "boosts", "87"})
	if r.code != 0 || r.stdout != "r1  nice  (Gate Keeper)\n" {
		t.Fatalf("got %+v", r)
	}
}

func TestCardBoost(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	r := runGo([]string{"card", "boost", "87", "Nice hinge"})
	if r.code != 0 || r.stdout != "boost r2 added to card #87\n" {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].path != "/1/cards/87/reactions" || w[0].body != `{"reaction":{"content":"Nice hinge"}}` {
		t.Errorf("writes %+v", w)
	}
	r = runGo([]string{"--json", "card", "boost", "87", "ok"})
	if r.stdout != "{\n  \"id\": \"r2\",\n  \"content\": \"ok\"\n}\n" {
		t.Errorf("json %q", r.stdout)
	}
}

func TestCardBoostTooLong(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	r := runGo([]string{"card", "boost", "87", "seventeen letters"})
	if r.code != 2 || r.stderr != "fz: a boost is at most 16 characters; this one is 17\n" {
		t.Fatalf("got %+v", r)
	}
	if len(calls()) != 0 {
		t.Errorf("sent %+v", calls())
	}
	// 16 emoji are 16 characters to Fizzy, though 64 bytes.
	if r := runGo([]string{"card", "boost", "87", strings.Repeat("👍", 16)}); r.code != 0 {
		t.Errorf("16 emoji: %+v", r)
	}
}

func TestCardBoostDryRun(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	r := runGo([]string{"--dry-run", "card", "boost", "87", "ok"})
	if r.code != 0 || r.stdout != "dry-run: boost not added to card #87\n" || !strings.Contains(r.stderr, "[dry-run] POST ") {
		t.Fatalf("got %+v", r)
	}
	if len(calls()) != 0 {
		t.Errorf("sent %+v", calls())
	}
}

func TestCardImage(t *testing.T) {
	card := `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},"image_url":"https://fizzy.example.com/i.png"}`
	_, calls := extraServer(t, card)

	r := runGo([]string{"card", "image", "87"})
	if r.code != 2 || r.stderr != "fz: card image only removes an image; add --remove\n" {
		t.Errorf("no --remove: %+v", r)
	}
	r = runGo([]string{"card", "image", "87", "--remove"})
	if r.code != 2 || r.stderr != "about to remove the image from: #87    [maybe] Fix the gate (board Home)\nrefusing without --yes\n" {
		t.Errorf("no --yes: %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Fatalf("sent %+v", w)
	}
	r = runGo([]string{"card", "image", "87", "--remove", "--yes"})
	if r.code != 0 || r.stdout != "card #87 image removed\n" {
		t.Errorf("--yes: %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].method != "DELETE" || w[0].path != "/1/cards/87/image" {
		t.Errorf("writes %+v", w)
	}
}

func TestCardImageNone(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"title":"Fix the gate","image_url":null}`)
	r := runGo([]string{"card", "image", "87", "--remove", "--yes"})
	if r.code != 0 || r.stdout != "card #87 has no image\n" {
		t.Errorf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Errorf("sent %+v", w)
	}
}

func cardsQuery(cs []extraCall) (string, bool) {
	for _, c := range cs {
		if c.path == "/1/cards" {
			return c.query, true
		}
	}
	return "", false
}

func TestCardListFilters(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	r := runGo([]string{"card", "list", "--board", "Home", "--state", "golden", "--search", "gate",
		"--column", "next", "--tag", "#Waiting", "--assignee", "Gate Keeper",
		"--sort", "newest", "--created", "thisweek", "--closed", "lastmonth"})
	if r.code != 0 || r.stdout != "#88    [maybe] Oil the hinge\n" {
		t.Fatalf("got %+v", r)
	}
	q, ok := cardsQuery(calls())
	want := "board_ids%5B%5D=b1&indexed_by=golden&terms%5B%5D=gate&column_ids%5B%5D=c2&tag_ids%5B%5D=t2" +
		"&assignee_ids%5B%5D=u1&sorted_by=newest&creation=thisweek&closure=lastmonth"
	if !ok || q != want {
		t.Errorf("query\n got %s\nwant %s", q, want)
	}
}

func TestCardListUnassigned(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	if r := runGo([]string{"card", "list", "--unassigned"}); r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	if q, _ := cardsQuery(calls()); q != "assignment_status=unassigned" {
		t.Errorf("query %s", q)
	}
}

func TestCardListFilterUsage(t *testing.T) {
	_, calls := extraServer(t, `{}`)
	for _, c := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"card", "list", "--column", "Now"}, "fz: --column needs --board\n"},
		{[]string{"card", "list", "--assignee", "u1", "--unassigned"}, "fz: use --assignee or --unassigned, not both\n"},
		{[]string{"card", "list", "--sort", "best"}, "fz: argument --sort: invalid choice: 'best' (choose from 'latest', 'newest', 'oldest')\n"},
		{[]string{"card", "list", "--created", "someday"}, "fz: argument --created: invalid choice: 'someday' (choose from 'today', 'yesterday', 'thisweek', 'lastweek', 'thismonth', 'lastmonth', 'thisyear', 'lastyear')\n"},
		{[]string{"card", "list", "--closed", "never"}, "fz: argument --closed: invalid choice: 'never' (choose from 'today', 'yesterday', 'thisweek', 'lastweek', 'thismonth', 'lastmonth', 'thisyear', 'lastyear')\n"},
	} {
		r := runGo(c.args)
		if r.code != 2 || r.stderr != c.stderr {
			t.Errorf("%v: got %+v", c.args, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("usage errors sent %+v", calls())
	}
}

func TestCardListUnknownTag(t *testing.T) {
	extraServer(t, `{}`)
	r := runGo([]string{"card", "list", "--tag", "nope"})
	if r.code != 1 || r.stderr != "fz: no tag 'nope' (have: buy, waiting)\n" {
		t.Fatalf("got %+v", r)
	}
}

// A name that differs only in case still matches when one user has it.
func TestCardAssignNameIgnoresCase(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[],"has_more_assignees":false}`)
	r := runGo([]string{"card", "assign", "87", "hinge smith"})
	if r.code != 0 || r.stdout != "card #87: assigned Hinge Smith (u2, matched by name)\n" {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].body != `{"assignee_id":"u2"}` {
		t.Errorf("writes %+v", w)
	}
}

// --assignee does not pass an unknown id through; only activity --creator does.
func TestCardListAssigneeUnknownID(t *testing.T) {
	extraServer(t, `{}`)
	r := runGo([]string{"card", "list", "--assignee", "03abc123def456ghi789jkl0mn"})
	if r.code != 1 || !strings.HasPrefix(r.stderr, "fz: no user '03abc123def456ghi789jkl0mn'") {
		t.Fatalf("got %+v", r)
	}
}
