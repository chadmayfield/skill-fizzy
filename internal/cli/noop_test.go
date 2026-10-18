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
