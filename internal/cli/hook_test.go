package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pythonHookNote is the closing line the example Python hook prints. The Go
// command prints its own line instead, so parity compares everything above it.
const pythonHookNote = "Follow the fizzy-cards skill's rules for moving cards and ticking steps."

const hookBoards = `[{"id":"b-home","name":"Home"},{"id":"b-garden","name":"Garden"}]`

// Server order on purpose: Next before Now, so the hook has to sort.
var hookCards = map[string]string{
	"Home": `[
 {"number":87,"title":"Fix the gate","column":{"id":"c1","name":"Next"},"golden":false},
 {"number":88,"title":"Oil the hinge","column":{"id":"c2","name":"Now"},"golden":true},
 {"number":89,"title":"Buy hinge pins","column":null,"golden":false},
 {"number":90,"title":"Paint the gate","column":{"id":"c2","name":"Now"},"golden":false},
 {"number":91,"title":"Measure the gap","column":{"id":"c3","name":"Waiting"},"golden":false}
]`,
	"Garden": `[
 {"number":92,"title":"Hang the garden gate","column":null,"golden":false},
 {"number":93,"title":"Find a spare hinge","column":{"id":"c4","name":"Waiting"},"golden":true}
]`,
}

const hookMap = `{
 "gatehouse": {"board": "Home"},
 "shed": {"board": "Garden"},
 "latch": {"board": "Home", "card": 89},
 "hinge": {"board": "Home", "card": 88},
 "porch": {"board": "Home", "card": 120},
 "stoop": {"board": "Home", "card": "88"},
 "deck": {"board": "Home", "card": 0}
}`

// hookServer answers the two calls the hook makes and counts every request.
func hookServer(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	ids := map[string]string{"b-home": "Home", "b-garden": "Garden"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/1/boards":
			io.WriteString(w, hookBoards)
		case r.URL.Path == "/1/cards" && len(r.URL.Query()) == 1 && ids[r.URL.Query().Get("board_ids[]")] != "":
			io.WriteString(w, hookCards[ids[r.URL.Query().Get("board_ids[]")]])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// hookFixture is a folder tree for the hook to run in, a repos map, and a
// fake fz for the Python script.
type hookFixture struct {
	root, mapPath, fakeFz, home, script string
	srv                                 *httptest.Server
	hits                                *int64
}

func newHookFixture(t *testing.T) hookFixture {
	t.Helper()
	srv, hits := hookServer(t)
	f := hookFixture{root: t.TempDir(), srv: srv, hits: hits}
	// Resolve the script before any test changes directory.
	script, err := filepath.Abs("../../examples/hooks/fizzy-session-hook.py")
	if err != nil {
		t.Fatal(err)
	}
	f.script = script
	f.home = parityEnv(t, srv)
	for _, d := range []string{"gatehouse", "shed/latch", "hinge", "porch", "attic", "stoop", "deck"} {
		if err := os.MkdirAll(filepath.Join(f.root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.mapPath = filepath.Join(f.root, "repos.json")
	writeFile(t, f.mapPath, hookMap)
	t.Setenv("FIZZY_REPOS_MAP", f.mapPath)

	// The fake fz checks it was called the way the script calls fz, then
	// prints the board's cards.
	bin := t.TempDir()
	for board, cards := range hookCards {
		writeFile(t, filepath.Join(bin, "cards-"+board+".json"), cards)
	}
	f.fakeFz = filepath.Join(bin, "fz")
	writeFile(t, f.fakeFz, "#!/bin/sh\n"+
		`[ "$1" = --json ] && [ "$2" = cards ] && [ "$3" = --board ] || exit 3`+"\n"+
		`cat "$(dirname "$0")/cards-$4.json"`+"\n")
	if err := os.Chmod(f.fakeFz, 0o755); err != nil {
		t.Fatal(err)
	}

	// gatehouse is a git repo, and its worktree sits under shed, so the
	// path alone would pick the Garden board.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	repo := filepath.Join(f.root, "gatehouse")
	git("init", "-q", repo)
	git("-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	git("-C", repo, "worktree", "add", "-q", filepath.Join(f.root, "shed", "feature-x"))
	return f
}

// runHook runs the Go command with stdin.
func runHook(stdin string, extra ...string) result {
	root := NewRoot()
	root.SetIn(strings.NewReader(stdin))
	var out, errb bytes.Buffer
	code := run(root, append([]string{"hook", "session-start"}, extra...), &out, &errb)
	return result{out.String(), errb.String(), code}
}

// runPythonHook runs the example script the way Claude Code would, in dir.
func runPythonHook(t *testing.T, f hookFixture, dir, stdin string) result {
	t.Helper()
	cmd := exec.Command("python3", "-I", f.script)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + f.home, "FIZZY_REPOS_MAP=" + os.Getenv("FIZZY_REPOS_MAP"), "FIZZY_FZ=" + f.fakeFz}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	code := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("python3: %v", err)
		}
		code = ee.ExitCode()
	}
	return result{out.String(), errb.String(), code}
}

// splitNote returns the output without its closing line, and that line.
func splitNote(s string) (string, string) {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return "", ""
	}
	i := strings.LastIndex(s, "\n")
	return s[:i+1], s[i+1:]
}

func TestHookCommandRegistered(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"hook", "session-start"})
	if err != nil || cmd.Name() != "session-start" || cmd.Parent().Name() != "hook" {
		t.Fatalf("hook session-start not found: %v", err)
	}
	if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" || cmd.Parent().Short == "" {
		t.Error("hook session-start needs Short, Long and Example, and hook a Short")
	}
	if !strings.Contains(cmd.Long, `"command": "fz hook session-start"`) {
		t.Error("Long should show the settings.json entry")
	}
}

