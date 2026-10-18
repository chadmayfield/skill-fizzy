//go:build unix && !solaris && !aix

package cli

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// staleLockHelp ends the refusal for a held lock. Here the lock is a
// flock(2), which the kernel drops when its process dies, so a held lock
// always means a live apply.
const staleLockHelp = "Wait for it to finish, then rerun"

// acquireLock opens the lock file l, creating it, and takes an exclusive
// flock(2) on it without waiting. The kernel releases the flock when fz
// exits, even on SIGKILL, so a file left by a killed apply does not block
// the next one. The returned function removes the file and then closes it.
// It returns errLockHeld when another process holds the flock, and
// errLockUnsafe when l is a symlink, is not regular or has multiple hard
// links. A nil function and nil error mean the folder cannot hold a lock.
func acquireLock(l string) (func(), error) {
	for range 5 {
		f, err := os.OpenFile(l, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
		if err != nil {
			if _, lerr := os.Lstat(l); lerr == nil {
				return nil, errLockUnsafe
			}
			return nil, nil
		}
		fi, err := f.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			f.Close()
			return nil, errLockUnsafe
		}
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
			f.Close()
			return nil, errLockUnsafe
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				return nil, errLockHeld
			}
			return nil, err
		}
		// An apply that was finishing may have removed the file after this
		// one opened it. The flock is then on a file no other apply can
		// find, so open the path again.
		if pfi, err := os.Lstat(l); err != nil || !os.SameFile(fi, pfi) {
			f.Close()
			continue
		}
		if err := f.Truncate(0); err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
		}
		return func() {
			os.Remove(l)
			f.Close()
		}, nil
	}
	return nil, errLockHeld
}
