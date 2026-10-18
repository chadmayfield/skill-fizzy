package cli

import (
	"strings"
	"testing"
)

// emptyTextRoutes serves board Home (B1), column Next (c1), webhook H1 and
// user U1, so every command below gets as far as its write.
func emptyTextRoutes() map[string]string {
	return map[string]string{
		"GET /1/boards":             `[{"id":"B1","name":"Home"}]`,
		"GET /1/boards/B1":          `{"id":"B1","name":"Home","all_access":true}`,
		"GET /1/boards/B1/columns":  `[{"id":"c1","name":"Next","color":{"name":"Lime","value":"var(--color-card-4)"}}]`,
		"GET /1/boards/B1/webhooks": "[]",
		"GET /1/boards/B1/webhooks/H1": `{"id":"H1","name":"Hinge alerts","payload_url":"https://fizzy.example.com/hook","active":true,` +
			`"subscribed_actions":["card_closed"]}`,
		"GET /1/users":    "[" + acctUser + "]",
		"GET /1/users/U1": acctUser,
		"GET /1/cards/87": `{"number":87,"title":"Fix the gate","tags":[]}`,
	}
}

// Empty or blank text exits 2 with "<what> cannot be empty" and sends
// nothing, with or without --dry-run.
func TestEmptyTextRefuses(t *testing.T) {
	_, calls := acctServer(t, emptyTextRoutes())
	for _, blank := range []string{"", "   "} {
		for _, c := range []struct {
			args []string
			what string
		}{
			{[]string{"board", "create", blank}, "the board name"},
			{[]string{"card", "create", "Home", blank}, "the card title"},
			{[]string{"step", "add", "87", blank}, "the step text"},
			{[]string{"comment", "add", "87", blank}, "the comment text"},
			{[]string{"comment", "edit", "87", "c1", blank}, "the comment text"},
			{[]string{"card", "boost", "87", blank}, "a boost"},
			{[]string{"comment", "react", "87", "c1", blank}, "a reaction"},
			{[]string{"user", "edit", "U1", "--name", blank, "--yes"}, "--name"},
			{[]string{"webhook", "create", "Home", "--name", blank, "--url", "https://fizzy.example.com/hook", "--actions", "card_closed", "--yes"}, "--name"},
			{[]string{"webhook", "edit", "Home", "H1", "--name", blank, "--yes"}, "--name"},
			{[]string{"board", "edit", "Home", "--name", blank}, "--name"},
			{[]string{"column", "create", "Home", blank}, "the column name"},
			{[]string{"column", "edit", "Home", "Next", "--name", blank}, "--name"},
		} {
			for _, dry := range []bool{false, true} {
				a := c.args
				if dry {
					a = append([]string{"--dry-run"}, c.args...)
				}
				r := runGo(a)
				want := "fz: " + c.what + " cannot be empty\n"
				if r.code != 2 || r.stdout != "" || r.stderr != want {
					t.Errorf("%q: got %+v, want stderr %q", a, r, want)
				}
			}
		}
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("sent %s %s %s", c.method, c.path, c.body)
		}
	}
}

// Every name flag, and the board and column name arguments, trim spaces the
// same way user edit does.
func TestNamesAreTrimmed(t *testing.T) {
	routes := emptyTextRoutes()
	routes["PUT /1/boards/B1"] = ""
	routes["POST /1/boards/B1/columns"] = ""
	routes["PUT /1/boards/B1/columns/c1"] = ""
	routes["POST /1/boards/B1/webhooks"] = `{"id":"H2","name":"Gate","payload_url":"https://fizzy.example.com/hook","active":true,"subscribed_actions":["card_closed"]}`
	routes["PATCH /1/boards/B1/webhooks/H1"] = `{"id":"H1","name":"Gate","payload_url":"https://fizzy.example.com/hook","active":true,"subscribed_actions":["card_closed"]}`
	routes["PUT /1/users/U1"] = acctUser
	_, calls := acctServer(t, routes)
	for _, args := range [][]string{
		{"board", "edit", "Home", "--name", "  Gate  "},
		{"column", "create", "Home", "  Gate  "},
		{"column", "edit", "Home", "Next", "--name", "  Gate  "},
		{"webhook", "create", "Home", "--name", "  Gate  ", "--url", "https://fizzy.example.com/hook", "--actions", "card_closed", "--yes"},
		{"webhook", "edit", "Home", "H1", "--name", "  Gate  ", "--yes"},
		{"user", "edit", "U1", "--name", "  Gate  ", "--yes"},
	} {
		if r := runGo(args); r.code != 0 {
			t.Errorf("%q: %+v", args, r)
		}
	}
	var bodies []string
	for _, c := range calls() {
		if c.method != "GET" {
			bodies = append(bodies, c.body)
		}
	}
	if len(bodies) != 6 {
		t.Fatalf("writes = %q", bodies)
	}
	for _, b := range bodies {
		if !strings.Contains(b, `"name":"Gate"`) {
			t.Errorf("body %s does not carry the trimmed name", b)
		}
	}
}

// An empty tag is refused before the card is created or read.
func TestEmptyTagRefuses(t *testing.T) {
	_, calls := acctServer(t, emptyTextRoutes())
	for argv, want := range map[string]string{
		"card create Home Gate --tag #": "fz: --tag cannot be empty\n",
		"card tag 87 #":                 "fz: the tag cannot be empty\n",
	} {
		r := runGo(strings.Fields(argv))
		if r.code != 2 || r.stdout != "" || r.stderr != want {
			t.Errorf("%s: got %+v", argv, r)
		}
	}
	for _, c := range calls() {
		if c.method != "GET" || c.path == "/1/cards/87" {
			t.Errorf("sent %s %s %s", c.method, c.path, c.body)
		}
	}
}
