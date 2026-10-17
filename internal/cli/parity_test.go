package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stepA = "03abc123def456ghi789jkl0mn"

// fakeFizzy serves canned boards, columns and cards, and accepts writes the
// way Fizzy does: 201 with a Location header for creates, 204 otherwise.
func fakeFizzy(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	card := func(n int, title, column string, extra string) string {
		col := "null"
		if column != "" {
			col = fmt.Sprintf(`{"id":"c-%s","name":%q,"color":"var(--color-card-default)"}`, strings.ToLower(column), column)
		}
		return fmt.Sprintf(`{"id":"k%d","number":%d,"title":%q,"status":"published","board":{"id":"b1","name":"Home"},"column":%s,"url":"%s/1/cards/%d"%s}`,
			n, n, title, col, srv.URL, n, extra)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /1/boards", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"id":"b1","name":"Home","auto_postpone_period_in_days":30,"url":"`+srv.URL+`/1/boards/b1"},`+
			`{"name":"Garden","id":"b2","all_access":true}]`)
	})
	mux.HandleFunc("GET /1/boards/b1/columns", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"id":"c-later","name":"Later","color":"var(--color-card-default)"},{"id":"c-next","name":"Next","color":"var(--color-card-1)"},{"id":"c-now","name":"Now","color":"var(--color-card-2)"}]`)
	})
	mux.HandleFunc("GET /1/boards/b2/columns", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `[]`) })
	mux.HandleFunc("GET /1/cards", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("board_ids[]") == "b2" {
			io.WriteString(w, `[]`)
			return
		}
		if q.Get("page") == "2" {
			io.WriteString(w, `[`+card(89, "Paint the fence", "Now", `,"closed":true,"golden":false,"tags":[]`)+`]`)
			return
		}
		next := srv.URL + r.URL.Path + "?page=2"
		if r.URL.RawQuery != "" {
			next = srv.URL + r.URL.Path + "?" + r.URL.RawQuery + "&page=2"
		}
		w.Header().Set("Link", `<`+next+`>; rel="next"`)
		io.WriteString(w, `[`+card(87, "Fix the gate", "Now", `,"closed":false,"golden":true,"tags":["buy","waiting"]`)+`,`+
			card(88, "Oil the hinge café", "", `,"closed":false,"golden":false,"tags":[]`)+`]`)
	})
	mux.HandleFunc("GET /1/cards/{n}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("n") {
		case "87":
			io.WriteString(w, card(87, "Fix the gate", "Now", `,"closed":false,"golden":true,"tags":["buy","waiting"],`+
				`"description":"  Swap the **hinge**.\n\nThen oil it.  \n","description_html":"<p>Swap</p>","last_active_at":"2026-10-01T10:00:00Z",`+
				`"steps":[{"id":"`+stepA+`","content":"Order the hinge","completed":false},{"id":"03zzz999yyy888xxx777www6vu","content":"Measure the gap","completed":true}],"score":1.50`))
		case "88":
			io.WriteString(w, card(88, "Oil the hinge café", "", `,"closed":false,"golden":false,"tags":[],"description":""`))
		case "89":
			io.WriteString(w, card(89, "Paint the fence", "Now", `,"closed":true,"golden":false,"tags":[],"steps":[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"Not found"}`)
		}
	})
	created := func(loc string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", srv.URL+loc)
			w.WriteHeader(http.StatusCreated)
		}
	}
	mux.HandleFunc("POST /1/boards", created("/1/boards/b9.json"))
	mux.HandleFunc("POST /1/boards/{id}/columns", created("/1/boards/b9/columns/c9.json"))
	mux.HandleFunc("POST /1/boards/{id}/cards", created("/1/cards/90.json"))
	mux.HandleFunc("POST /1/cards/{n}/steps", created("/1/cards/87/steps/03new000000000000000000000.json"))
	mux.HandleFunc("POST /1/cards/{n}/comments", created("/1/cards/87/comments/cm1.json"))
	mux.HandleFunc("POST /1/rails/active_storage/direct_uploads", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"signed_id":"blob-id","attachable_sgid":"att-sgid","direct_upload":{"url":"`+srv.URL+`/storage/up","headers":{"Content-Type":"image/jpeg"}}}`)
	})
	mux.HandleFunc("PUT /storage/up", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /links/ok", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	mux.HandleFunc("GET /links/missing", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type result struct {
	stdout, stderr string
	code           int
}

