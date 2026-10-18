// Package cli builds the fz command tree.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

// registrations holds one function per command file. Each command file calls
// register from its own init(), so adding a command never edits this file or
// any shared list. NewRoot applies every registered function in the order
// the init functions ran.
var registrations []func(root *cobra.Command)

// register adds a function that NewRoot calls to attach commands to the root.
func register(f func(root *cobra.Command)) {
	registrations = append(registrations, f)
}

// subgroup returns the command group named name, or nil. Go runs a
// package's init functions in file name order, so a file that adds to a
// group must sort after the file that registers it, as cards_edit.go sorts
// after cards.go.
func subgroup(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// usageError marks an error caused by bad flags or an unknown command.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// NewRoot returns the root command with the persistent flags and every
// registered command attached.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "fz",
		Short:         "Command-line client for the Fizzy API",
		Long:          "fz is a command-line client for the Fizzy API. The server URL, account and token come from the config file or from environment variables.",
		Version:       "dev",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})
	root.PersistentFlags().Bool("dry-run", false, "show what would change without sending it")
	root.PersistentFlags().Bool("json", false, "print JSON instead of text")
	root.PersistentFlags().String("config", "", "path to the config file")
	for _, f := range registrations {
		f(root)
	}
	root.SetHelpCommand(helpCommand())
	return root
}

// helpCommand is cobra's help command, except that a topic that names no
// command is a usage error, so fz help nope exits 2 as fz nope does.
func helpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long:  "Help prints the help for any command. Run fz help followed by the command's words, such as fz help card list.",
		Args:  args(cobra.ArbitraryArgs),
		RunE: func(c *cobra.Command, a []string) error {
			cmd, rest, err := c.Root().Find(a)
			if cmd == nil || err != nil || len(rest) > 0 {
				return usagef("unknown help topic %s; fz help lists the commands", pyRepr(strings.Join(a, " ")))
			}
			cmd.InitDefaultHelpFlag()
			cmd.InitDefaultVersionFlag()
			return cmd.Help()
		},
	}
}

// run rewrites old command forms, executes root with args and returns the
// process exit code: 0 on success, 1 for errors, 2 for usage errors and
// refusals.
func run(root *cobra.Command, args []string, out, errw io.Writer) int {
	args = Rewrite(args)
	root.SetArgs(args)
	root.SetOut(terminalWriter{out})
	root.SetErr(terminalWriter{errw})
	err := unknownSubcommand(root, args)
	if err == nil {
		err = root.Execute()
	}
	if err == nil {
		return 0
	}
	var code exitCode
	if errors.As(err, &code) {
		return int(code)
	}
	// The error can hold server text, such as a user or tag name, so it
	// goes through the terminal cleaner like everything else fz prints.
	fmt.Fprintf(terminalWriter{errw}, "fz: %s\n", errorText(err))
	if _, ok := err.(usageError); ok || fizzy.IsRefusal(err) || strings.HasPrefix(err.Error(), "unknown command") {
		return 2
	}
	return 1
}

// terminalWriter passes everything fz prints through config.TerminalText,
// so no text from the server can send a terminal escape or overwrite a
// line. JSON output passes unchanged: it holds only escaped ASCII. Each
// write is cleaned on its own; fz writes each line, or each whole output,
// in one call.
type terminalWriter struct{ w io.Writer }

func (t terminalWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(t.w, config.TerminalText(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

// unknownSubcommand reports "fz card nope" as a usage error. Cobra only
// checks unknown commands on the root; a group such as card would print its
// help and exit 0.
func unknownSubcommand(root *cobra.Command, args []string) error {
	cmd, rest, err := root.Find(args)
	if err != nil || cmd == root || cmd.Runnable() || !cmd.HasSubCommands() {
		return nil
	}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "-") {
			return usageError{fmt.Errorf("unknown command %q for %q", a, cmd.CommandPath())}
		}
		// Skip the value of a long flag that takes one, as in --config x.json.
		if strings.HasPrefix(a, "--") && !strings.Contains(a, "=") {
			if f := cmd.Flag(a[2:]); f != nil && f.NoOptDefVal == "" {
				i++
			}
		}
	}
	return nil
}

// Execute runs fz with the process arguments and returns the exit code.
func Execute(version, commit, date string) int {
	root := NewRoot()
	setVersion(root, resolveVersion(version, debug.ReadBuildInfo), commit, date)
	return run(root, os.Args[1:], os.Stdout, os.Stderr)
}
