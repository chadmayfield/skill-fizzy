//go:build unix

package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// A config file owned by another user is refused, whatever its mode: the
// owner could change the command fz runs.
func TestConfigFileOwnedByAnotherUser(t *testing.T) {
	isolate(t)
	p := writeFile(t, `{"url":"https://fizzy.example.com","account":"1"}`)
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	old := currentUID
	currentUID = func() int { return os.Getuid() + 1 }
	t.Cleanup(func() { currentUID = old })
	_, err := Load(p)
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "owned by another user") {
		t.Errorf("error = %v", err)
	}
}
