package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionFlag(t *testing.T) {
	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := out.String(); got != "fz dev\n" {
		t.Fatalf("version output = %q, want %q", got, "fz dev\n")
	}
}

func TestPersistentFlags(t *testing.T) {
	root := NewRoot()
	for _, name := range []string{"dry-run", "json", "config"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing persistent flag --%s", name)
		}
	}
}

func TestRegisteredFunctionIsApplied(t *testing.T) {
	saved := registrations
	t.Cleanup(func() { registrations = saved })
	registrations = nil
	register(func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{Use: "probe"})
	})
	root := NewRoot()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "probe" {
			found = true
		}
	}
	if !found {
		t.Fatal("NewRoot did not apply the registered function")
	}
}

func TestRun(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"help", []string{}, 0, ""},
		{"unknown flag", []string{"--nope"}, 2, "fz: "},
		{"unknown command", []string{"nope"}, 2, "fz: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			saved := registrations
			t.Cleanup(func() { registrations = saved })
			// Cobra only reports unknown commands when the root has subcommands.
			register(func(root *cobra.Command) {
				root.AddCommand(&cobra.Command{Use: "probe", Run: func(*cobra.Command, []string) {}})
			})
			code := run(NewRoot(), tc.args, &out, &errb)
			if code != tc.wantCode {
				t.Errorf("code = %d, want %d (stderr %q)", code, tc.wantCode, errb.String())
			}
			if tc.wantErr != "" && !strings.HasPrefix(errb.String(), tc.wantErr) {
				t.Errorf("stderr = %q, want prefix %q", errb.String(), tc.wantErr)
			}
		})
	}
}

// The root help's EXIT STATUS and FILES text match what fz does.
func TestRootHelpExitAndFiles(t *testing.T) {
	long := NewRoot().Long
	if !strings.Contains(long, "2 means a usage error or any refusal.") {
		t.Errorf("EXIT STATUS does not cover every refusal:\n%s", long)
	}
	if !strings.Contains(long, "FIZZY_TOKEN_CMD") || !strings.Contains(long[strings.Index(long, "FILES"):strings.Index(long, "ENVIRONMENT")], "FIZZY_TOKEN_CMD") {
		t.Errorf("FILES does not name FIZZY_TOKEN_CMD:\n%s", long)
	}
}
