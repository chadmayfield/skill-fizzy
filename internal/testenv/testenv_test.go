package testenv_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/testenv"
)

func TestIsolate(t *testing.T) {
	realHome, _ := os.UserHomeDir()
	for _, v := range []string{"HOME", "FIZZY_CONFIG", "FIZZY_URL", "FIZZY_ACCOUNT", "FIZZY_TOKEN", "FIZZY_TOKEN_CMD", "FIZZY_REPOS_MAP", "GOCACHE", "GOPATH"} {
		t.Setenv(v, os.Getenv(v))
	}
	t.Setenv("FIZZY_URL", "https://fizzy.example.com")
	t.Setenv("FIZZY_ACCOUNT", "1")
	t.Setenv("FIZZY_TOKEN", "T")
	t.Setenv("FIZZY_REPOS_MAP", "/tmp/repos.json")
	cleanup, err := testenv.Isolate()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if home := os.Getenv("HOME"); home == realHome || !strings.Contains(home, "fz-test-home-") {
		t.Errorf("HOME = %q", home)
	}
	if _, err := os.Stat(os.Getenv("FIZZY_CONFIG")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("FIZZY_CONFIG %q exists: %v", os.Getenv("FIZZY_CONFIG"), err)
	}
	for _, v := range []string{"FIZZY_URL", "FIZZY_ACCOUNT", "FIZZY_TOKEN", "FIZZY_TOKEN_CMD", "FIZZY_REPOS_MAP"} {
		if _, set := os.LookupEnv(v); set {
			t.Errorf("%s is still set", v)
		}
	}
	if _, err := config.Load(""); !errors.Is(err, config.ErrConfig) {
		t.Errorf("config.Load = %v, want a config error", err)
	}
}

// Isolate moves HOME, which moves the default go env file, so it pins
// GOENV to the file in use before. A test that runs the go command keeps
// settings such as GOPROXY or GOPRIVATE.
func TestIsolateKeepsGoEnvFile(t *testing.T) {
	for _, v := range []string{"HOME", "FIZZY_CONFIG", "GOCACHE", "GOPATH", "GOENV"} {
		t.Setenv(v, os.Getenv(v))
	}
	os.Unsetenv("GOENV")
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	want := filepath.Join(dir, "go", "env")
	cleanup, err := testenv.Isolate()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got := os.Getenv("GOENV"); got != want {
		t.Errorf("GOENV = %q, want %q", got, want)
	}
}

// After GuardNetwork, the default transport dials loopback addresses only.
// A test whose guard regresses fails at once and never reaches a real host.
func TestGuardNetwork(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	testenv.GuardNetwork()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("loopback: %v", err)
	}
	resp.Body.Close()
	for _, u := range []string{"http://storage.example.com/put", "https://fizzy.example.com/", "http://192.0.2.1/", "http://[2001:db8::1]/"} {
		start := time.Now()
		_, err := http.Get(u)
		if err == nil || !strings.Contains(err.Error(), "tests dial only loopback addresses") {
			t.Errorf("%s: %v", u, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s took %v", u, d)
		}
	}
}
