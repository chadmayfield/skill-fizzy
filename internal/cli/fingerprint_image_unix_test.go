//go:build unix

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

// The image hash refuses a file that is not regular, as Upload does, and
// never opens it, so a FIFO cannot hang it. ResolveImage already reports a
// FIFO as not found, so this is the check behind it.
func TestImageHashRefusesFIFO(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "pipe.jpg")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Skip("no FIFO:", err)
	}
	_, err := imageHash(pipe)
	if !fizzy.IsRefusal(err) || !strings.Contains(err.Error(), "is not a regular file") {
		t.Errorf("err = %v, want Upload's refusal", err)
	}
}
