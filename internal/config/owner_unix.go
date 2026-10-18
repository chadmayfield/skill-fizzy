//go:build unix

package config

import (
	"os"
	"syscall"
)

// fileUID returns the owner of fi, and false when the file system gives
// none.
func fileUID(fi os.FileInfo) (int, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
