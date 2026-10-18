//go:build !unix

package config

import "os"

// fileUID gives no owner off Unix, so the owner check is skipped there.
func fileUID(os.FileInfo) (int, bool) { return 0, false }