func runPython(t *testing.T, fz string, env []string, args []string) result {
	t.Helper()
	cmd := exec.Command("python3", append([]string{fz}, args...)...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("python3: %v", err)
	}
	return result{out.String(), errb.String(), code}
}

func runGo(args []string) result {
	var out, errb bytes.Buffer
	code := run(NewRoot(), append([]string{}, args...), &out, &errb)
	return result{out.String(), errb.String(), code}
}

// parityEnv points both implementations at srv with a throwaway home, so
// neither reads the real config nor writes the real action log.
func parityEnv(t *testing.T, srv *httptest.Server) []string {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{
		"FIZZY_URL": srv.URL, "FIZZY_ACCOUNT": "1", "FIZZY_TOKEN": "T", "FIZZY_CONFIG": "/nonexistent",
		"FIZZY_TOKEN_CMD": "", "FIZZY_INBOX": filepath.Join(home, "inbox"), "FIZZY_RULES_CARD": "", "HOME": home,
	}
	var env []string
	for _, kv := range os.Environ() {
		if _, ok := vars[strings.SplitN(kv, "=", 2)[0]]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range vars {
		t.Setenv(k, v)
		env = append(env, k+"="+v)
	}
	return env
}

// oldForm turns a noun-form command line back into the fz 0.1 form, the
// only one the Python fz understands.
func oldForm(args []string) []string {
	i := commandIndex(args)
	if i+1 >= len(args) {
		return args
	}
	head, noun, verb, rest := args[:i:i], args[i], args[i+1], args[i+2:]
	out := func(words ...string) []string { return append(append(head, words...), rest...) }
	switch noun + " " + verb {
	case "board list":
		return out("boards")
	case "board create":
		return out("board-create")
	case "column list":
		return out("columns")
	case "card list":
		return out("cards")
	case "card show", "card create", "card move", "card reopen", "card golden", "card tag", "card delete":
		return out(verb)
	case "step add":
		return out("step")
	case "comment add":
		return out("comment")
	case "step check", "step uncheck":
		return append(append(head, "step", rest[0], "--"+verb, rest[1]), rest[2:]...)
	}
	return args
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParityWithPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	fz, err := filepath.Abs(filepath.Join("..", "..", "bin", "fz"))
	if err != nil {
		t.Fatal(err)
	}
	srv := fakeFizzy(t)
	env := parityEnv(t, srv)

	dir := t.TempDir()
	img := filepath.Join(dir, "hinge.jpg")
	writeFile(t, img, "not really a jpeg")
	drafts := filepath.Join(dir, "drafts")
	if err := os.Mkdir(drafts, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(drafts, "hinge.jpg"), "jpeg bytes")
	good := filepath.Join(drafts, "gate.md")
	writeFile(t, good, "---\nboard: Home\ncolumn: Next\ntags: buy, #Waiting\ngolden: true\n---\n# Fix the gate\n\n"+
		"Swap the **hinge** on the side gate.\n\n![Hinge](hinge.jpg)\n\n- [Guide]("+srv.URL+"/links/ok)\n- "+srv.URL+"/links/missing\n\n"+
		"## Steps\n- [ ] Order the hinge\n- [x] Measure the gap\n")
	done := filepath.Join(drafts, "latch.md")
	writeFile(t, done, "---\nboard: Home\ncard: 87\n---\n# Fix the latch\n\nTwo screws.\n")
	bad := filepath.Join(dir, "bad.md")
	writeFile(t, bad, "# No front matter\n")
	plain := filepath.Join(dir, "plain.md")
	writeFile(t, plain, "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n\n## Steps\n- [ ] Buy oil\n")

	cases := [][]string{
		// reads, text and JSON
		{"boards"}, {"board", "list"}, {"--json", "boards"},
		{"columns", "Home"}, {"column", "list", "Home"}, {"columns", "Garden"}, {"--json", "columns", "Home"}, {"--json", "columns", "b2"},
		{"cards"}, {"cards", "--board", "Home"}, {"card", "list", "--board", "Garden"},
		{"cards", "--state", "golden", "--search", "gate hinge"}, {"cards", "--search", ""},
		{"--json", "cards", "--board", "Home"}, {"--json", "cards", "--board", "Garden"},
		{"show", "87"}, {"show", "88"}, {"show", "89"}, {"card", "show", "87"}, {"--json", "show", "87"}, {"--json", "card", "show", "88"},
		// writes, dry-run
		{"--dry-run", "board-create", "Patio", "--postpone", "30", "--columns", "Later, Next,,Now"},
		{"--dry-run", "board", "create", "Patio"},
		{"--dry-run", "--json", "board-create", "Pätio", "--columns", "Später"},
		{"--dry-run", "create", "Home", "Fix the latch", "--description", "Use **two** screws", "--column", "Next", "--tag", "buy"},
		{"--dry-run", "card", "create", "Home", "Fix the latch"},
		{"--dry-run", "--json", "create", "Home", "Fix the latch"},
		{"--dry-run", "create", "Home", "Fix the latch", "--description-file", plain},
		{"--dry-run", "move", "87", "--to", "Now"}, {"--dry-run", "move", "87", "--to", "maybe"},
		{"--dry-run", "move", "87", "--to", "done"}, {"--dry-run", "move", "87", "--to", "Not Now"},
		{"--dry-run", "card", "move", "87", "--board", "Garden"}, {"--dry-run", "--json", "move", "87", "--board", "Home", "--to", "Next"},
		{"--dry-run", "reopen", "89"}, {"--dry-run", "--json", "reopen", "89"},
		{"--dry-run", "golden", "87"}, {"--dry-run", "golden", "87", "--off"}, {"--dry-run", "--json", "card", "golden", "87", "--off"},
		{"--dry-run", "tag", "87", "#Hinge"}, {"--dry-run", "tag", "87", "buy"}, {"--dry-run", "tag", "87", "buy", "--remove"},
		{"--dry-run", "card", "tag", "87", "hinge", "--remove"}, {"--dry-run", "--json", "tag", "87", "hinge"},
		{"--dry-run", "step", "87", "Order the hinge", "--done"}, {"--dry-run", "--json", "step", "add", "87", "Order the hinge"},
		{"--dry-run", "step", "87", "--check", stepA}, {"--dry-run", "step", "87", "--uncheck=" + stepA},
		{"--dry-run", "--json", "step", "87", "--check=" + stepA},
		{"--dry-run", "step", "check", "87", stepA}, {"--dry-run", "--json", "step", "uncheck", "87", stepA},
		{"--dry-run", "comment", "87", "Measured: **42 mm**"}, {"--dry-run", "--json", "comment", "add", "87", "ok"},
		{"--dry-run", "delete", "87", "--yes"}, {"--dry-run", "--json", "card", "delete", "87", "--yes"},
		{"--dry-run", "upload", img}, {"--dry-run", "--json", "upload", img},
		// writes against the fake server
		{"create", "Home", "Fix the latch", "--column", "Next", "--tag", "hinge"}, {"--json", "create", "Home", "Fix the latch"},
		{"board-create", "Patio", "--postpone", "30", "--columns", "Later,Now"}, {"--json", "board-create", "Patio"},
		{"step", "87", "Order the hinge"}, {"comment", "87", "hi"}, {"move", "87", "--to", "Now"},
		{"tag", "87", "hinge"}, {"delete", "87", "--yes"}, {"upload", img}, {"--json", "upload", img},
		// drafts
		{"draft", "check", drafts}, {"draft", "check", good, "--links"}, {"draft", "check", bad, plain},
		{"draft", "preview", good, plain}, {"draft", "preview", drafts},
		{"--dry-run", "draft", "apply", drafts}, {"--dry-run", "draft", "apply", plain},
		// errors
		{"cards", "--board", "Nope"}, {"--dry-run", "move", "87", "--to", "Nope"}, {"--dry-run", "create", "Nope", "x"},
		{"delete", "87"}, {"--json", "delete", "87"}, {"show", "999"}, {"draft", "apply", bad},
	}
	for _, args := range cases {
		py := runPython(t, fz, env, oldForm(args))
		got := runGo(args)
		if got != py {
			t.Errorf("fz %q\n go: code %d\n%s--- stderr\n%s\n py: code %d\n%s--- stderr\n%s",
				args, got.code, got.stdout, got.stderr, py.code, py.stdout, py.stderr)
		}
	}

	// Usage errors: argparse and cobra word them differently, so only the
	// exit code is compared.
	for _, args := range [][]string{
		{"show", "abc"}, {"show"}, {"cards", "--state", "bogus"}, {"board-create", "X", "--postpone", "5"},
		{"--nope", "boards"}, {"nope"}, {"create", "Home"}, {"draft", "check"},
	} {
		py := runPython(t, fz, env, args)
		got := runGo(args)
		if got.code != py.code {
			t.Errorf("fz %q: go exit %d, python exit %d (go stderr %q)", args, got.code, py.code, got.stderr)
		}
	}
}

// step list, step edit and step delete are new, so they have no Python
// counterpart to compare with.
func TestNewStepCommands(t *testing.T) {
	srv := fakeFizzy(t)
	parityEnv(t, srv)
	cases := []struct {
		args []string
		want result
	}{
		{[]string{"step", "list", "87"}, result{"[ ] " + stepA + "  Order the hinge\n[x] 03zzz999yyy888xxx777www6vu  Measure the gap\n", "", 0}},
		{[]string{"step", "list", "88"}, result{"(no steps)\n", "", 0}},
		{[]string{"--json", "step", "list", "88"}, result{"[]\n", "", 0}},
		{[]string{"--json", "step", "list", "87"}, result{"[\n  {\n    \"id\": \"" + stepA + "\",\n    \"content\": \"Order the hinge\",\n    \"completed\": false\n  },\n" +
			"  {\n    \"id\": \"03zzz999yyy888xxx777www6vu\",\n    \"content\": \"Measure the gap\",\n    \"completed\": true\n  }\n]\n", "", 0}},
		{[]string{"--dry-run", "step", "edit", "87", stepA, "Order two hinges"}, result{"step updated\n",
			"[dry-run] PUT " + srv.URL + "/1/cards/87/steps/" + stepA + ` {"step": {"content": "Order two hinges"}}` + "\n", 0}},
		{[]string{"--dry-run", "--json", "step", "edit", "87", stepA, "x"}, result{"{\n  \"id\": \"" + stepA + "\"\n}\n",
			"[dry-run] PUT " + srv.URL + "/1/cards/87/steps/" + stepA + ` {"step": {"content": "x"}}` + "\n", 0}},
		{[]string{"--dry-run", "step", "delete", "87", stepA}, result{"", "refusing without --yes\n", 2}},
		{[]string{"--dry-run", "step", "delete", "87", stepA, "--yes"}, result{"step deleted\n",
			"[dry-run] DELETE " + srv.URL + "/1/cards/87/steps/" + stepA + " \n", 0}},
		{[]string{"--dry-run", "--json", "step", "delete", "87", stepA, "--yes"}, result{"{\n  \"deleted\": \"" + stepA + "\"\n}\n",
			"[dry-run] DELETE " + srv.URL + "/1/cards/87/steps/" + stepA + " \n", 0}},
		{[]string{"--dry-run", "step", "check", "87", stepA}, result{"step updated\n",
			"[dry-run] PUT " + srv.URL + "/1/cards/87/steps/" + stepA + ` {"step": {"completed": true}}` + "\n", 0}},
	}
	for _, tc := range cases {
		if got := runGo(tc.args); got != tc.want {
			t.Errorf("fz %q\n got  %+v\n want %+v", tc.args, got, tc.want)
		}
	}
}

func TestDraftPreviewNeedsNoConfig(t *testing.T) {
	t.Setenv("FIZZY_URL", "")
	t.Setenv("FIZZY_ACCOUNT", "")
	t.Setenv("FIZZY_CONFIG", "/nonexistent")
	d := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, d, "---\nboard: Home\n---\n# Fix the gate\n\nSwap the hinge.\n")
	got := runGo([]string{"draft", "preview", d})
	want := "=== Fix the gate\nboard: Home   column: Maybe?   tags: -   golden: False\n<p>Swap the hinge.</p>\n\n"
	if got.code != 0 || got.stdout != want {
		t.Errorf("got %+v, want stdout %q", got, want)
	}
	if got := runGo([]string{"boards"}); got.code != 1 || !strings.HasPrefix(got.stderr, "fz: set FIZZY_URL and FIZZY_ACCOUNT") {
		t.Errorf("boards without config: %+v", got)
	}
}

func TestUnknownSubcommandIsUsageError(t *testing.T) {
	got := runGo([]string{"card", "nope"})
	if got.code != 2 || !strings.Contains(got.stderr, `unknown command "nope" for "fz card"`) {
		t.Errorf("got %+v", got)
	}
}
