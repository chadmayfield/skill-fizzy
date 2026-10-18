//go:build unix

package config

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestConfigRefusesPipe checks that neither config loader waits for a writer.
func TestConfigRefusesPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, read := range []struct {
		name string
		run  func() error
	}{
		{"config", func() error { _, err := Load(path); return err }},
		{"inbox", func() error { _, err := LoadInbox(path); return err }},
	} {
		t.Run(read.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- read.run() }()
			select {
			case err := <-done:
				if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "regular file") {
					t.Errorf("error %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("config read blocked on a pipe")
			}
		})
	}
}
