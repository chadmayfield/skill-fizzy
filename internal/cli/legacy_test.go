package cli

import (
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	cases := []struct{ old, new []string }{
		{[]string{"boards"}, []string{"board", "list"}},
		{[]string{"board-create", "Garden", "--postpone", "365"}, []string{"board", "create", "Garden", "--postpone", "365"}},
		{[]string{"columns", "Home"}, []string{"column", "list", "Home"}},
		{[]string{"cards", "--board", "Home"}, []string{"card", "list", "--board", "Home"}},
		{[]string{"show", "87"}, []string{"card", "show", "87"}},
		{[]string{"create", "Home", "Fix gate"}, []string{"card", "create", "Home", "Fix gate"}},
		{[]string{"move", "5", "--to", "now"}, []string{"card", "move", "5", "--to", "now"}},
		{[]string{"reopen", "5"}, []string{"card", "reopen", "5"}},
		{[]string{"golden", "5", "--off"}, []string{"card", "golden", "5", "--off"}},
		{[]string{"tag", "5", "buy"}, []string{"card", "tag", "5", "buy"}},
		{[]string{"tag", "87", "list"}, []string{"card", "tag", "87", "list"}},
		{[]string{"tag", "--remove", "87", "buy"}, []string{"card", "tag", "87", "buy", "--remove"}},
		{[]string{"tag", "87", "--remove", "buy"}, []string{"card", "tag", "87", "--remove", "buy"}},
		{[]string{"tag", "87", "buy", "--config", "f"}, []string{"card", "tag", "87", "buy", "--config", "f"}},
		{[]string{"tag", "--config", "f", "87", "buy"}, []string{"card", "tag", "87", "buy", "--config", "f"}},
		{[]string{"tag", "--config=f", "--remove", "87", "buy"}, []string{"card", "tag", "87", "buy", "--config=f", "--remove"}},
		{[]string{"tag", "--config", "f", "list"}, []string{"tag", "--config", "f", "list"}},
		{[]string{"tag", "--json", "list"}, []string{"tag", "--json", "list"}},
		{[]string{"tag", "list"}, []string{"tag", "list"}},
		{[]string{"tag", "87", "buy", "--remove"}, []string{"card", "tag", "87", "buy", "--remove"}},
		{[]string{"--dry-run", "tag", "--remove", "87", "buy"}, []string{"--dry-run", "card", "tag", "87", "buy", "--remove"}},
		{[]string{"delete", "5", "--yes"}, []string{"card", "delete", "5", "--yes"}},
		{[]string{"comment", "5", "hi"}, []string{"comment", "add", "5", "hi"}},
		{[]string{"step", "5", "Order it", "--done"}, []string{"step", "add", "5", "Order it", "--done"}},
		{[]string{"step", "5", "--check", "S1"}, []string{"step", "check", "5", "S1"}},
		{[]string{"step", "5", "--uncheck", "S1"}, []string{"step", "uncheck", "5", "S1"}},
		{[]string{"step", "5", "--check=S1"}, []string{"step", "check", "5", "S1"}},
		{[]string{"--json=true", "cards"}, []string{"--json=true", "card", "list"}},
		{[]string{"--json=false", "--dry-run=true", "show", "87"}, []string{"--json=false", "--dry-run=true", "card", "show", "87"}},
		{[]string{"--dry-run=false", "tag", "87", "buy"}, []string{"--dry-run=false", "card", "tag", "87", "buy"}},
	}
	for _, tc := range cases {
		if got := Rewrite(append([]string{}, tc.old...)); !reflect.DeepEqual(got, tc.new) {
			t.Errorf("Rewrite(%q) = %q, want %q", tc.old, got, tc.new)
		}
	}
}

func TestRewritePassesNewFormsThrough(t *testing.T) {
	for _, argv := range [][]string{
		{"card", "edit", "5", "--title", "x"},
		{"step", "list", "5"},
		{"tag", "list"},
		{"tag", "--json", "list"},
		{"--json", "tag", "list"},
		{"tag", "--remove", "buy", "87"},
		{"comment", "add", "5", "x"},
		{"review"},
		{"draft", "apply", "d/"},
		{"help", "card"},
		{},
	} {
		if got := Rewrite(append([]string{}, argv...)); !reflect.DeepEqual(got, argv) && !(len(got) == 0 && len(argv) == 0) {
			t.Errorf("Rewrite(%q) = %q, want it unchanged", argv, got)
		}
	}
}

func TestRewriteWithGlobalFlags(t *testing.T) {
	cases := []struct{ old, new []string }{
		{[]string{"--dry-run", "move", "5", "--to", "now"}, []string{"--dry-run", "card", "move", "5", "--to", "now"}},
		{[]string{"--config", "x.json", "step", "5", "--check=S1"}, []string{"--config", "x.json", "step", "check", "5", "S1"}},
		{[]string{"--json", "--config=x.json", "show", "5"}, []string{"--json", "--config=x.json", "card", "show", "5"}},
	}
	for _, tc := range cases {
		if got := Rewrite(append([]string{}, tc.old...)); !reflect.DeepEqual(got, tc.new) {
			t.Errorf("Rewrite(%q) = %q, want %q", tc.old, got, tc.new)
		}
	}
}

