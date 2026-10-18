// Package testenv keeps tests away from the real fz config and server.
// Each test package that builds a client or runs commands calls Main from
// its TestMain.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fizzyVars are the environment variables fz reads. Isolate unsets them, so
// a test that needs one sets it with t.Setenv.
var fizzyVars = []string{
	"FIZZY_URL", "FIZZY_ACCOUNT", "FIZZY_TOKEN", "FIZZY_TOKEN_CMD",
	"FIZZY_INBOX", "FIZZY_RULES_CARD",
}

// Isolate points HOME at a new temporary directory, points FIZZY_CONFIG at
// a file that does not exist, and unsets the other FIZZY_ variables. The Go
// build and module caches keep their real places, so a test that runs the
// go command still finds them. It returns a function that removes the
// directory.
func Isolate() (func(), error) {
	if err := keepGoDirs(); err != nil {
		return nil, err
	}
	home, err := os.MkdirTemp("", "fz-test-home-")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv("HOME", home); err != nil {
		return nil, err
	}
	if err := os.Setenv("FIZZY_CONFIG", filepath.Join(home, "no-such-config.json")); err != nil {
		return nil, err
	}
	for _, v := range fizzyVars {
		if err := os.Unsetenv(v); err != nil {
			return nil, err
		}
	}
	return func() { os.RemoveAll(home) }, nil
}

// keepGoDirs sets GOCACHE and GOPATH to the places Go would use under the
// real home directory, unless they are already set.
func keepGoDirs() error {
	if os.Getenv("GOCACHE") == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		if err := os.Setenv("GOCACHE", filepath.Join(dir, "go-build")); err != nil {
			return err
		}
	}
	if os.Getenv("GOPATH") == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if err := os.Setenv("GOPATH", filepath.Join(home, "go")); err != nil {
			return err
		}
	}
	return nil
}

// Main isolates the environment, runs the tests and exits with their code.
func Main(m *testing.M) {
	cleanup, err := Isolate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
