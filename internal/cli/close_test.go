package cli

import (
	"strings"
	"testing"
)

const (
	openCard   = `{"number":87,"title":"Fix the gate","closed":false,"board":{"id":"b1","name":"Home"}}`
	closedCard = `{"number":87,"title":"Fix the gate","closed":true,"board":{"id":"b1","name":"Home"}}`
)

func TestCardCloseSendsClosure(t *testing.T) {
	_, calls := extraServer(t, openCard)
	r := runGo([]string{"card", "close", "87"})
	if r.code != 0 || r.stdout != "card #87 closed\n" || r.stderr != "" {
		t.Errorf("got %+v", r)
	}
	w := extraWrites(calls())
	if len(w) != 1 || w[0].method != "POST" || w[0].path != "/1/cards/87/closure" {
		t.Errorf("writes %+v", w)
	}
	// No column lookup: a board with a custom column named Done is no obstacle.
	for _, c := range calls() {
		if strings.HasSuffix(c.path, "/columns") {
			t.Errorf("read the columns: %+v", c)
		}
	}
}

func TestCardCloseJSON(t *testing.T) {
	_, _ = extraServer(t, openCard)
	r := runGo([]string{"--json", "card", "close", "87"})
	if r.code != 0 || r.stdout != "{\n  \"number\": 87,\n  \"changed\": true\n}\n" {
		t.Errorf("got %+v", r)
	}
}

func TestCardCloseDryRun(t *testing.T) {
	_, calls := extraServer(t, openCard)
	r := runGo([]string{"--dry-run", "card", "close", "87"})
	if r.code != 0 || !strings.HasSuffix(r.stdout, "dry-run: nothing sent; would close card #87\n") {
		t.Errorf("text: %+v", r)
	}
	r = runGo([]string{"--dry-run", "--json", "card", "close", "87"})
	if r.code != 0 || !strings.Contains(r.stdout, "\"changed\": true") || !strings.HasSuffix(r.stdout, "\"dry_run\": true\n}\n") {
		t.Errorf("json: %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 0 {
		t.Errorf("dry run sent %+v", w)
	}
}

func TestCardCloseAlreadyClosed(t *testing.T) {
	_, calls := extraServer(t, closedCard)
	n := func() int { return len(extraWrites(calls())) }
	r := runGo([]string{"card", "close", "87"})
	if r.code != 0 || r.stdout != "card #87 is already closed; no change needed\n" || r.stderr != "" {
		t.Errorf("got %+v", r)
	}
	checkNoChange(t, []string{"card", "close", "87"}, n)
}

func TestCardCloseHasNoYes(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"card", "close"})
	if err != nil || cmd.Name() != "close" {
		t.Fatalf("card close not found: %v", err)
	}
	if cmd.Flags().Lookup("yes") != nil {
		t.Error("card close should not take --yes")
	}
}
