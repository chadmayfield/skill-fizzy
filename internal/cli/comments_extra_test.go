package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const cmtA = `{"id":"c1","created_at":"2026-01-02T03:04:05Z","body":{"plain_text":"Gate latches.\nSecond line.","html":"<p>x</p>"},"creator":{"name":"Pat Hinge"}}`

func commentServer(t *testing.T) (*httptest.Server, func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []seenCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seenCall{r.Method, r.URL.Path, string(b)})
		mu.Unlock()
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/reactions"):
			io.WriteString(w, `[{"id":"r1","content":"ok","reacter":{"name":"Pat Hinge"}}]`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/comments"):
			io.WriteString(w, "["+cmtA+"]")
		case r.Method == "GET":
			io.WriteString(w, cmtA)
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "c-forbidden"):
			w.WriteHeader(403)
			io.WriteString(w, "not yours")
		case r.Method == "POST":
			w.Header().Set("Location", "/1/cards/87/comments/c1/reactions/r2")
			w.WriteHeader(201)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenCall { mu.Lock(); defer mu.Unlock(); return append([]seenCall(nil), calls...) }
}

func TestCommentExtraRegistered(t *testing.T) {
	for _, n := range []string{"list", "show", "edit", "delete", "react", "reactions", "unreact"} {
		cmd, _, err := NewRoot().Find([]string{"comment", n})
		if err != nil || cmd.Name() != n || cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Errorf("comment %s: %v", n, err)
		}
	}
}

func TestCommentList(t *testing.T) {
	srv, _ := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "list", "87"})
	if r.code != 0 || r.stdout != "c1  Pat Hinge  2026-01-02T03:04:05Z  Gate latches.\n" {
		t.Fatalf("%+v", r)
	}
	r = runGo([]string{"--json", "comment", "list", "87"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "[\n  {\n    \"id\": \"c1\"") {
		t.Fatalf("%+v", r)
	}
}

func TestCommentShow(t *testing.T) {
	srv, _ := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "show", "87", "c1"})
	want := "c1  Pat Hinge  2026-01-02T03:04:05Z\nGate latches.\nSecond line.\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("%+v", r)
	}
}

func TestCommentEdit(t *testing.T) {
	srv, calls := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "edit", "87", "c1", "Gate **latches**."})
	if r.code != 0 || r.stdout != "comment c1 updated\n" {
		t.Fatalf("%+v", r)
	}
	cs := calls()
	if cs[0].method != "PUT" || cs[0].path != "/1/cards/87/comments/c1" || !strings.Contains(unescapeHTML(cs[0].body), `<strong>latches</strong>`) {
		t.Errorf("%+v", cs)
	}
}

func TestCommentEditForbidden(t *testing.T) {
	srv, _ := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "edit", "87", "c-forbidden", "x"})
	if r.code != 1 || !strings.Contains(r.stderr, "fz: ") || !strings.Contains(r.stderr, "403") {
		t.Fatalf("%+v", r)
	}
}

func TestCommentDeleteNeedsYes(t *testing.T) {
	srv, calls := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "delete", "87", "c1"})
	if r.code != 2 || !strings.Contains(r.stderr, "about to delete comment c1") || !strings.Contains(r.stderr, "refusing without --yes") {
		t.Fatalf("%+v", r)
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("wrote: %+v", c)
		}
	}
	r = runGo([]string{"comment", "delete", "87", "c1", "--yes"})
	if r.code != 0 || r.stdout != "comment c1 deleted\n" {
		t.Fatalf("%+v", r)
	}
	last := calls()[len(calls())-1]
	if last.method != "DELETE" || last.path != "/1/cards/87/comments/c1" {
		t.Errorf("%+v", last)
	}
}

func TestCommentDeleteDryRun(t *testing.T) {
	srv, calls := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"--dry-run", "comment", "delete", "87", "c1", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] DELETE ") {
		t.Fatalf("%+v", r)
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("wrote: %+v", c)
		}
	}
}

func TestCommentReactFlow(t *testing.T) {
	srv, calls := commentServer(t)
	editEnv(t, srv)
	r := runGo([]string{"comment", "react", "87", "c1", "ok"})
	if r.code != 0 || r.stdout != "reaction added: r2\n" {
		t.Fatalf("%+v", r)
	}
	if c := calls()[0]; c.method != "POST" || c.body != `{"reaction":{"content":"ok"}}` {
		t.Errorf("%+v", c)
	}
	r = runGo([]string{"comment", "reactions", "87", "c1"})
	if r.code != 0 || r.stdout != "r1  Pat Hinge  ok\n" {
		t.Fatalf("%+v", r)
	}
	r = runGo([]string{"comment", "unreact", "87", "c1", "r1"})
	if r.code != 0 || r.stdout != "reaction r1 removed\n" {
		t.Fatalf("%+v", r)
	}
}

// unescapeHTML undoes encoding/json's HTML escaping so bodies compare readably.
func unescapeHTML(s string) string {
	return strings.NewReplacer("\\u003c", "<", "\\u003e", ">", "\\u0026", "&").Replace(s)
}
