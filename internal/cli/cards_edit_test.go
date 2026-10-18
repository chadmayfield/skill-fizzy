package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type seenCall struct{ method, path, body string }

// recordingServer starts a test server that records each request's method,
// path and body, sets a JSON content type, and then calls h. The function it
// returns gives a copy of the calls so far.
func recordingServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []seenCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seenCall{r.Method, r.URL.Path, string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenCall(nil), calls...)
	}
}

// editServer answers GET /1/cards/5 with card and records every call.
func editServer(t *testing.T, card string) (*httptest.Server, func() []seenCall) {
	t.Helper()
	return recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, card)
			return
		}
		io.WriteString(w, "{}")
	})
}

func editEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	parityEnv(t, srv)
}

func putBody(t *testing.T, calls []seenCall) map[string]map[string]string {
	t.Helper()
	for _, c := range calls {
		if c.method == http.MethodPut {
			var m map[string]map[string]string
			if err := json.Unmarshal([]byte(c.body), &m); err != nil {
				t.Fatalf("PUT body %q: %v", c.body, err)
			}
			return m
		}
	}
	t.Fatal("no PUT sent")
	return nil
}

func TestCardEditCommandRegistered(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"card", "edit"})
	if err != nil || cmd.Name() != "edit" || cmd.Parent().Name() != "card" {
		t.Fatalf("card edit not found: %v", err)
	}
	if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
		t.Error("card edit needs Short, Long and Example")
	}
}

func TestCardEditTitleOnly(t *testing.T) {
	srv, calls := editServer(t, "{}")
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--title", "New"})
	if r.code != 0 || r.stdout != "card #5 updated: title\n" {
		t.Fatalf("got %+v", r)
	}
	cs := calls()
	if len(cs) != 1 || cs[0].path != "/1/cards/5" || cs[0].body != `{"card":{"title":"New"}}` {
		t.Errorf("calls %+v", cs)
	}
}

func TestCardEditDescriptionAndStatusSorted(t *testing.T) {
	srv, calls := editServer(t, "{}")
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--description", "Gate latches.", "--status", "published", "--title", "T"})
	if r.code != 0 || r.stdout != "card #5 updated: description, status, title\n" {
		t.Fatalf("got %+v", r)
	}
	m := putBody(t, calls())
	if m["card"]["description"] != "<p>Gate latches.</p>" || m["card"]["status"] != "published" || m["card"]["title"] != "T" {
		t.Errorf("body %+v", m)
	}
}

func TestCardEditDescriptionFile(t *testing.T) {
	srv, calls := editServer(t, "{}")
	editEnv(t, srv)
	f := filepath.Join(t.TempDir(), "goal.md")
	if err := os.WriteFile(f, []byte("## Goal\r\nGate latches.\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runGo([]string{"card", "edit", "5", "--description-file", f})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	d := putBody(t, calls())["card"]["description"]
	if !strings.Contains(d, "<h3>Goal</h3>") || strings.Contains(d, "\r") {
		t.Errorf("description %q", d)
	}
}

func TestCardEditJSON(t *testing.T) {
	srv, _ := editServer(t, "{}")
	editEnv(t, srv)
	r := runGo([]string{"--json", "card", "edit", "5", "--title", "New", "--status", "drafted"})
	want := "{\n  \"number\": 5,\n  \"changed\": [\n    \"status\",\n    \"title\"\n  ]\n}\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v\nwant %q", r, want)
	}
}

func TestCardEditReplaceReadsThenPutsUnwrapped(t *testing.T) {
	card := `{"description_html": "<div class=\"action-text-content\">\n  <p>Hinge H1 is serviced</p>\n</div>\n"}`
	srv, calls := editServer(t, card)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "Hinge H1 ", "Hinge H1S "})
	if r.code != 0 || r.stdout != "card #5 updated: description\n" {
		t.Fatalf("got %+v", r)
	}
	cs := calls()
	if len(cs) != 2 || cs[0].method != "GET" || cs[1].method != "PUT" {
		t.Fatalf("calls %+v", cs)
	}
	if got := putBody(t, cs)["card"]["description"]; got != "<p>Hinge H1S is serviced</p>" {
		t.Errorf("description %q", got)
	}
}

