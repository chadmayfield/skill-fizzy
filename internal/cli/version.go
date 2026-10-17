package cli

import "runtime/debug"

// resolveVersion returns the version fz reports. A version stamped by the
// Makefile is used as it is. An unstamped build ("dev"), such as one from
// go install, uses the module version from read, or failing that the first
// 7 characters of the VCS revision, with "-dirty" when the tree had local
// changes.
func resolveVersion(stamp string, read func() (*debug.BuildInfo, bool)) string {
	if stamp != "dev" {
		return stamp
	}
	info, ok := read()
	if !ok || info == nil {
		return stamp
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return stamp
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
