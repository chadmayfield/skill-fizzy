//go:build !unix || solaris || aix

package cli

import (
	"errors"
	"fmt"
	"os"
)

// staleLockHelp ends the refusal for a held lock. Here the lock is a file
// made with O_EXCL, which stays if fz is killed, so a held lock may be
// stale.
const staleLockHelp = "If no apply is running, the lock was left by one that stopped: check the draft's front matter, then delete the lock"

// acquireLock creates the lock file l with O_EXCL. The returned function
// removes it. A file that already exists is errLockHeld, a symlink
// included. A nil function and nil error mean the folder cannot hold a
// lock.
func acquireLock(l string) (func(), error) {
	f, err := os.OpenFile(l, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, errLockHeld
	}
	if err != nil {
		return nil, nil
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(l) }, nil
}
