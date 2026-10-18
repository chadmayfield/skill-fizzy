//go:build !unix

package config

import "os/exec"

// ownGroup leaves cmd as it is. Off Unix, a cancel kills the command only.
func ownGroup(*exec.Cmd) {}
