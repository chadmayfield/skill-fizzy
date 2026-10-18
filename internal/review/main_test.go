package review

import (
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/testenv"
)

// TestMain keeps every test in this package away from the real config,
// token and server. See internal/testenv.
func TestMain(m *testing.M) { testenv.Main(m) }
