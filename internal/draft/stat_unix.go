//go:build unix

package draft

import (
	"errors"
	"os"
	"syscall"
)

// linkCount returns the number of hard links to the file fi describes.
func linkCount(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}

// keepOwner gives f the owner and group of the file fi describes. A user
// who is not root can only give a file away to a group they are in, so a
// change the system does not allow is skipped, not an error.
func keepOwner(f *os.File, fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	err := f.Chown(int(st.Uid), int(st.Gid))
	if errors.Is(err, syscall.EPERM) {
		err = f.Chown(-1, int(st.Gid))
	}
	if errors.Is(err, syscall.EPERM) {
		return nil
	}
	return err
}
