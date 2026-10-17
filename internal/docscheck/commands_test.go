package docscheck

import (
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/cli"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/spf13/cobra"
)

// commandDocs are the docs whose fz lines must parse.
var commandDocs = []string{"README.md", "docs/USERGUIDE.md", "AGENTS.md"}

// fzLines returns every line inside a fenced code block that starts with
// "fz ", with any trailing shell comment removed.
func fzLines(text string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence && strings.HasPrefix(trimmed, "fz ") {
			out = append(out, stripComment(trimmed))
		}
	}
	return out
}

// stripComment drops a " # ..." shell comment that sits outside quotes.
func stripComment(line string) string {
	var quote rune
	prev := ' '
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#' && prev == ' ':
			return strings.TrimSpace(line[:i])
		}
		prev = r
	}
	return line
}

// newRoot returns the fz root with cobra's help and completion commands and
// version flag added, as Execute adds them at run time.
func newRoot() *cobra.Command {
	root := cli.NewRoot()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	root.InitDefaultVersionFlag()
	return root
}

// checkLine resolves one fz line through the command tree the way the help
// examples are checked, and returns what is wrong with it, or "".
func checkLine(line string) string {
	words, err := config.SplitWords(line)
	if err != nil {
		return err.Error()
	}
	cmd, rest, err := newRoot().Find(cli.Rewrite(words[1:]))
	if err != nil {
		return err.Error()
	}
	cmd.InitDefaultHelpFlag()
	if err := cmd.ParseFlags(rest); err != nil {
		return "flags: " + err.Error()
	}
	positional := cmd.Flags().Args()
	if !cmd.Runnable() && len(positional) > 0 {
		return "unknown command " + positional[0] + " for " + cmd.CommandPath()
	}
	// A --help line prints help, so cobra skips the argument check.
	if help, _ := cmd.Flags().GetBool("help"); help {
		return ""
	}
	if err := cmd.ValidateArgs(positional); err != nil {
		return "args: " + err.Error()
	}
	return ""
}

func TestDocCommandsParse(t *testing.T) {
	for _, rel := range commandDocs {
		lines := fzLines(read(t, rel))
		// AGENTS.md may have none; the user docs must have some, or the
		// extractor is broken.
		if len(lines) == 0 && rel != "AGENTS.md" {
			t.Errorf("%s: no fz lines found in code blocks", rel)
		}
		for _, line := range lines {
			if msg := checkLine(line); msg != "" {
				t.Errorf("%s: %q: %s", rel, line, msg)
			}
		}
	}
}

// The checker must catch the mistakes it exists for.
func TestCheckLineRejects(t *testing.T) {
	for _, line := range []string{
		"fz card lst",
		"fz card show",
		"fz card show 87 --nope",
		"fz step add 87",
		"fz nope",
	} {
		if checkLine(line) == "" {
			t.Errorf("%q: passed, want an error", line)
		}
	}
	for _, line := range []string{
		"fz --version",
		"fz help card",
		"fz completion zsh",
		"fz card edit --help",
		`fz step 87 "Order the hinge"`,
		"fz card show 87 # a comment",
	} {
		if msg := checkLine(stripComment(line)); msg != "" {
			t.Errorf("%q: %s", line, msg)
		}
	}
}

func TestStripComment(t *testing.T) {
	for in, want := range map[string]string{
		"fz board list   # boards":     "fz board list",
		`fz comment add 87 "a # b"`:    `fz comment add 87 "a # b"`,
		"fz card list --search x#y":    "fz card list --search x#y",
		`fz step add 87 'it # is' # x`: `fz step add 87 'it # is'`,
	} {
		if got := stripComment(in); got != want {
			t.Errorf("stripComment(%q) = %q, want %q", in, got, want)
		}
	}
}
