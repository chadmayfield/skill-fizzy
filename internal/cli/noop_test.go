package cli

import (
	"strings"
	"testing"
)

const noChangeText = "dry-run: nothing sent; no change needed\n"

// checkNoChange runs args with --dry-run, as text and as JSON. The text is
// the dry-run no-change line, the JSON carries "dry_run": true, and nothing
// is written.
func checkNoChange(t *testing.T, args []string, writes func() int) {
	t.Helper()
	r := runGo(append([]string{"--dry-run"}, args...))
	if r.code != 0 || r.stdout != noChangeText {
		t.Errorf("%q text: %+v", args, r)
	}
	r = runGo(append([]string{"--dry-run", "--json"}, args...))
	if r.code != 0 || !strings.HasSuffix(r.stdout, "\"dry_run\": true\n}\n") || !strings.Contains(r.stdout, "\"changed\": false") && !strings.Contains(r.stdout, "\"removed\": false") {
		t.Errorf("%q json: %+v", args, r)
	}
	if n := writes(); n != 0 {
		t.Errorf("%q sent %d writes", args, n)
	}
}

func TestNoChangeUnderDryRunBoardAccessAll(t *testing.T) {
	srv, calls := bxServer(t, true)
	editEnv(t, srv)
	checkNoChange(t, []string{"board", "access", "Home", "--all", "--yes"}, func() int { return len(writes(calls())) })
}

func TestNoChangeUnderDryRunAssign(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[{"id":"u1"}],"has_more_assignees":false}`)
	n := func() int { return len(extraWrites(calls())) }
	checkNoChange(t, []string{"card", "assign", "87", "u1"}, n)
	checkNoChange(t, []string{"card", "unassign", "87", "u2"}, n)
}

func TestNoChangeUnderDryRunImage(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"title":"Fix the gate","image_url":null}`)
	checkNoChange(t, []string{"card", "image", "87", "--remove", "--yes"}, func() int { return len(extraWrites(calls())) })
}

func TestNoChangeUnderDryRunWebhookActivate(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	checkNoChange(t, []string{"webhook", "activate", "B1", "H1"}, func() int { return len(writes(calls())) })
}

// checkNoChangeLive runs args without --dry-run, with and without --yes. A
// change that is already in place needs no --yes: it prints want, exits 0
// and sends nothing.
func checkNoChangeLive(t *testing.T, args []string, want string, writes func() int) {
	t.Helper()
	for _, argv := range [][]string{args, append(append([]string{}, args...), "--yes")} {
		r := runGo(argv)
		if r.code != 0 || r.stdout != want+"\n" || r.stderr != "" {
			t.Errorf("%q: %+v", argv, r)
		}
	}
	if n := writes(); n != 0 {
		t.Errorf("%q sent %d writes", args, n)
	}
}

func TestNoChangeUserEditSameName(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/users": "[" + acctUser + "," + acctUser2 + "]", "GET /1/users/U1": acctUser, "PUT /1/users/U1": acctUser})
	n := func() int { return len(writes(calls())) }
	args := []string{"user", "edit", "U1", "--name", "Alex"}
	checkNoChangeLive(t, args, "user U1 is already named Alex; no change needed", n)
	checkNoChange(t, append(args, "--yes"), n)
}

func TestNoChangeAccountPostponeSameDays(t *testing.T) {
	_, calls := acctServer(t, map[string]string{"GET /1/account/settings": acctSettings, "PUT /1/account/entropy": acctSettings})
	n := func() int { return len(writes(calls())) }
	args := []string{"account", "postpone", "30"}
	checkNoChangeLive(t, args, "the auto-postpone period of Gate Works is already 30 days; no change needed", n)
	checkNoChange(t, append(args, "--yes"), n)
}

func TestNoChangeBoardAccessSameUsers(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	n := func() int { return len(writes(calls())) }
	// The board's list is u1 and u2. Order and repeats do not matter.
	args := []string{"board", "access", "Home", "--users", "Sam,u1,Pat"}
	checkNoChangeLive(t, args, "board Home already gives access to exactly these users; no change needed", n)
	checkNoChange(t, append(args, "--yes"), n)
	// A different set is still a change and still needs --yes.
	r := runGo([]string{"board", "access", "Home", "--users", "u1"})
	if r.code != 2 {
		t.Errorf("a real change: %+v", r)
	}
}
