package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// fz upload warns on stderr when it falls back to signed_id, and still
// prints the id on stdout.
func TestUploadSignedIDFallbackWarnsOnStderr(t *testing.T) {
	acctServer(t, map[string]string{
		"POST /1/rails/active_storage/direct_uploads": `{"signed_id":"BLOB","direct_upload":{"url":"/put","headers":{}}}`,
		"PUT /put": "",
	})
	p := filepath.Join(t.TempDir(), "hinge.png")
	os.WriteFile(p, []byte("x"), 0o644)
	r := runGo([]string{"upload", p})
	if r.code != 0 || r.stdout != "BLOB\n" || !strings.Contains(r.stderr, "the attachment may not render") {
		t.Fatalf("got %+v", r)
	}
}

// A comment with no text gets a preview line with no trailing spaces.
func TestCommentDeletePreviewEmptyBody(t *testing.T) {
	acctServer(t, map[string]string{
		"GET /1/cards/87/comments/c1": `{"id":"c1","created_at":"2026-10-17T10:00:00Z","body":{"plain_text":"","html":""},"creator":{"id":"u1","name":"Alex"}}`,
	})
	r := runGo([]string{"comment", "delete", "87", "c1"})
	if r.code != 2 || r.stderr != "about to delete comment c1  Alex  2026-10-17T10:00:00Z\nrefusing without --yes\n" {
		t.Errorf("got %+v", r)
	}
}

// A 403 on the board list says nothing about webhooks, so fz does not
// blame the admin role for it.
func TestWebhookBoardLookup403IsNotAboutAdmins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "list", "Gate"})
	if r.code != 1 || strings.Contains(r.stderr, "admins") || !strings.Contains(r.stderr, "GET /1/boards -> HTTP 403") {
		t.Errorf("got %+v", r)
	}
}
