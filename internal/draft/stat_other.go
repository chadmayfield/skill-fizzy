//go:build !unix

package draft

import "os"

// linkCount is 1 off Unix, where fz does not read the link count.
func linkCount(os.FileInfo) uint64 { return 1 }

// keepOwner does nothing off Unix.
func keepOwner(*os.File, os.FileInfo) error { return nil }