func TestCardEditReplaceAmbiguousNeedsAll(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>H1 and H1S</p>"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "H1", "H1S"})
	if r.code != 2 || !strings.Contains(r.stderr, "2 times") {
		t.Fatalf("got %+v", r)
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("wrote %+v", c)
		}
	}
	r = runGo([]string{"card", "edit", "5", "--replace", "H1", "H2", "--all"})
	if r.code != 0 {
		t.Fatalf("with --all: %+v", r)
	}
	if got := putBody(t, calls())["card"]["description"]; got != "<p>H2 and H2S</p>" {
		t.Errorf("description %q", got)
	}
}

func TestCardEditReplaceDryRun(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>gate</p>"}`)
	editEnv(t, srv)
	r := runGo([]string{"--dry-run", "card", "edit", "5", "--replace", "gate", "hinge"})
	if r.code != 0 || !strings.Contains(r.stderr, `[dry-run] PUT `+srv.URL+`/1/cards/5 {"card": {"description": "<p>hinge</p>"}}`) {
		t.Fatalf("got %+v", r)
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("wrote %+v", c)
		}
	}
}

func TestCardEditErrors(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>x</p>"}`)
	editEnv(t, srv)
	descFile := filepath.Join(t.TempDir(), "goal.md")
	writeFile(t, descFile, "Fix the gate")
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"nothing", []string{"card", "edit", "5"}, 2, "nothing to update"},
		{"missing phrase", []string{"card", "edit", "5", "--replace", "y", "z"}, 1, "not found"},
		{"replace needs both", []string{"card", "edit", "5", "--replace", "y"}, 2, "--replace needs OLD and NEW"},
		{"extra args", []string{"card", "edit", "5", "y", "z"}, 2, "OLD and NEW only go with --replace"},
		{"all alone", []string{"card", "edit", "5", "--all", "--title", "T"}, 2, "--all only goes with --replace"},
		{"replace and description", []string{"card", "edit", "5", "--replace", "x", "y", "--description", "d"}, 2, "use --replace or a new description, not both"},
		{"replace and description file", []string{"card", "edit", "5", "--replace", "x", "y", "--description-file", descFile}, 2, "use --replace or a new description, not both"},
		{"missing description file", []string{"card", "edit", "5", "--description-file", filepath.Join(t.TempDir(), "nope.md")}, 2, "--description-file"},
		{"blank title", []string{"card", "edit", "5", "--title", "   "}, 2, "the card title cannot be empty"},
		{"empty title", []string{"card", "edit", "5", "--title", ""}, 2, "the card title cannot be empty"},
		{"description and file", []string{"card", "edit", "5", "--description", "d", "--description-file", "f"}, 2, "not allowed with"},
		{"bad status", []string{"card", "edit", "5", "--status", "gone"}, 2, "invalid choice"},
		{"bad number", []string{"card", "edit", "abc", "--title", "T"}, 2, "invalid int value"},
		{"empty old", []string{"card", "edit", "5", "--replace", "", "z"}, 2, "OLD must not be empty"},
		{"allow-empty alone", []string{"card", "edit", "5", "--allow-empty", "--title", "T"}, 2, "--allow-empty only goes with --description, --description-file or --replace"},
	}
	for _, tc := range cases {
		r := runGo(tc.args)
		if r.code != tc.wantCode || !strings.Contains(r.stderr, tc.wantErr) {
			t.Errorf("%s: got %+v, want code %d and %q", tc.name, r, tc.wantCode, tc.wantErr)
		}
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("an error case wrote %+v", c)
		}
	}
}

