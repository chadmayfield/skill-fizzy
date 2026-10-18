package docscheck

import (
	"regexp"
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/cli"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/spf13/cobra"
)

// commandDocs are the docs whose fz lines must parse, both in fenced code
// blocks and in inline backticks.
var commandDocs = []string{
	"README.md", "docs/USERGUIDE.md", "AGENTS.md", "CONTRIBUTING.md",
	"SECURITY.md", "skill/SKILL.md", "examples/hooks/README.md",
}

// fencedDocs must have fz lines in code blocks, or the extractor is broken.
var fencedDocs = map[string]bool{"README.md": true, "docs/USERGUIDE.md": true}

// inlineSpan matches an inline code span that starts with "fz ".
var inlineSpan = regexp.MustCompile("`(fz [^`]+)`")

// placeholder matches a <word> placeholder such as <n> or <step-id>.
var placeholder = regexp.MustCompile(`<[^<>]+>`)

// inlineFz returns every inline `fz ...` span outside fenced code blocks.
func inlineFz(text string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range inlineSpan.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// fillSpan turns a span from prose into a command line that can be parsed.
// It drops the brackets around optional parts and "..." repeats, and puts
// 87 in place of each <placeholder>. It returns "" for a span that shows
// the command syntax itself, such as fz <noun> <verb> --help, or that names
// a help topic by placeholder.
func fillSpan(span string) string {
	words := strings.Fields(span)
	if len(words) > 1 && placeholder.MatchString(words[1]) {
		return ""
	}
	if len(words) > 2 && words[1] == "help" && placeholder.MatchString(words[2]) {
		return ""
	}
	span = strings.NewReplacer("[", "", "]", "", "...", "").Replace(span)
	return placeholder.ReplaceAllString(span, "87")
}

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
func checkLine(line string) string { return check(line, true) }

// checkInline checks a span from prose. Prose often names a command without
// its arguments, as in "fz card show prints one card", so the argument count
// is checked only when the span gives at least one argument.
func checkInline(span string) string { return check(span, false) }

func check(line string, fullArgs bool) string {
	words, err := config.SplitWords(line)
	if err != nil {
		return err.Error()
	}
	root := newRoot()
	cmd, rest, err := root.Find(cli.Rewrite(words[1:]))
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
	// cobra's help command accepts any words, so check that they name a
	// real command.
	if cmd.Name() == "help" && cmd.Parent() == root {
		if len(positional) == 0 {
			return ""
		}
		target, extra, err := root.Find(positional)
		if err != nil || target == root || len(extra) > 0 {
			return "help names no command: " + strings.Join(positional, " ")
		}
		return ""
	}
	if !fullArgs && len(positional) == 0 {
		return ""
	}
	if err := cmd.ValidateArgs(positional); err != nil {
		return "args: " + err.Error()
	}
	return ""
}

func TestDocCommandsParse(t *testing.T) {
	for _, rel := range commandDocs {
		text := read(t, rel)
		lines := fzLines(text)
		if len(lines) == 0 && fencedDocs[rel] {
			t.Errorf("%s: no fz lines found in code blocks", rel)
		}
		for _, line := range lines {
			if msg := checkLine(line); msg != "" {
				t.Errorf("%s: %q: %s", rel, line, msg)
			}
		}
		for _, span := range inlineFz(text) {
			line := fillSpan(span)
			if line == "" {
				continue
			}
			if msg := checkInline(line); msg != "" {
				t.Errorf("%s: `%s`: %s", rel, span, msg)
			}
		}
	}
}

// The prose extractor finds spans outside code blocks only, and fills in
// placeholders.
func TestInlineSpans(t *testing.T) {
	text := "Run `fz card show <n>` or `fz board list`.\n```sh\n`fz card nope`\n```\nThen `fz`.\n"
	got := inlineFz(text)
	if strings.Join(got, "|") != "fz card show <n>|fz board list" {
		t.Errorf("inlineFz = %q", got)
	}
	for in, want := range map[string]string{
		"fz card show <n>":                    "fz card show 87",
		`fz card assign <n> "<user>"`:         `fz card assign 87 "87"`,
		"fz review [<board>...]":              "fz review 87",
		"fz card list [--state maybe/closed]": "fz card list --state maybe/closed",
		"fz <noun> <verb> --help":             "",
		"fz help <noun>":                      "",
	} {
		if got := fillSpan(in); got != want {
			t.Errorf("fillSpan(%q) = %q, want %q", in, got, want)
		}
	}
	for _, span := range []string{"fz card shw 87", "fz card show 87 --nope", "fz help nonexistent", "fz step add 87"} {
		if checkInline(span) == "" {
			t.Errorf("%q: passed, want an error", span)
		}
	}
	for _, span := range []string{"fz card show", "fz step list", "fz help card", "fz help card edit"} {
		if msg := checkInline(span); msg != "" {
			t.Errorf("%q: %s", span, msg)
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
		"fz help nonexistent",
		"fz help card lst",
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
