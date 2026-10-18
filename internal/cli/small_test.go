package cli

import (
	"strings"
	"testing"
)

// A board with no auto-postpone period prints "postpone off", not "Noned".
func TestBoardShowPostponeOff(t *testing.T) {
	acctServer(t, map[string]string{
		"GET /1/boards":    `[{"id":"B1","name":"Home"}]`,
		"GET /1/boards/B1": `{"id":"B1","name":"Home","all_access":true,"auto_postpone_period_in_days":null}`,
	})
	r := runGo([]string{"board", "show", "Home"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "B1  Home  (postpone off)\n") {
		t.Fatalf("got %+v", r)
	}
}

func TestNotificationReadArgsMessage(t *testing.T) {
	_, calls := acctServer(t, map[string]string{})
	for argv, want := range map[string]string{
		"notification read":          "fz: give an ID or --all\n",
		"notification read N1 --all": "fz: give an ID or --all, not both\n",
	} {
		if r := runGo(strings.Fields(argv)); r.code != 2 || r.stderr != want {
			t.Errorf("%s: %+v", argv, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("calls = %q", callList(calls()))
	}
}

// The create preview names the resolved board, and the delete preview
// names the webhook.
func TestWebhookPreviewsNameBoardAndWebhook(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "create", "B1", "--name", "Hinge alerts", "--url", "https://hooks.example.com/gate", "--actions", "card_closed"})
	if r.code != 2 || !strings.HasPrefix(r.stderr, "about to create webhook 'Hinge alerts': card data on board Gate (B1) goes to ") {
		t.Errorf("create: %+v", r)
	}
	r = runGo([]string{"webhook", "delete", "Gate", "H1"})
	if r.code != 2 || !strings.HasPrefix(r.stderr, "about to delete webhook H1 'Gate alerts': https://hooks.example.com/gate sending card_closed,comment_created\n") {
		t.Errorf("delete: %+v", r)
	}
}

// A repeated action is sent once, and the preview lists it once.
func TestWebhookActionsDeduplicated(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "create", "Gate", "--name", "Hinge alerts", "--url", "https://hooks.example.com/gate",
		"--actions", "card_closed,comment_created,card_closed", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, " for card_closed,comment_created\n") {
		t.Fatalf("create: %+v", r)
	}
	r = runGo([]string{"webhook", "edit", "Gate", "H1", "--actions", "card_closed, card_closed", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, ": actions card_closed\n") {
		t.Fatalf("edit: %+v", r)
	}
	w := writes(calls())
	if len(w) != 2 || !strings.Contains(w[0].body, `"subscribed_actions":["card_closed","comment_created"]}`) ||
		!strings.Contains(w[1].body, `"subscribed_actions":["card_closed"]}`) {
		t.Errorf("writes %+v", w)
	}
}

// An export reply with an empty body has no id. fz says so, and its JSON
// holds no null.
func TestExportStartedWithoutBody(t *testing.T) {
	acctServer(t, map[string]string{"POST /1/account/exports": ""})
	r := runGo([]string{"export", "account"})
	if r.code != 0 || r.stdout != "export started; the server sent no id\n" {
		t.Errorf("text: %+v", r)
	}
	r = runGo([]string{"--json", "export", "account"})
	if r.code != 0 || r.stdout != "{\n  \"started\": true\n}\n" {
		t.Errorf("json: %+v", r)
	}
}