func TestCardEditRefusesEmptyDescription(t *testing.T) {
	srv, calls := editServer(t, "{}")
	editEnv(t, srv)
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, []byte(" \n\t\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"card", "edit", "5", "--description", ""},
		{"card", "edit", "5", "--description", "  \n"},
		{"card", "edit", "5", "--description-file", empty},
		{"card", "edit", "5", "--description", "", "--title", "T"},
	} {
		r := runGo(args)
		if r.code != 2 || !strings.Contains(r.stderr, "the new description is empty; add --allow-empty to clear it") {
			t.Errorf("fz %q: got %+v", args, r)
		}
	}
	if cs := calls(); len(cs) != 0 {
		t.Fatalf("a refused edit sent %+v", cs)
	}
	r := runGo([]string{"card", "edit", "5", "--description", "", "--allow-empty"})
	if r.code != 0 || r.stdout != "card #5 updated: description\n" {
		t.Fatalf("with --allow-empty: %+v", r)
	}
	if cs := calls(); len(cs) != 1 || cs[0].body != `{"card":{"description":""}}` {
		t.Errorf("calls %+v", cs)
	}
}

func TestCardEditReplaceKeepsLinks(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>Buy a <a href=\"https://example.com/hinge\">hinge</a></p>"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "hinge", "hinges"})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	want := `<p>Buy a <a href="https://example.com/hinge">hinges</a></p>`
	if got := putBody(t, calls())["card"]["description"]; got != want {
		t.Errorf("description %q", got)
	}
}

func TestCardEditReplaceRefusesEmptyResult(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<div class=\"action-text-content\"><p>Hinge</p></div>"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "Hinge", ""})
	if r.code != 2 || !strings.Contains(r.stderr, "the new description is empty; add --allow-empty to clear it") {
		t.Fatalf("got %+v", r)
	}
	for _, c := range calls() {
		if c.method != http.MethodGet {
			t.Errorf("a refused edit wrote %+v", c)
		}
	}
	r = runGo([]string{"card", "edit", "5", "--replace", "Hinge", "", "--allow-empty"})
	if r.code != 0 || r.stdout != "card #5 updated: description\n" {
		t.Fatalf("with --allow-empty: %+v", r)
	}
	if got := putBody(t, calls())["card"]["description"]; got != "<p></p>" {
		t.Errorf("description %q", got)
	}
}

func TestCardEditReplaceKeepsImageOnlyResult(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>Hinge</p><action-text-attachment sgid=\"x\"></action-text-attachment>"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "Hinge", ""})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	if got := putBody(t, calls())["card"]["description"]; !strings.Contains(got, "action-text-attachment") {
		t.Errorf("description %q", got)
	}
}

// A --replace that leaves only an image is not empty.
func TestCardEditReplaceKeepsImgOnlyResult(t *testing.T) {
	srv, calls := editServer(t, `{"description_html": "<p>Hinge</p><img src=\"hinge.jpg\">"}`)
	editEnv(t, srv)
	r := runGo([]string{"card", "edit", "5", "--replace", "Hinge", ""})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	if got := putBody(t, calls())["card"]["description"]; !strings.Contains(got, "<img") {
		t.Errorf("description %q", got)
	}
}

// card create and step edit refuse what they cannot use with exit 2.
func TestCreateAndStepEditUsageErrors(t *testing.T) {
	srv, calls := editServer(t, `{}`)
	editEnv(t, srv)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"card", "create", "Home", "Fix the gate", "--description-file", filepath.Join(t.TempDir(), "nope.md")}, "--description-file"},
		{[]string{"step", "edit", "87", "03abc123def456ghi789jkl0mn", ""}, "the step text cannot be empty"},
		{[]string{"step", "edit", "87", "03abc123def456ghi789jkl0mn", "  "}, "the step text cannot be empty"},
	} {
		r := runGo(tc.args)
		if r.code != 2 || !strings.Contains(r.stderr, tc.want) {
			t.Errorf("fz %q: %+v, want exit 2 and %q", tc.args, r, tc.want)
		}
	}
	for _, c := range calls() {
		if c.method != http.MethodGet {
			t.Errorf("wrote %+v", c)
		}
	}
}
