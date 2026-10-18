package fizzy

import (
	"net/http"
	"strings"
	"testing"
)

const hookJSON = `{"id":"H1","name":"Gate alerts","payload_url":"https://hooks.example.com/gate","active":true,"signing_secret":"s3cret","subscribed_actions":["card_closed","comment_created"],"created_at":"2026-01-02T03:04:05.000Z","url":"https://fizzy.example.com/1/boards/B1/webhooks/H1","board":{"id":"B1","name":"Gate"}}`

func TestWebhookDecodeKeepsRaw(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[" + hookJSON + "]"))
	})
	hooks, err := f.Webhooks("B1")
	if err != nil || len(hooks) != 1 {
		t.Fatalf("got %v %v", hooks, err)
	}
	h := hooks[0]
	if h.ID != "H1" || h.Name != "Gate alerts" || h.PayloadURL != "https://hooks.example.com/gate" || !h.Active ||
		len(h.SubscribedActions) != 2 || h.Board.ID != "B1" {
		t.Errorf("decoded %+v", h)
	}
	if string(h.Raw()) != hookJSON {
		t.Errorf("raw %s", h.Raw())
	}
}

func TestWebhookPaths(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deliveries") {
			w.Write([]byte(`[{"id":"D1","state":"completed","request":null,"response":{"code":200,"error":null},"event":{"action":"card_closed"}}]`))
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/webhooks") {
			w.Write([]byte("[" + hookJSON + "]"))
			return
		}
		w.Write([]byte(hookJSON))
	})
	if _, err := f.Webhook("B1", "H1"); err != nil {
		t.Fatal(err)
	}
	ds, err := f.WebhookDeliveries("B1", "H1")
	if err != nil || len(ds) != 1 || ds[0].State != "completed" || ds[0].Response.Code != 200 || ds[0].Request != nil || ds[0].Event.Action != "card_closed" {
		t.Fatalf("deliveries %+v %v", ds, err)
	}
	if err := f.DeleteWebhook("B1", "H1"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range s.calls {
		got = append(got, c.Method+" "+c.Path)
	}
	want := []string{"GET /1/boards/B1/webhooks/H1", "GET /1/boards/B1/webhooks/H1/deliveries", "DELETE /1/boards/B1/webhooks/H1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v", got)
	}
}

func TestCreateWebhook(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/boards/B1/webhooks/H1.json")
		w.WriteHeader(201)
		w.Write([]byte(hookJSON))
	})
	h, err := f.CreateWebhook("B1", "Gate alerts", "https://hooks.example.com/gate", []string{"card_closed", "comment_created"})
	if err != nil || h.ID != "H1" {
		t.Fatalf("got %+v %v", h, err)
	}
	c := s.calls[0]
	want := `{"webhook":{"name":"Gate alerts","url":"https://hooks.example.com/gate","subscribed_actions":["card_closed","comment_created"]}}`
	if c.Method != "POST" || c.Path != "/1/boards/B1/webhooks" || strings.ReplaceAll(strings.ReplaceAll(string(c.Body), ", ", ","), ": ", ":") != want {
		t.Errorf("sent %s %s %s", c.Method, c.Path, c.Body)
	}
}

func TestCreateWebhookFallsBackToLocation(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/boards/B1/webhooks/H9.json")
		w.WriteHeader(201)
	})
	h, err := f.CreateWebhook("B1", "n", "https://hooks.example.com/x", []string{"card_closed"})
	if err != nil || h.ID != "H9" {
		t.Fatalf("got %+v %v", h, err)
	}
}

func TestCreateWebhookDryRun(t *testing.T) {
	f, s, out, _ := newFizzy(t, true, nil)
	h, err := f.CreateWebhook("B1", "n", "https://hooks.example.com/x", []string{"card_closed"})
	if err != nil || h.ID != "" || len(s.calls) != 0 {
		t.Fatalf("got %+v %v calls %d", h, err, len(s.calls))
	}
	if !strings.HasPrefix(out.String(), "[dry-run] POST ") {
		t.Errorf("out %q", out)
	}
}

func TestCreateWebhookValidates(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, nil)
	cases := []struct {
		url     string
		actions []string
		want    string
	}{
		{"http://hooks.example.com/x", []string{"card_closed"}, "https"},
		{"ftp://hooks.example.com/x", []string{"card_closed"}, "https"},
		{"https:///x", []string{"card_closed"}, "host"},
		{"not a url", []string{"card_closed"}, "https"},
		{"https://hooks.example.com/x", []string{"card_nope"}, "card_nope"},
		{"https://hooks.example.com/x", nil, "at least one"},
	}
	for _, c := range cases {
		_, err := f.CreateWebhook("B1", "n", c.url, c.actions)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: err %v, want %q", c, err, c.want)
		}
	}
	if len(s.calls) != 0 {
		t.Errorf("sent %d requests", len(s.calls))
	}
}

func TestWebhookURLLocalhostMayBeHTTP(t *testing.T) {
	for _, u := range []string{"http://localhost:8080/x", "http://127.0.0.1/x", "http://[::1]:9/x", "https://hooks.example.com/x"} {
		if err := CheckWebhookURL(u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	if err := CheckWebhookURL("http://localhost.example.com/x"); err == nil {
		t.Error("localhost.example.com must need https")
	}
}

func TestUpdateWebhookUsesPatch(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(hookJSON))
	})
	name := "Hinge alerts"
	if _, err := f.UpdateWebhook("B1", "H1", &name, []string{"card_closed"}); err != nil {
		t.Fatal(err)
	}
	c := s.calls[0]
	if c.Method != "PATCH" || c.Path != "/1/boards/B1/webhooks/H1" || !strings.Contains(string(c.Body), `"name"`) || strings.Contains(string(c.Body), `"url"`) {
		t.Errorf("sent %s %s %s", c.Method, c.Path, c.Body)
	}
	if _, err := f.UpdateWebhook("B1", "H1", nil, nil); err == nil {
		t.Error("empty update must fail")
	}
	if _, err := f.UpdateWebhook("B1", "H1", nil, []string{"bogus"}); err == nil {
		t.Error("bad action must fail")
	}
}

func TestActivateWebhookSkipsActive(t *testing.T) {
	inactive := strings.Replace(hookJSON, `"active":true`, `"active":false`, 1)
	body := inactive
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	})
	changed, err := f.ActivateWebhook("B1", "H1")
	if err != nil || !changed {
		t.Fatalf("inactive: %v %v", changed, err)
	}
	if len(s.calls) != 2 || s.calls[1].Method != "POST" || s.calls[1].Path != "/1/boards/B1/webhooks/H1/activation" {
		t.Errorf("calls %+v", s.calls)
	}
	body = hookJSON
	s.calls = nil
	changed, err = f.ActivateWebhook("B1", "H1")
	if err != nil || changed || len(s.calls) != 1 {
		t.Errorf("active: %v %v calls %d", changed, err, len(s.calls))
	}
}