// TestOldAndNewReachTheSameCommand checks that each 0.1 form and its new
// form resolve to the same cobra command.
func TestOldAndNewReachTheSameCommand(t *testing.T) {
	pairs := [][2][]string{
		{{"move", "5", "--to", "now"}, {"card", "move", "5", "--to", "now"}},
		{{"step", "5", "x"}, {"step", "add", "5", "x"}},
		{{"comment", "5", "x"}, {"comment", "add", "5", "x"}},
		{{"step", "5", "--check", "S1"}, {"step", "check", "5", "S1"}},
	}
	for _, p := range pairs {
		root := NewRoot()
		oldCmd, _, err := root.Find(Rewrite(p[0]))
		if err != nil {
			t.Fatalf("Find(%q): %v", p[0], err)
		}
		newCmd, _, err := root.Find(p[1])
		if err != nil {
			t.Fatalf("Find(%q): %v", p[1], err)
		}
		if !oldCmd.Runnable() || oldCmd != newCmd {
			t.Errorf("%q reached %q, %q reached %q", p[0], oldCmd.CommandPath(), p[1], newCmd.CommandPath())
		}
	}
}

func TestStepCheckArgs(t *testing.T) {
	root := NewRoot()
	cmd, rest, err := root.Find(Rewrite([]string{"step", "5", "--check", "S1"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cmd.Flags().Args(), " "); got != "5 S1" {
		t.Errorf("args = %q, want %q", got, "5 S1")
	}
}

func TestGlobalFlagsParse(t *testing.T) {
	root := NewRoot()
	cmd, rest, err := root.Find([]string{"--dry-run", "--json", "card", "show", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dry-run", "json"} {
		if v, err := cmd.Flags().GetBool(name); err != nil || !v {
			t.Errorf("--%s = %v (%v), want true", name, v, err)
		}
	}
	if cmd.CommandPath() != "fz card show" {
		t.Errorf("command = %q, want fz card show", cmd.CommandPath())
	}
}

// The old flag-first tag form reaches card tag end to end.
func TestOldTagRemoveFlagFirst(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"tags":["buy"]}`)
	r := runGo([]string{"tag", "--remove", "87", "buy"})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].path != "/1/cards/87/taggings" {
		t.Errorf("writes %+v", w)
	}
}

// A tag named "list" still works in the old form, because the word after
// tag is a card number.
func TestOldTagNamedList(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"tags":[]}`)
	r := runGo([]string{"tag", "87", "list"})
	if r.code != 0 {
		t.Fatalf("got %+v", r)
	}
	if w := extraWrites(calls()); len(w) != 1 || w[0].path != "/1/cards/87/taggings" {
		t.Errorf("writes %+v", w)
	}
}

// A value flag after the old tag line keeps its value.
func TestOldTagWithConfigFlag(t *testing.T) {
	_, calls := extraServer(t, `{"number":87,"tags":[]}`)
	cfg := filepath.Join(t.TempDir(), "none.json")
	for _, args := range [][]string{
		{"tag", "87", "buy", "--config", cfg},
		{"tag", "--config", cfg, "87", "buy"},
	} {
		r := runGo(args)
		if r.code != 0 {
			t.Fatalf("%q: got %+v", args, r)
		}
	}
	if w := extraWrites(calls()); len(w) != 2 {
		t.Errorf("writes %+v", w)
	}
}

// Words after -- are text, so the 0.1 rewrite never reads a --check
// there as a tick.
func TestOldStepIgnoresWordsAfterDoubleDash(t *testing.T) {
	srv, seen := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/1/cards/87/steps/03abc123def456ghi789jkl0mn.json")
		w.WriteHeader(http.StatusCreated)
	})
	parityEnv(t, srv)
	for _, tc := range []struct {
		argv []string
		code int
		sent string // the step text sent, or "" for nothing
	}{
		{[]string{"step", "87", "--", "--check", "S1"}, 2, ""},
		{[]string{"step", "87", "--", "--uncheck", "S1"}, 2, ""},
		{[]string{"step", "87", "--", "--check=S1"}, 0, "--check=S1"},
		{[]string{"step", "87", "--", "--check"}, 0, "--check"},
	} {
		got := Rewrite(append([]string{}, tc.argv...))
		want := append([]string{"step", "add"}, tc.argv[1:]...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Rewrite(%q) = %q, want %q", tc.argv, got, want)
		}
		before := len(seen())
		r := runGo(tc.argv)
		calls := seen()[before:]
		if r.code != tc.code {
			t.Errorf("fz %q: %+v, want exit %d", tc.argv, r, tc.code)
		}
		switch {
		case tc.sent == "" && len(calls) != 0:
			t.Errorf("fz %q sent %+v", tc.argv, calls)
		case tc.sent != "" && (len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/1/cards/87/steps" ||
			!strings.Contains(calls[0].body, `"content":"`+tc.sent+`"`)):
			t.Errorf("fz %q sent %+v, want one step add of %q", tc.argv, calls, tc.sent)
		}
	}
}
