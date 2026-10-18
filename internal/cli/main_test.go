package cli

import (
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/testenv"
)

// TestMain keeps every test in this package away from the real config,
// token and server. See internal/testenv.
func TestMain(m *testing.M) { testenv.Main(m) }

// With no per-test environment, a read command stops at a config error and
// never builds a client.
func TestNoTestEnvMeansConfigError(t *testing.T) {
	r := runGo([]string{"board", "list"})
	if r.code != 1 || !strings.HasPrefix(r.stderr, "fz: set FIZZY_URL and FIZZY_ACCOUNT, or url/account in ") || r.stdout != "" {
		t.Fatalf("got %+v", r)
	}
	if strings.Contains(r.stderr, "dial") || strings.Contains(r.stderr, "http") {
		t.Errorf("a network attempt: %q", r.stderr)
	}
}
