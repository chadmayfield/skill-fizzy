package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
			col = fmt.Sprintf(`{"id":"c-%s","name":%q,"color":{"name":"Blue","value":"var(--color-card-default)"}}`, strings.ToLower(column), column)
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
		io.WriteString(w, `[{"id":"c-later","name":"Later","color":{"name":"Blue","value":"var(--color-card-default)"}},{"id":"c-next","name":"Next","color":{"name":"Blue","value":"var(--color-card-1)"}},{"id":"c-now","name":"Now","color":{"name":"Blue","value":"var(--color-card-2)"}}]`)
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

func runGo(args []string) result {
	var out, errb bytes.Buffer
	code := run(NewRoot(), append([]string{}, args...), &out, &errb)
	return result{out.String(), errb.String(), code}
}

// parityEnv points fz at srv with a throwaway home, so it neither reads the
// real config nor writes the real action log.
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

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pythonGolden holds what the Python fz printed for each parity case. It was
// captured once from the Python fz (bin/fz, Python 3.9) on 2026-10-17, while
// that code still existed, and is the reference the Go fz must match. Nothing
// regenerates it: the Python fz is gone, and writing this file from Go output
// would turn the test into Go checking itself. Fix Go, not the file.
const pythonGolden = "testdata/parity/python-fz.json"

// URL, DIR and HOME placeholders stand for the per-run fake server URL, the
// fixture directory and the throwaway home in the golden file.
const (
	urlMark  = "{{URL}}"
	dirMark  = "{{DIR}}"
	homeMark = "{{HOME}}"
)

type parityCase struct {
	// Args is the command line Go runs. PythonArgs is the fz 0.1 form the
	// Python fz was given, when it differs.
	Args       []string `json:"args"`
	PythonArgs []string `json:"python_args,omitempty"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Code       int      `json:"code"`
}

type parityFile struct {
	Note  string       `json:"note"`
	Cases []parityCase `json:"cases"`
	// UsageCases are usage errors. argparse and cobra word them
	// differently, so only the exit code is compared.
	UsageCases []parityCase `json:"usage_cases"`
}

// parityFixture is the fake server, the environment and the files the
// parity cases read.
type parityFixture struct {
	srv                           *httptest.Server
	env                           []string
	home, dir                     string
	img, drafts, good, bad, plain string
}

func newParityFixture(t *testing.T) parityFixture {
	t.Helper()
	srv := fakeFizzy(t)
	env := parityEnv(t, srv)
	f := parityFixture{srv: srv, env: env, home: os.Getenv("HOME"), dir: t.TempDir()}
	f.img = filepath.Join(f.dir, "hinge.jpg")
	writeFile(t, f.img, "not really a jpeg")
	f.drafts = filepath.Join(f.dir, "drafts")
	if err := os.Mkdir(f.drafts, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.drafts, "hinge.jpg"), "jpeg bytes")
	f.good = filepath.Join(f.drafts, "gate.md")
	writeFile(t, f.good, "---\nboard: Home\ncolumn: Next\ntags: buy, #Waiting\ngolden: true\n---\n# Fix the gate\n\n"+
		"Swap the **hinge** on the side gate.\n\n![Hinge](hinge.jpg)\n\n- [Guide]("+srv.URL+"/links/ok)\n- "+srv.URL+"/links/missing\n\n"+
		"## Steps\n- [ ] Order the hinge\n- [x] Measure the gap\n")
	writeFile(t, filepath.Join(f.drafts, "latch.md"), "---\nboard: Home\ncard: 87\n---\n# Fix the latch\n\nTwo screws.\n")
	f.bad = filepath.Join(f.dir, "bad.md")
	writeFile(t, f.bad, "# No front matter\n")
	f.plain = filepath.Join(f.dir, "plain.md")
	writeFile(t, f.plain, "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n\n## Steps\n- [ ] Buy oil\n")
	return f
}

// mark replaces this run's server URL and directories with placeholders.
func (f parityFixture) mark(s string) string {
	return strings.NewReplacer(f.srv.URL, urlMark, f.dir, dirMark, f.home, homeMark).Replace(s)
}

// unmark turns placeholders back into this run's server URL and directories.
func (f parityFixture) unmark(s string) string {
	return strings.NewReplacer(urlMark, f.srv.URL, dirMark, f.dir, homeMark, f.home).Replace(s)
}

func (f parityFixture) markAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = f.mark(a)
	}
	return out
}

func (f parityFixture) unmarkAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = f.unmark(a)
	}
	return out
}

func readParityFile(t *testing.T) parityFile {
	t.Helper()
	raw, err := os.ReadFile(pythonGolden)
	if err != nil {
		t.Fatal(err)
	}
	var p parityFile
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) == 0 || len(p.UsageCases) == 0 {
		t.Fatalf("%s holds no cases", pythonGolden)
	}
	return p
}

// TestParityWithPython runs every case in the golden file through the Go fz
// and compares stdout, stderr and the exit code with what the Python fz
// printed.
func TestParityWithPython(t *testing.T) {
	f := newParityFixture(t)
	p := readParityFile(t)
	for _, c := range p.Cases {
		args := f.unmarkAll(c.Args)
		want := result{f.unmark(c.Stdout), f.unmark(c.Stderr), c.Code}
		// Go's skip line names --update, which Python's fz does not have.
		// That one line is the only intended difference in draft apply.
		want.stdout = strings.ReplaceAll(want.stdout, "skip latch.md: already card #87\n",
			"skip latch.md: already card #87 (use --update to push changes)\n")
		got := runGo(args)
		if got != want {
			t.Errorf("fz %q\n go: code %d\n%s--- stderr\n%s\n py: code %d\n%s--- stderr\n%s",
				c.Args, got.code, got.stdout, got.stderr, want.code, want.stdout, want.stderr)
		}
	}
	for _, c := range p.UsageCases {
		got := runGo(f.unmarkAll(c.Args))
		if got.code != c.Code {
			t.Errorf("fz %q: go exit %d, python exit %d (go stderr %q)", c.Args, got.code, c.Code, got.stderr)
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
