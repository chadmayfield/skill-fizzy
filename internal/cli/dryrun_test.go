package cli

import (
	"strings"
	"testing"
)

// dryRunRoutes answers every GET the plan 2 write commands make before they
// write.
var dryRunRoutes = map[string]string{
	"GET /1/boards":                     `[{"id":"b1","name":"Home"}]`,
	"GET /1/boards/b1":                  `{"id":"b1","name":"Home","all_access":false}`,
	"GET /1/boards/b1/columns":          `[{"id":"c1","name":"Next","color":{"name":"Lime","value":"var(--color-card-4)"}}]`,
	"GET /1/boards/b1/columns/c1/cards": `[]`,
	"GET /1/cards":                      `[]`,
	"GET /1/cards/87":                   `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},"assignees":[{"id":"U1"}],"has_more_assignees":false,"image_url":"https://fizzy.example.com/i.png"}`,
	"GET /1/cards/87/comments/c1":       cmtA,
	"GET /1/users":                      "[" + acctUser + "," + acctUser2 + "]",
	"GET /1/users/U1":                   acctUser,
	"GET /1/account/settings":           acctSettings,
	"GET /my/identity":                  acctIdentity,
	"GET /1/boards/b1/webhooks/H1":      strings.Replace(testWebhook, `"active":true`, `"active":false`, 1),
}

// Every write command the parity goldens do not pin says in dry-run mode
// that nothing was sent, and sends nothing.
func TestDryRunTextSaysNothingSent(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"card", "watch", "87"}, "would watch card #87"},
		{[]string{"card", "unwatch", "87"}, "would unwatch card #87"},
		{[]string{"card", "pin", "87"}, "would pin card #87"},
		{[]string{"card", "unpin", "87"}, "would unpin card #87"},
		{[]string{"card", "assign", "87", "sam@example.com"}, "would assign Sam (U2, matched by email) to card #87"},
		{[]string{"card", "unassign", "87", "U1"}, "would unassign Alex (U1, matched by id) from card #87"},
		{[]string{"card", "boost", "87", "Nice hinge"}, "would add a boost to card #87"},
		{[]string{"card", "unboost", "87", "r1"}, "would remove boost r1 from card #87"},
		{[]string{"card", "image", "87", "--remove", "--yes"}, "would remove the image from card #87"},
		{[]string{"card", "edit", "87", "--title", "Fix the gate hinge"}, "would update card #87: title"},
		{[]string{"comment", "edit", "87", "c1", "Gate latches."}, "would update comment c1"},
		{[]string{"comment", "delete", "87", "c1", "--yes"}, "would delete comment c1"},
		{[]string{"comment", "react", "87", "c1", "ok"}, "would add a reaction to comment c1"},
		{[]string{"comment", "unreact", "87", "c1", "r1"}, "would remove reaction r1"},
		{[]string{"board", "edit", "Home", "--name", "Garden"}, "would update board Home: name"},
		{[]string{"board", "delete", "Home", "--yes"}, "would delete board Home"},
		{[]string{"board", "publish", "Home", "--yes"}, "would publish board Home"},
		{[]string{"board", "unpublish", "Home"}, "would unpublish board Home"},
		{[]string{"board", "access", "Home", "--all", "--yes"}, "would set board Home access to all users"},
		{[]string{"board", "access", "Home", "--users", "U1,U2", "--yes"}, "would set board Home access to 2 users"},
		{[]string{"column", "create", "Home", "Later"}, "would create column 'Later'"},
		{[]string{"column", "edit", "Home", "Next", "--name", "Later"}, "would update column Next: name"},
		{[]string{"column", "delete", "Home", "Next", "--yes"}, "would delete column Next"},
		{[]string{"notification", "read", "N1"}, "would mark notification N1 read"},
		{[]string{"notification", "read", "--all"}, "would mark all notifications read"},
		{[]string{"notification", "unread", "N1"}, "would mark notification N1 unread"},
		{[]string{"notification", "settings", "--email", "daily"}, "would set email to daily"},
		{[]string{"user", "edit", "U1", "--name", "Gate Keeper", "--yes"}, "would rename user U1 to Gate Keeper"},
		{[]string{"user", "timezone", "America/Denver"}, "would set timezone to America/Denver"},
		{[]string{"account", "postpone", "90", "--yes"}, "would set account auto-postpone to 90 days"},
		{[]string{"export", "account"}, "would start an account export"},
		{[]string{"export", "mine"}, "would start your export"},
		{[]string{"webhook", "create", "Home", "--name", "Gate alerts", "--url", "https://hooks.example.com/gate", "--actions", "card_closed", "--yes"}, "would create webhook 'Gate alerts'"},
		{[]string{"webhook", "edit", "Home", "H1", "--name", "Hinge alerts", "--yes"}, "would update webhook H1: name"},
		{[]string{"webhook", "delete", "Home", "H1", "--yes"}, "would delete webhook H1"},
		{[]string{"webhook", "activate", "Home", "H1"}, "would activate webhook H1"},
	} {
		name := strings.Join(c.args, " ")
		_, calls := acctServer(t, dryRunRoutes)
		r := runGo(append([]string{"--dry-run"}, c.args...))
		want := "dry-run: nothing sent; " + c.want + "\n"
		if r.code != 0 || r.stdout != want {
			t.Errorf("%s: got %+v, want stdout %q", name, r, want)
		}
		if !strings.Contains(r.stderr, "[dry-run] ") {
			t.Errorf("%s: no [dry-run] line in %q", name, r.stderr)
		}
		for _, call := range calls() {
			if call.method != "GET" {
				t.Errorf("%s: sent %s %s", name, call.method, call.path)
			}
		}
		r = runGo(append([]string{"--dry-run", "--json"}, c.args...))
		if r.code != 0 || !strings.HasPrefix(r.stdout, "{\n") || !strings.Contains(r.stdout, "\n  \"dry_run\": true\n}\n") {
			t.Errorf("--json %s: got %+v, want a dry_run key", name, r)
		}
	}
}