func TestHookParity(t *testing.T) {
	f := newHookFixture(t)
	_, pyErr := exec.LookPath("python3")
	if pyErr != nil {
		t.Log("python3 is not on PATH: checking the Go command only, not parity with the Python script")
	}
	stdin := func(dir string) string { return `{"session_id":"s1","cwd":"` + filepath.Join(f.root, dir) + `"}` }
	nowNext := "Fizzy: this repo maps to the Home board.\nOpen cards in Now and Next:\n" +
		"  #88 [Now] Oil the hinge ★\n  #90 [Now] Paint the gate\n  #87 [Next] Fix the gate\n"
	nothing := "Fizzy: this repo maps to the Garden board.\nNothing in Now or Next.\n"
	cases := []struct {
		name, stdin, dir, want string
		missingMap             bool
	}{
		{name: "board with Now and Next", stdin: stdin("gatehouse"), want: nowNext},
		{name: "nothing in Now or Next", stdin: stdin("shed"), want: nothing},
		{name: "golden card entry", stdin: stdin("hinge"), want: "Fizzy: this repo's card is on the Home board:\n  #88 [Now] Oil the hinge ★\n"},
		{name: "card in Maybe? and the deepest name wins", stdin: stdin("shed/latch"), want: "Fizzy: this repo's card is on the Home board:\n  #89 [Maybe?] Buy hinge pins\n"},
		{name: "card not open", stdin: stdin("porch"), want: "Fizzy: this repo's card is on the Home board:\n  #120 (not open)\n"},
		{name: "card as a string never matches", stdin: stdin("stoop"), want: "Fizzy: this repo's card is on the Home board:\n  #88 (not open)\n"},
		{name: "card 0 means the whole board", stdin: stdin("deck"), want: nowNext},
		{name: "unmapped folder", stdin: stdin("attic"), want: ""},
		{name: "git worktree resolves to its repo", stdin: stdin("shed/feature-x"), want: nowNext},
		{name: "stdin not JSON uses the current folder", stdin: "not json", dir: "shed", want: nothing},
		{name: "empty stdin uses the current folder", stdin: "", dir: "shed", want: nothing},
		{name: "no cwd in stdin uses the current folder", stdin: `{"session_id":"s1"}`, dir: "shed", want: nothing},
		{name: "map file missing", stdin: stdin("gatehouse"), missingMap: true, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.missingMap {
				t.Setenv("FIZZY_REPOS_MAP", filepath.Join(f.root, "nope.json"))
			}
			dir := f.root
			if tc.dir != "" {
				dir = filepath.Join(f.root, tc.dir)
			}
			t.Chdir(dir)

			g := runHook(tc.stdin)
			if g.code != 0 || g.stderr != "" {
				t.Fatalf("go: %+v", g)
			}
			gBody, gNote := splitNote(g.stdout)
			if gBody != tc.want {
				t.Errorf("go stdout:\n%s\nwant:\n%s", g.stdout, tc.want)
			}
			if tc.want != "" && gNote != defaultHookNote {
				t.Errorf("go closing line %q, want %q", gNote, defaultHookNote)
			}
			if pyErr != nil {
				return
			}
			p := runPythonHook(t, f, dir, tc.stdin)
			if p.code != 0 || p.stderr != "" {
				t.Fatalf("python: %+v", p)
			}
			pBody, pNote := splitNote(p.stdout)
			if pBody != gBody {
				t.Errorf("python and go differ:\npython:\n%s\ngo:\n%s", p.stdout, g.stdout)
			}
			if tc.want != "" && pNote != pythonHookNote {
				t.Errorf("python closing line %q", pNote)
			}
		})
	}
}

