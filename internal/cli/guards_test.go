package cli

import (
	"strings"
	"testing"
)

// The PATCH reply carries signing_secret. webhook edit prints the reply
// with --json, so it must drop the secret, as list and show do.
func TestWebhookEditJSONHidesSecret(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"--json", "webhook", "edit", "B1", "H1", "--name", "Hinge alerts", "--yes"})
	if r.code != 0 || !strings.Contains(r.stdout, `"id": "H1"`) {
		t.Fatalf("got %+v", r)
	}
	if strings.Contains(r.stdout, "signing_secret") || strings.Contains(r.stdout, "s3cret") ||
		strings.Contains(r.stderr, "s3cret") {
		t.Errorf("webhook edit printed the signing secret: %+v", r)
	}
}

// With two assignees, assign and unassign change only the target user, and
// the target being first or last in the list makes no difference.
func TestAssignWithTwoAssigneesTogglesOnlyTheTarget(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"assignees":[{"id":"u1"},{"id":"u2"}],"has_more_assignees":false}`)
	for _, who := range []string{"u1", "u2"} {
		r := runGo([]string{"card", "assign", "87", who})
		if r.code != 0 || !strings.HasSuffix(r.stdout, "already assigned\n") {
			t.Errorf("assign %s: %+v", who, r)
		}
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Fatalf("assigning assigned users sent %+v", w)
	}
	for i, who := range []string{"u1", "u2"} {
		r := runGo([]string{"card", "unassign", "87", who})
		w := extraWrites(calls())
		if r.code != 0 || len(w) != i+1 || w[i].body != `{"assignee_id":"`+who+`"}` {
			t.Errorf("unassign %s: %+v writes %+v", who, r, w)
		}
	}
}