// Without --dry-run, --json has no dry_run key.
func TestJSONWithoutDryRunHasNoKey(t *testing.T) {
	acctServer(t, map[string]string{"POST /1/cards/87/watch": ""})
	r := runGo([]string{"--json", "card", "watch", "87"})
	if r.code != 0 || r.stdout != "{\n  \"number\": 87,\n  \"watching\": true\n}\n" {
		t.Fatalf("got %+v", r)
	}
}

func TestDryRunLine(t *testing.T) {
	for in, want := range map[string]string{
		"moved to board Garden":                   "dry-run: nothing sent; would move to board Garden",
		"moved to Next":                           "dry-run: nothing sent; would move to Next",
		"title and description sent":              "dry-run: nothing sent; would send the title and description",
		"step added: Oil the hinge":               "dry-run: nothing sent; would add step: Oil the hinge",
		"step ticked: 03abc123def456ghi789jkl0mn": "dry-run: nothing sent; would tick step: 03abc123def456ghi789jkl0mn",
		"step deleted: Old step":                  "dry-run: nothing sent; would delete step: Old step",
		"tag added: hinge":                        "dry-run: nothing sent; would add tag: hinge",
		"tag removed: buy":                        "dry-run: nothing sent; would remove tag: buy",
		"golden on":                               "dry-run: nothing sent; would turn golden on",
		"golden off":                              "dry-run: nothing sent; would turn golden off",
		"step kept, not in draft: Gate":           "step kept, not in draft: Gate",
		"tag kept, not in draft: buy":             "tag kept, not in draft: buy",
		"card is closed; column left alone":       "card is closed; column left alone",
	} {
		if got := dryRunLine(in); got != want {
			t.Errorf("dryRunLine(%q) = %q", in, got)
		}
	}
}

// Every list command added in plan 2 prints a placeholder when the list is
// empty, as card list prints "(no cards)".
func TestEmptyListsSayNone(t *testing.T) {
	routes := map[string]string{
		"GET /1/boards":                           `[{"id":"b1","name":"Home"}]`,
		"GET /1/boards/b1/columns":                `[{"id":"c1","name":"Next"}]`,
		"GET /1/boards/b1/columns/c1/cards":       `[]`,
		"GET /1/cards/87/comments":                `[]`,
		"GET /1/cards/87/comments/c1/reactions":   `[]`,
		"GET /1/boards/b1/webhooks":               `[]`,
		"GET /1/boards/b1/webhooks/H1/deliveries": `[]`,
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"comment", "list", "87"}, "(no comments)"},
		{[]string{"comment", "reactions", "87", "c1"}, "(no reactions)"},
		{[]string{"column", "cards", "Home", "Next"}, "(no cards)"},
		{[]string{"webhook", "list", "Home"}, "(no webhooks)"},
		{[]string{"webhook", "deliveries", "Home", "H1"}, "(no deliveries)"},
	} {
		acctServer(t, routes)
		r := runGo(c.args)
		if r.code != 0 || r.stdout != c.want+"\n" {
			t.Errorf("%s: got %+v", strings.Join(c.args, " "), r)
		}
		r = runGo(append([]string{"--json"}, c.args...))
		if r.code != 0 || r.stdout != "[]\n" {
			t.Errorf("--json %s: got %+v", strings.Join(c.args, " "), r)
		}
	}
}
