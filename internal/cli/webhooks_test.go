package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testWebhook = `{"id":"H1","name":"Gate alerts","payload_url":"https://hooks.example.com/gate","active":true,"signing_secret":"s3cret","subscribed_actions":["card_closed","comment_created"],"created_at":"2026-01-02T03:04:05.000Z"}`

// webhookServer serves one board "Gate" (id B1) and the webhook testWebhook, and
// records every call. Deliveries answer deliveriesStatus.
func webhookServer(t *testing.T, hook string, deliveriesStatus int) (*httptest.Server, func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []seenCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seenCall{r.Method, r.URL.Path, string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"B1","name":"Gate"}]`)
		case strings.HasSuffix(r.URL.Path, "/deliveries"):
			if deliveriesStatus != 200 {
				w.WriteHeader(deliveriesStatus)
				io.WriteString(w, "forbidden")
				return
			}
			io.WriteString(w, `[{"id":"D1","state":"completed","request":null,"response":{"code":200,"error":null},"event":{"action":"card_closed"}},{"id":"D2","state":"pending","request":null,"response":null,"event":{"action":"comment_created"}}]`)
		case r.URL.Path == "/1/boards/B1/webhooks" && r.Method == http.MethodGet:
			io.WriteString(w, "["+hook+"]")
		case r.URL.Path == "/1/boards/B1/webhooks" && r.Method == http.MethodPost:
			w.Header().Set("Location", "/1/boards/B1/webhooks/H1.json")
			w.WriteHeader(201)
			io.WriteString(w, hook)
		default:
			io.WriteString(w, hook)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenCall(nil), calls...)
	}
}

func TestWebhookCommandsRegistered(t *testing.T) {
	for _, name := range []string{"list", "show", "create", "edit", "delete", "activate", "deliveries"} {
		cmd, _, err := NewRoot().Find([]string{"webhook", name})
		if err != nil || cmd.Name() != name || cmd.Parent().Name() != "webhook" {
			t.Errorf("webhook %s not found: %v", name, err)
			continue
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Errorf("webhook %s needs Short, Long and Example", name)
		}
	}
}

func TestWebhookListAndShow(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "list", "Gate"})
	want := "H1  [active] Gate alerts -> https://hooks.example.com/gate  card_closed,comment_created\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("list %+v", r)
	}
	if strings.Contains(r.stdout, "s3cret") {
		t.Error("list printed the signing secret")
	}
	r = runGo([]string{"webhook", "show", "B1", "H1"})
	if r.code != 0 || r.stdout != want || strings.Contains(r.stdout, "s3cret") {
		t.Fatalf("show %+v", r)
	}
	r = runGo([]string{"--json", "webhook", "show", "B1", "H1"})
	if r.code != 0 || !strings.Contains(r.stdout, `"payload_url": "https://hooks.example.com/gate"`) {
		t.Fatalf("show json %+v", r)
	}
	r = runGo([]string{"--json", "webhook", "list", "B1"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "[\n  {") {
		t.Fatalf("list json %+v", r)
	}
}

func TestWebhookCreateRefusesWithoutYes(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "create", "B1", "--name", "Gate alerts", "--url", "https://hooks.example.com/gate", "--actions", "card_closed,comment_created"})
	if r.code != 2 || r.stdout != "" {
		t.Fatalf("got %+v", r)
	}
	for _, want := range []string{"https://hooks.example.com/gate", "card_closed,comment_created", "refusing without --yes"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, r.stderr)
		}
	}
	if len(writes(calls())) != 0 {
		t.Error("sent a write without --yes")
	}
}

func TestWebhookCreate(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "create", "B1", "--name", "Gate alerts", "--url", "https://hooks.example.com/gate", "--actions", "card_closed,comment_created", "--yes"})
	// The secret goes to stderr with the warning, so stdout can be logged or
	// piped without it.
	if r.code != 0 || r.stdout != "webhook created: H1\n" ||
		!strings.HasSuffix(r.stderr, "save the signing secret now; fz hides it from list and show\nsigning secret: s3cret\n") {
		t.Fatalf("got %+v", r)
	}
	w := writes(calls())
	if len(w) != 1 || w[0].method != "POST" || w[0].path != "/1/boards/B1/webhooks" || !strings.Contains(w[0].body, `"subscribed_actions":["card_closed","comment_created"]`) {
		t.Errorf("writes %+v", w)
	}
}

func TestWebhookCreateUsageErrors(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	for _, args := range [][]string{
		{"webhook", "create", "B1", "--url", "https://h.example.com/x", "--actions", "card_closed", "--yes"},
		{"webhook", "create", "B1", "--name", "n", "--actions", "card_closed", "--yes"},
		{"webhook", "create", "B1", "--name", "n", "--url", "https://h.example.com/x", "--yes"},
		{"webhook", "create", "B1", "--name", "n", "--url", "http://h.example.com/x", "--actions", "card_closed", "--yes"},
		{"webhook", "create", "B1", "--name", "n", "--url", "https://h.example.com/x", "--actions", "card_nope", "--yes"},
	} {
		r := runGo(args)
		if r.code != 2 || !strings.HasPrefix(r.stderr, "fz: ") {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if len(calls()) != 0 {
		t.Errorf("made %d requests", len(calls()))
	}
}

func TestWebhookCreateDryRun(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"--dry-run", "webhook", "create", "B1", "--name", "n", "--url", "https://h.example.com/x", "--actions", "card_closed", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] POST") || len(writes(calls())) != 0 || !strings.Contains(r.stdout, "dry-run: nothing sent") || strings.Contains(r.stdout, "created") {
		t.Fatalf("got %+v", r)
	}
}

func TestWebhookEdit(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "edit", "B1", "H1", "--actions", "card_closed"})
	if r.code != 2 || len(writes(calls())) != 0 {
		t.Fatalf("no yes: %+v", r)
	}
	for _, want := range []string{"https://hooks.example.com/gate", "card_closed", "refusing without --yes"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, r.stderr)
		}
	}
	r = runGo([]string{"webhook", "edit", "B1", "H1", "--name", "Hinge alerts", "--actions", "card_closed", "--yes"})
	if r.code != 0 || r.stdout != "webhook H1 updated: name, actions\n" {
		t.Fatalf("got %+v", r)
	}
	w := writes(calls())
	if len(w) != 1 || w[0].method != "PATCH" || w[0].path != "/1/boards/B1/webhooks/H1" || strings.Contains(w[0].body, `"url"`) {
		t.Errorf("writes %+v", w)
	}
	r = runGo([]string{"webhook", "edit", "B1", "H1", "--yes"})
	if r.code != 2 {
		t.Errorf("empty edit: %+v", r)
	}
}

func TestWebhookDelete(t *testing.T) {
	srv, calls := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "delete", "B1", "H1"})
	if r.code != 2 || !strings.Contains(r.stderr, "https://hooks.example.com/gate") || !strings.Contains(r.stderr, "refusing without --yes") || len(writes(calls())) != 0 {
		t.Fatalf("no yes: %+v", r)
	}
	r = runGo([]string{"webhook", "delete", "B1", "H1", "--yes"})
	if r.code != 0 || r.stdout != "webhook H1 deleted\n" {
		t.Fatalf("got %+v", r)
	}
	w := writes(calls())
	if len(w) != 1 || w[0].method != "DELETE" || w[0].path != "/1/boards/B1/webhooks/H1" {
		t.Errorf("writes %+v", w)
	}
	r = runGo([]string{"--dry-run", "webhook", "delete", "B1", "H1", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] DELETE") || len(writes(calls())) != 1 {
		t.Errorf("dry run: %+v", r)
	}
}

func TestWebhookActivate(t *testing.T) {
	inactive := strings.Replace(testWebhook, `"active":true`, `"active":false`, 1)
	srv, calls := webhookServer(t, inactive, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "activate", "B1", "H1"})
	if r.code != 0 || r.stdout != "webhook H1 activated\n" {
		t.Fatalf("got %+v", r)
	}
	if w := writes(calls()); len(w) != 1 || w[0].path != "/1/boards/B1/webhooks/H1/activation" {
		t.Errorf("writes %+v", w)
	}
	srv2, calls2 := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv2)
	r = runGo([]string{"webhook", "activate", "B1", "H1"})
	if r.code != 0 || r.stdout != "webhook H1 already active\n" || len(writes(calls2())) != 0 {
		t.Errorf("already active: %+v", r)
	}
}

func TestWebhookDeliveries(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "deliveries", "B1", "H1"})
	want := "D1  completed  card_closed  HTTP 200\nD2  pending  comment_created  no response yet\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
}

func TestWebhookDeliveries403(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 403)
	parityEnv(t, srv)
	r := runGo([]string{"webhook", "deliveries", "B1", "H1"})
	if r.code != 1 || !strings.Contains(r.stderr, "account admin") || !strings.Contains(r.stderr, "403") {
		t.Fatalf("got %+v", r)
	}
}

func TestWebhookJSONHidesSecret(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	for _, args := range [][]string{{"--json", "webhook", "list", "B1"}, {"--json", "webhook", "show", "B1", "H1"}} {
		r := runGo(args)
		if r.code != 0 || strings.Contains(r.stdout, "signing_secret") || strings.Contains(r.stdout, "s3cret") {
			t.Errorf("%v: %+v", args, r)
		}
		if !strings.Contains(r.stdout, `"payload_url"`) || strings.Index(r.stdout, `"active"`) > strings.Index(r.stdout, `"subscribed_actions"`) {
			t.Errorf("%v: keys lost or reordered: %s", args, r.stdout)
		}
	}
	r := runGo([]string{"--json", "webhook", "show", "B1", "H1", "--reveal-secret"})
	if r.code != 0 || !strings.Contains(r.stdout, `"signing_secret": "s3cret"`) {
		t.Errorf("reveal: %+v", r)
	}
}

func TestWebhookCreateJSONShowsSecret(t *testing.T) {
	srv, _ := webhookServer(t, testWebhook, 200)
	parityEnv(t, srv)
	r := runGo([]string{"--json", "webhook", "create", "B1", "--name", "n", "--url", "https://h.example.com/x", "--actions", "card_closed", "--yes"})
	if r.code != 0 || !strings.Contains(r.stdout, `"signing_secret": "s3cret"`) || !strings.Contains(r.stderr, "save the signing secret now") {
		t.Errorf("%+v", r)
	}
}

func TestWebhookActivateDryRun(t *testing.T) {
	inactive := strings.Replace(testWebhook, `"active":true`, `"active":false`, 1)
	srv, calls := webhookServer(t, inactive, 200)
	parityEnv(t, srv)
	r := runGo([]string{"--dry-run", "webhook", "activate", "B1", "H1"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] POST") || !strings.Contains(r.stdout, "dry-run: nothing sent") || strings.Contains(r.stdout, "activated\n") || len(writes(calls())) != 0 {
		t.Errorf("%+v", r)
	}
}
