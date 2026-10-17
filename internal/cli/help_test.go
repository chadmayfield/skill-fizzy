package cli

import (
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/spf13/cobra"
)

// emDash is U+2014, which no help text may contain.
const emDash = '\u2014'

// walk calls f for cmd and every command below it, skipping cobra's own
// help and completion commands.
func walk(cmd *cobra.Command, f func(*cobra.Command)) {
	f(cmd)
	for _, c := range cmd.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		walk(c, f)
	}
}

// wantCommands is the command tree Task 6 builds. Later tasks add to it.
var wantCommands = []string{
	"fz board create", "fz board list",
	"fz card create", "fz card delete", "fz card edit", "fz card golden", "fz card list", "fz card move",
	"fz card reopen", "fz card show", "fz card tag",
	"fz column list",
	"fz comment add",
	"fz draft apply", "fz draft check", "fz draft preview",
	"fz hook session-start", "fz man", "fz review",
	"fz step add", "fz step check", "fz step delete", "fz step edit", "fz step list", "fz step uncheck",
	"fz upload",
}

func TestCommandTree(t *testing.T) {
	have := map[string]bool{}
	walk(NewRoot(), func(c *cobra.Command) {
		if c.Runnable() {
			have[c.CommandPath()] = true
		}
	})
	for _, name := range wantCommands {
		if !have[name] {
			t.Errorf("missing command %q", name)
		}
	}
}

func TestHelpText(t *testing.T) {
	root := NewRoot()
	walk(root, func(c *cobra.Command) {
		for field, text := range map[string]string{"Short": c.Short, "Long": c.Long, "Example": c.Example} {
			if strings.ContainsRune(text, emDash) {
				t.Errorf("%s: %s has an em dash", c.CommandPath(), field)
			}
		}
		if strings.ContainsRune(c.Flags().FlagUsages()+c.UsageString(), emDash) {
			t.Errorf("%s: flag help or usage has an em dash", c.CommandPath())
		}
		if c.HasSubCommands() && c.Short == "" {
			t.Errorf("%s: group has no Short", c.CommandPath())
		}
		if !c.Runnable() {
			return
		}
		if c.Short == "" || c.Long == "" || c.Example == "" {
			t.Errorf("%s: needs Short, Long and Example", c.CommandPath())
		}
		for _, line := range strings.Split(c.Example, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "fz ") {
				t.Errorf("%s: example %q does not start with fz", c.CommandPath(), line)
				continue
			}
			checkExample(t, c.CommandPath(), line)
		}
	})
}

// checkExample splits an example like a shell would, rewrites it, and checks
// that a fresh root resolves it to the command it documents, with flags and
// positional arguments that parse.
func checkExample(t *testing.T, path, line string) {
	t.Helper()
	words, err := config.SplitWords(line)
	if err != nil {
		t.Errorf("%s: example %q: %v", path, line, err)
		return
	}
	args := Rewrite(words[1:])
	cmd, rest, err := NewRoot().Find(args)
	if err != nil {
		t.Errorf("%s: example %q: %v", path, line, err)
		return
	}
	if cmd.CommandPath() != path {
		t.Errorf("%s: example %q resolves to %q", path, line, cmd.CommandPath())
		return
	}
	if err := cmd.ParseFlags(rest); err != nil {
		t.Errorf("%s: example %q: flags: %v", path, line, err)
		return
	}
	if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
		t.Errorf("%s: example %q: args: %v", path, line, err)
	}
}

func TestOldFormsInHelpResolve(t *testing.T) {
	for _, line := range []string{
		"fz boards", "fz board-create Garden --postpone 365", "fz columns Home",
		"fz cards --board Home", "fz show 87", `fz create Home "Fix the gate"`,
		"fz move 87 --to Now", "fz reopen 87", "fz golden 87 --off", "fz tag 87 buy",
		"fz delete 87 --yes", `fz step 87 "Order the hinge" --done`,
		"fz step 87 --check 03abc123def456ghi789jkl0mn", "fz step 87 --uncheck=03abc123def456ghi789jkl0mn",
		`fz comment 87 "Measured: 42 mm"`, "fz --dry-run --config x.json move 87 --to done",
	} {
		words, _ := config.SplitWords(line)
		cmd, rest, err := NewRoot().Find(Rewrite(words[1:]))
		if err != nil || !cmd.Runnable() {
			t.Errorf("%q: not a runnable command (%v)", line, err)
			continue
		}
		if err := cmd.ParseFlags(rest); err != nil {
			t.Errorf("%q: %v", line, err)
			continue
		}
		if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
			t.Errorf("%q: %v", line, err)
		}
	}
}
