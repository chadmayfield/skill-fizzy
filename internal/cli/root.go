// Package cli builds the fz command tree.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

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
	return root
}

// run executes root with args and returns the process exit code: 0 on
// success, 1 for errors, 2 for usage errors.
func run(root *cobra.Command, args []string, out, errw io.Writer) int {
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errw)
	err := root.Execute()
	if err == nil {
		return 0
	}
	fmt.Fprintf(errw, "fz: %s\n", err)
	if _, ok := err.(usageError); ok || strings.HasPrefix(err.Error(), "unknown command") {
		return 2
	}
	return 1
}

// Execute runs fz with the process arguments and returns the exit code.
func Execute(version, commit, date string) int {
	_, _ = commit, date
	root := NewRoot()
	root.Version = version
	return run(root, os.Args[1:], os.Stdout, os.Stderr)
}