func TestHookNote(t *testing.T) {
	f := newHookFixture(t)
	in := `{"cwd":"` + filepath.Join(f.root, "porch") + `"}`
	want := "Fizzy: this repo's card is on the Home board:\n  #120 (not open)\n"
	if r := runHook(in, "--note", "Ask before moving a card."); r.code != 0 || r.stdout != want+"Ask before moving a card.\n" {
		t.Errorf("--note: %+v", r)
	}
	if r := runHook(in, "--note", ""); r.code != 0 || r.stdout != want {
		t.Errorf(`--note "": %+v`, r)
	}
}

func TestHookUnmappedSendsNoRequest(t *testing.T) {
	f := newHookFixture(t)
	// A config that would fail to load proves nothing alone, so also count
	// requests: an unmapped folder must make none.
	r := runHook(`{"cwd":"` + filepath.Join(f.root, "attic") + `"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	if n := atomic.LoadInt64(f.hits); n != 0 {
		t.Errorf("unmapped folder sent %d requests", n)
	}
}

func TestHookFailuresPrintNothing(t *testing.T) {
	cases := map[string]func(t *testing.T, f hookFixture){
		"no config": func(t *testing.T, f hookFixture) { t.Setenv("FIZZY_URL", "") },
		"network error": func(t *testing.T, f hookFixture) {
			dead := httptest.NewServer(http.NotFoundHandler())
			dead.Close()
			t.Setenv("FIZZY_URL", dead.URL)
		},
		"server error": func(t *testing.T, f hookFixture) {
			bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			}))
			t.Cleanup(bad.Close)
			t.Setenv("FIZZY_URL", bad.URL)
		},
		"bad map JSON":   func(t *testing.T, f hookFixture) { writeFile(t, f.mapPath, "{not json") },
		"entry no board": func(t *testing.T, f hookFixture) { writeFile(t, f.mapPath, `{"gatehouse": {"card": 87}}`) },
		"unknown board":  func(t *testing.T, f hookFixture) { writeFile(t, f.mapPath, `{"gatehouse": {"board": "Attic"}}`) },
		"empty entry":    func(t *testing.T, f hookFixture) { writeFile(t, f.mapPath, `{"gatehouse": {}}`) },
		"map not object": func(t *testing.T, f hookFixture) { writeFile(t, f.mapPath, `["gatehouse"]`) },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newHookFixture(t)
			setup(t, f)
			r := runHook(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`)
			if r != (result{"", "", 0}) {
				t.Errorf("got %+v", r)
			}
		})
	}
}

func TestHookTimeout(t *testing.T) {
	f := newHookFixture(t)
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { <-release }))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(release) })
	t.Setenv("FIZZY_URL", slow.URL)
	old := hookTimeout
	hookTimeout = 200 * time.Millisecond
	t.Cleanup(func() { hookTimeout = old })

	start := time.Now()
	r := runHook(`{"cwd":"` + filepath.Join(f.root, "gatehouse") + `"}`)
	if r != (result{"", "", 0}) {
		t.Errorf("got %+v", r)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v, want about %v", d, hookTimeout)
	}
}
