// Package testenv keeps tests away from the real fz config and server.
// Each test package that builds a client or runs commands calls Main from
// its TestMain.
package testenv

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fizzyVars are the environment variables fz reads. Isolate unsets them, so
// a test that needs one sets it with t.Setenv.
var fizzyVars = []string{
	"FIZZY_URL", "FIZZY_ACCOUNT", "FIZZY_TOKEN", "FIZZY_TOKEN_CMD",
	"FIZZY_INBOX", "FIZZY_RULES_CARD", "FIZZY_REPOS_MAP",
}

// Isolate points HOME at a new temporary directory, points FIZZY_CONFIG at
// a file that does not exist, and unsets the other FIZZY_ variables. The Go
// build and module caches and the go env file keep their real places, so a
// test that runs the go command still finds them and its settings. It returns a function that removes the
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
	// The go env file holds go env -w settings such as GOPROXY. Its default
	// place is under the user config dir, which follows HOME.
	if os.Getenv("GOENV") == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		if err := os.Setenv("GOENV", filepath.Join(dir, "go", "env")); err != nil {
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

// GuardNetwork makes http.DefaultTransport dial only loopback addresses and
// localhost, with no proxy. It covers fz's API client, but not transports a
// test or the --links checker builds, or subprocesses such as the Python hook.
func GuardNetwork() {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	d := &net.Dialer{Timeout: 5 * time.Second}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("testenv: refused to dial %s: tests dial only loopback addresses", addr)
		}
		return d.DialContext(ctx, network, addr)
	}
	http.DefaultTransport = t
}

// Main isolates the environment, guards the network, runs the tests and
// exits with their code.
func Main(m *testing.M) {
	cleanup, err := Isolate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		os.Exit(1)
	}
	GuardNetwork()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
