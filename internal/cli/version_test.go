package cli

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(main string, settings ...string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			bi := &debug.BuildInfo{Main: debug.Module{Path: "github.com/chadmayfield/skill-fizzy", Version: main}}
			for i := 0; i+1 < len(settings); i += 2 {
				bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
			}
			return bi, true
		}
	}
	noInfo := func() (*debug.BuildInfo, bool) { return nil, false }
	const rev = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name, stamp string
		read        func() (*debug.BuildInfo, bool)
		want        string
	}{
		{"make stamp wins", "v1.2.0", info("v9.9.9"), "v1.2.0"},
		{"make stamp wins over vcs", "v1.2.0-3-gabc1234-dirty", info("(devel)", "vcs.revision", rev), "v1.2.0-3-gabc1234-dirty"},
		{"go install of a tag", "dev", info("v1.2.0"), "v1.2.0"},
		{"go install of a pseudo-version", "dev", info("v0.0.0-20261017120000-0123456789ab"), "v0.0.0-20261017120000-0123456789ab"},
		{"devel falls back to the revision", "dev", info("(devel)", "vcs.revision", rev, "vcs.modified", "false"), "0123456"},
		{"devel with local edits", "dev", info("(devel)", "vcs.revision", rev, "vcs.modified", "true"), "0123456-dirty"},
		{"empty main version", "dev", info("", "vcs.revision", rev), "0123456"},
		{"short revision kept whole", "dev", info("(devel)", "vcs.revision", "abc"), "abc"},
		{"devel without vcs", "dev", info("(devel)"), "dev"},
		{"no build info", "dev", noInfo, "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.stamp, c.read); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveVersionEmptyStampActsLikeDev(t *testing.T) {
	read := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.2.0"}}, true
	}
	if got := resolveVersion("", read); got != "v1.2.0" {
		t.Errorf("empty stamp with build info: got %q", got)
	}
	noInfo := func() (*debug.BuildInfo, bool) { return nil, false }
	if got := resolveVersion("", noInfo); got != "dev" {
		t.Errorf("empty stamp without build info: got %q, want dev", got)
	}
}
