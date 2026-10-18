package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// resolveVersion returns the version fz reports. A version stamped by the
// Makefile is used as it is. An unstamped build ("dev" or empty), such as one from
// go install, uses the module version from read, or failing that the first
// 7 characters of the VCS revision, with "-dirty" when the tree had local
// changes.
func resolveVersion(stamp string, read func() (*debug.BuildInfo, bool)) string {
	if stamp == "" {
		stamp = "dev"
	}
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

// setVersion sets what fz --version prints: the version, then the commit
// and the build date in parentheses when the Makefile stamped them. An
// unstamped build has the commit "none" and the date "unknown", from
// cmd/fz. The man pages name the version only.
func setVersion(root *cobra.Command, version, commit, date string) {
	root.Version = version
	var build []string
	if commit != "" && commit != "none" {
		build = append(build, "commit "+commit)
	}
	if date != "" && date != "unknown" {
		build = append(build, "built "+date)
	}
	if len(build) == 0 {
		return
	}
	// The text goes in an annotation, not the template, so a stamp is
	// never read as template code.
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations["build"] = strings.Join(build, ", ")
	root.SetVersionTemplate(`{{.Name}} {{.Version}}{{with index .Annotations "build"}} ({{.}}){{end}}` + "\n")
}
