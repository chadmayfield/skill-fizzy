package cli

import (
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

func TestUserLineWithoutEmail(t *testing.T) {
	u := fizzy.User{ID: "U1", Name: "Alex", Role: "member", Active: true}
	if got := userLine(u); got != "U1  Alex member" {
		t.Errorf("no email: %q", got)
	}
	u.EmailAddress = "alex@example.com"
	if got := userLine(u); got != "U1  Alex <alex@example.com> member" {
		t.Errorf("with email: %q", got)
	}
}

func TestWebhookEditEmptyName(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "edit", "B1", "H1", "--name", "", "--yes"})
	if r.code != 2 || r.stderr != "fz: --name cannot be empty\n" {
		t.Fatalf("got %+v", r)
	}
	if len(calls()) != 0 {
		t.Errorf("calls %+v", calls())
	}
}

// A board whose reply has no user_ids does not say who has access, so
// board show does not print "0 users".
func TestBoardShowWithoutUserIDs(t *testing.T) {
	acctServer(t, map[string]string{
		"GET /1/boards":    `[{"id":"b1","name":"Home"}]`,
		"GET /1/boards/b1": `{"id":"b1","name":"Home","all_access":false,"auto_postpone_period_in_days":30}`,
	})
	r := runGo([]string{"board", "show", "Home"})
	if r.code != 0 || !strings.Contains(r.stdout, "access: selected users\n") {
		t.Fatalf("got %+v", r)
	}
}

func TestCommentHelpMakesNoUncitedClaims(t *testing.T) {
	root := NewRoot()
	for _, path := range [][]string{{"comment", "delete"}, {"comment", "unreact"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(cmd.Long, "Only its author can") || strings.Contains(cmd.Long, "Only the person who reacted can") {
			t.Errorf("%s: %q", strings.Join(path, " "), cmd.Long)
		}
	}
}
