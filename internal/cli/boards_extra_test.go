package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// bxServer serves one board "b1" (Home) with one custom column "c1" (Next)
// and records every call.
func bxServer(t *testing.T, allAccess bool) (*httptest.Server, func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []seenCall
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seenCall{r.Method, r.URL.Path, string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		key := r.Method + " " + r.URL.Path
		access := "false"
		if allAccess {
			access = "true"
		}
		switch key {
		case "GET /1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home","auto_postpone_period_in_days":30}]`)
		case "GET /1/boards/b1":
			io.WriteString(w, `{"id":"b1","name":"Home","all_access":`+access+`,"auto_postpone_period_in_days":30,"creator":{"name":"Pat"},"user_ids":["u1","u2"]}`)
		case "GET /1/boards/b1/accesses":
			io.WriteString(w, `{"board_id":"b1","all_access":`+access+`,"users":[{"id":"u1","name":"Pat","role":"owner","has_access":true,"involvement":"watching"},{"id":"u2","name":"Sam","role":"member","has_access":false,"involvement":null}]}`)
		case "GET /1/cards":
			switch r.URL.Query().Get("indexed_by") {
			case "closed":
				io.WriteString(w, `[{"number":90,"title":"Old"}]`)
				return
			case "not_now":
				io.WriteString(w, `[{"number":91,"title":"a"},{"number":92,"title":"b"},{"number":93,"title":"c"}]`)
				return
			}
			io.WriteString(w, `[{"number":87,"title":"Fix the gate","column":{"id":"c1","name":"Next"}},{"number":88,"title":"Oil the hinge"}]`)
		case "GET /1/boards/b1/columns":
			io.WriteString(w, `[{"id":"c1","name":"Next","color":{"name":"Lime","value":"var(--color-card-4)"}}]`)
		case "GET /1/boards/b1/columns/c1":
			io.WriteString(w, `{"id":"c1","name":"Next","color":{"name":"Lime","value":"var(--color-card-4)"},"cards_url":"x"}`)
		case "GET /1/boards/b1/columns/c1/cards":
			io.WriteString(w, `[{"number":87,"title":"Fix the gate","column":{"id":"c1","name":"Next"}}]`)
		case "POST /1/boards/b1/publication":
			w.WriteHeader(201)
			io.WriteString(w, `{"id":"b1","public_url":"https://fizzy.example.com/1/public/boards/xyz"}`)
		case "POST /1/boards/b1/columns":
			w.Header().Set("Location", srv.URL+"/1/boards/b1/columns/c2.json")
			w.WriteHeader(201)
		case "PUT /1/boards/b1":
			io.WriteString(w, `{}`)
		default:
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenCall(nil), calls...)
	}
}

func writes(cs []seenCall) []seenCall {
	var out []seenCall
	for _, c := range cs {
		if c.method != "GET" {
			out = append(out, c)
		}
	}
	return out
}

func TestBoardColumnCommandsRegistered(t *testing.T) {
	for _, path := range [][]string{
		{"board", "show"}, {"board", "edit"}, {"board", "delete"}, {"board", "publish"}, {"board", "unpublish"}, {"board", "access"},
		{"column", "show"}, {"column", "create"}, {"column", "edit"}, {"column", "delete"}, {"column", "cards"},
	} {
		cmd, _, err := NewRoot().Find(path)
		if err != nil || cmd.Name() != path[1] || cmd.Parent().Name() != path[0] {
			t.Errorf("%v not found: %v", path, err)
			continue
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Errorf("%v needs Short, Long and Example", path)
		}
	}
}

func TestBoardShow(t *testing.T) {
	srv, _ := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "show", "home"})
	if r.code != 0 || !strings.Contains(r.stdout, "b1  Home  (postpone 30d)") || !strings.Contains(r.stdout, "access: 2 users") || !strings.Contains(r.stdout, "creator: Pat") {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "board", "show", "b1"})
	if r.code != 0 || !strings.Contains(r.stdout, `"user_ids"`) {
		t.Fatalf("json: %+v", r)
	}
}

func TestBoardEdit(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "edit", "Home", "--name", "Garden", "--postpone", "90", "--public-description", "Hinge **notes**"})
	if r.code != 0 || r.stdout != "board Home updated: name, postpone, public-description\n" {
		t.Fatalf("got %+v", r)
	}
	w := writes(calls())
	want := `{"board":{"name":"Garden","auto_postpone_period_in_days":90,"public_description":"<p>Hinge <strong>notes</strong></p>"}}`
	body := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(w[0].body)
	if len(w) != 1 || w[0].path != "/1/boards/b1" || body != want {
		t.Errorf("writes %+v", w)
	}
}

func TestBoardEditErrors(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	for name, tc := range map[string]struct {
		args    []string
		wantErr string
	}{
		"nothing":      {[]string{"board", "edit", "Home"}, "give --name, --postpone or --public-description"},
		"bad postpone": {[]string{"board", "edit", "Home", "--postpone", "5"}, "invalid choice"},
	} {
		r := runGo(tc.args)
		if r.code != 2 || !strings.Contains(r.stderr, tc.wantErr) {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if len(writes(calls())) != 0 {
		t.Error("an error case wrote")
	}
}

func TestBoardEditDryRun(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"--dry-run", "board", "edit", "Home", "--name", "Garden"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] PUT "+srv.URL+`/1/boards/b1 {"board": {"name": "Garden"}}`) {
		t.Fatalf("got %+v", r)
	}
	if len(writes(calls())) != 0 {
		t.Error("dry run wrote")
	}
}

func TestBoardDeleteRefusesWithoutYes(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "delete", "Home"})
	if r.code != 2 || !strings.Contains(r.stderr, "Home") || !strings.Contains(r.stderr, "2 open, 1 closed, 3 not now cards") ||
		!strings.Contains(r.stderr, "Next") || !strings.Contains(r.stderr, "refusing without --yes") {
		t.Fatalf("got %+v", r)
	}
	if len(writes(calls())) != 0 {
		t.Errorf("refusal sent %+v", writes(calls()))
	}
}

func TestBoardDeleteWithYes(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "delete", "Home", "--yes"})
	w := writes(calls())
	if r.code != 0 || r.stdout != "board Home deleted\n" || len(w) != 1 || w[0].method != "DELETE" || w[0].path != "/1/boards/b1" {
		t.Fatalf("got %+v, writes %+v", r, w)
	}
}

func TestBoardPublish(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "publish", "Home"})
	if r.code != 2 || !strings.Contains(r.stderr, "anyone with the link") || !strings.Contains(r.stderr, "refusing without --yes") || len(writes(calls())) != 0 {
		t.Fatalf("refusal: %+v writes %+v", r, writes(calls()))
	}
	r = runGo([]string{"board", "publish", "Home", "--yes"})
	if r.code != 0 || r.stdout != "board Home published: https://fizzy.example.com/1/public/boards/xyz\n" {
		t.Fatalf("got %+v", r)
	}
	if w := writes(calls()); len(w) != 1 || w[0].method != "POST" || w[0].path != "/1/boards/b1/publication" {
		t.Errorf("writes %+v", w)
	}
}

func TestBoardUnpublishNeedsNoYes(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "unpublish", "Home"})
	w := writes(calls())
	if r.code != 0 || r.stdout != "board Home unpublished\n" || len(w) != 1 || w[0].method != "DELETE" || w[0].path != "/1/boards/b1/publication" {
		t.Fatalf("got %+v writes %+v", r, w)
	}
}

func TestBoardAccessList(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "access", "Home"})
	want := "all access: no\nPat  (owner)  watching\nSam  (member)  no access\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("got %+v", r)
	}
	if len(writes(calls())) != 0 {
		t.Error("listing wrote")
	}
}

func TestBoardAccessChangeRefusesWithoutYes(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	for _, args := range [][]string{
		{"board", "access", "Home", "--all"},
		{"board", "access", "Home", "--users", "u1,u3"},
	} {
		r := runGo(args)
		if r.code != 2 || !strings.Contains(r.stderr, "refusing without --yes") || !strings.Contains(r.stderr, "Home") {
			t.Fatalf("%v: %+v", args, r)
		}
		if len(writes(calls())) != 0 {
			t.Fatalf("%v: refusal sent %+v", args, writes(calls()))
		}
	}
	r := runGo([]string{"board", "access", "Home", "--users", "u1,u3"})
	if !strings.Contains(r.stderr, "replaces the whole list") || !strings.Contains(r.stderr, "u1, u3") {
		t.Errorf("refusal text: %s", r.stderr)
	}
}

func TestBoardAccessChangeWithYes(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "access", "Home", "--users", "u1, u3", "--yes"})
	w := writes(calls())
	if r.code != 0 || len(w) != 1 || w[0].method != "PUT" || w[0].body != `{"board":{"all_access":false,"user_ids":["u1","u3"]}}` {
		t.Fatalf("got %+v writes %+v", r, w)
	}
	r = runGo([]string{"board", "access", "Home", "--all", "--yes"})
	w = writes(calls())
	if r.code != 0 || len(w) != 2 || w[1].body != `{"board":{"all_access":true}}` {
		t.Fatalf("all: %+v writes %+v", r, w)
	}
}

func TestBoardAccessUsageErrors(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	for name, args := range map[string][]string{
		"both":      {"board", "access", "Home", "--all", "--users", "u1", "--yes"},
		"empty":     {"board", "access", "Home", "--users", " , ", "--yes"},
		"yes alone": {"board", "access", "Home", "--yes"},
	} {
		r := runGo(args)
		if r.code != 2 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if len(writes(calls())) != 0 {
		t.Error("an error case wrote")
	}
}

func TestBoardAccessDryRun(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"--dry-run", "board", "access", "Home", "--all", "--yes"})
	if r.code != 0 || !strings.Contains(r.stderr, "[dry-run] PUT "+srv.URL+`/1/boards/b1 {"board": {"all_access": true}}`) || len(writes(calls())) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestColumnShow(t *testing.T) {
	srv, _ := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "show", "Home", "next"})
	if r.code != 0 || r.stdout != "c1  Next  (color Lime)\n" {
		t.Fatalf("got %+v", r)
	}
	r = runGo([]string{"--json", "column", "show", "Home", "c1"})
	if r.code != 0 || !strings.Contains(r.stdout, `"cards_url"`) {
		t.Fatalf("json: %+v", r)
	}
}

func TestColumnCreate(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "create", "Home", "Soon", "--color", "lime"})
	w := writes(calls())
	if r.code != 0 || r.stdout != "column 'Soon' created: c2\n" || len(w) != 1 || w[0].body != `{"column":{"name":"Soon","color":"var(--color-card-4)"}}` {
		t.Fatalf("got %+v writes %+v", r, w)
	}
}

func TestColumnEdit(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "edit", "Home", "Next", "--name", "Soon", "--color", "var(--color-card-2)"})
	w := writes(calls())
	if r.code != 0 || r.stdout != "column Next updated: color, name\n" || len(w) != 1 || w[0].path != "/1/boards/b1/columns/c1" ||
		w[0].body != `{"column":{"name":"Soon","color":"var(--color-card-2)"}}` {
		t.Fatalf("got %+v writes %+v", r, w)
	}
	r = runGo([]string{"column", "edit", "Home", "Next"})
	if r.code != 2 || !strings.Contains(r.stderr, "give --name or --color") {
		t.Errorf("nothing: %+v", r)
	}
}

func TestColumnColorNames(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "create", "Home", "Soon", "--color", "Blue"})
	w := writes(calls())
	if r.code != 0 || len(w) != 1 || w[0].body != `{"column":{"name":"Soon","color":"var(--color-card-default)"}}` {
		t.Fatalf("create Blue: %+v writes %+v", r, w)
	}
	r = runGo([]string{"column", "edit", "Home", "Next", "--color", "PINK"})
	w = writes(calls())
	if r.code != 0 || len(w) != 2 || w[1].body != `{"column":{"color":"var(--color-card-8)"}}` {
		t.Fatalf("edit PINK: %+v writes %+v", r, w)
	}
	r = runGo([]string{"column", "create", "Home", "Soon", "--color", "4"})
	w = writes(calls())
	if r.code != 0 || len(w) != 3 || w[2].body != `{"column":{"name":"Soon","color":"var(--color-card-4)"}}` {
		t.Fatalf("create 4: %+v writes %+v", r, w)
	}
	r = runGo([]string{"column", "edit", "Home", "Next", "--color", "DEFAULT"})
	w = writes(calls())
	if r.code != 0 || len(w) != 4 || w[3].body != `{"column":{"color":"var(--color-card-default)"}}` {
		t.Fatalf("edit DEFAULT: %+v writes %+v", r, w)
	}
}

func TestColumnColorRefused(t *testing.T) {
	for _, argv := range [][]string{
		{"column", "create", "Home", "Soon", "--color", "red"},
		{"column", "create", "Home", "Soon", "--color", "var(--color-card-blue)"},
		{"column", "create", "Home", "Soon", "--color", ""},
		{"column", "edit", "Home", "Next", "--color", "red"},
		{"column", "edit", "Home", "Next", "--name", "Soon", "--color", "9"},
		{"--dry-run", "column", "edit", "Home", "Next", "--color", "red"},
	} {
		srv, calls := bxServer(t, false)
		editEnv(t, srv)
		r := runGo(argv)
		if r.code != 2 || r.stdout != "" || !strings.Contains(r.stderr, "Blue, Gray, Tan, Yellow, Lime, Aqua, Violet, Purple, Pink") || !strings.Contains(r.stderr, "default, 1 to 8") {
			t.Errorf("%v: %+v", argv, r)
		}
		if len(calls()) != 0 {
			t.Errorf("%v: sent %+v", argv, calls())
		}
	}
}

func TestColumnDelete(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "delete", "Home", "Next"})
	if r.code != 2 || !strings.Contains(r.stderr, "Next") || !strings.Contains(r.stderr, "1 open card") || !strings.Contains(r.stderr, "refusing without --yes") || len(writes(calls())) != 0 {
		t.Fatalf("refusal: %+v writes %+v", r, writes(calls()))
	}
	r = runGo([]string{"column", "delete", "Home", "Next", "--yes"})
	w := writes(calls())
	if r.code != 0 || r.stdout != "column Next deleted\n" || len(w) != 1 || w[0].method != "DELETE" || w[0].path != "/1/boards/b1/columns/c1" {
		t.Fatalf("got %+v writes %+v", r, w)
	}
}

func TestColumnCards(t *testing.T) {
	srv, _ := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"column", "cards", "Home", "Next"})
	if r.code != 0 || r.stdout != "#87    [Next] Fix the gate\n" {
		t.Fatalf("got %+v", r)
	}
}

func TestExistingListsUnchangedByNewCommands(t *testing.T) {
	srv, _ := bxServer(t, false)
	editEnv(t, srv)
	if r := runGo([]string{"board", "list"}); r.code != 0 || r.stdout != "b1  Home  (postpone 30d)\n" {
		t.Errorf("board list: %+v", r)
	}
}
