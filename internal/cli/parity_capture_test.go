package cli

// This file captured testdata/parity/python-fz.json from the Python fz on
// 2026-10-17 and goes away with the Python code. Run it with
//
//	FZ_CAPTURE_PYTHON=1 go test -run TestCapturePythonParity ./internal/cli/

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

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

func TestCapturePythonParity(t *testing.T) {
	if os.Getenv("FZ_CAPTURE_PYTHON") != "1" {
		t.Skip("set FZ_CAPTURE_PYTHON=1 to capture the Python fz output")
	}
	fz, err := filepath.Abs(filepath.Join("..", "..", "bin", "fz"))
	if err != nil {
		t.Fatal(err)
	}
	f := newParityFixture(t)
	env := f.env
	img, drafts, good, bad, plain := f.img, f.drafts, f.good, f.bad, f.plain
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
	usage := [][]string{
		{"show", "abc"}, {"show"}, {"cards", "--state", "bogus"}, {"board-create", "X", "--postpone", "5"},
		{"--nope", "boards"}, {"nope"}, {"create", "Home"}, {"draft", "check"},
	}
	record := func(args, pyArgs []string) parityCase {
		py := runPython(t, fz, env, pyArgs)
		c := parityCase{Args: f.markAll(args), Stdout: f.mark(py.stdout), Stderr: f.mark(py.stderr), Code: py.code}
		if !slices.Equal(args, pyArgs) {
			c.PythonArgs = f.markAll(pyArgs)
		}
		return c
	}
	p := parityFile{Note: "Captured from the Python fz (bin/fz) on 2026-10-17. Do not regenerate from Go."}
	for _, args := range cases {
		p.Cases = append(p.Cases, record(args, oldForm(args)))
	}
	for _, args := range usage {
		p.UsageCases = append(p.UsageCases, record(args, args))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pythonGolden), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonGolden, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
