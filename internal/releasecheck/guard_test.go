package releasecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gitIn runs git in dir with a fixed identity and returns its output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestReleaseGuard(t *testing.T) {
	for _, tool := range []string{"git", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available", tool)
		}
	}
	script, err := filepath.Abs("../../scripts/release-guard.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "work")
	gitIn(t, root, "init", "-q", "--bare", "-b", "main", origin)
	gitIn(t, root, "clone", "-q", origin, repo)
	gitIn(t, repo, "checkout", "-q", "-b", "main")
	commit := func(msg string) string {
		gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", msg)
		return gitIn(t, repo, "rev-parse", "HEAD")
	}
	onMain := commit("on main")
	gitIn(t, repo, "push", "-q", "origin", "main")
	gitIn(t, repo, "checkout", "-q", "-b", "side")
	onSide := commit("on a side branch")
	// The same fetch the workflow runs.
	gitIn(t, repo, "fetch", "-q", "origin", "main")

	cases := []struct {
		tag, sha string
		wantKind string
	}{
		{"v1.2.3", onMain, "release"},
		{"v0.0.1", onMain, "release"},
		{"v1.2.3-rc1", onMain, "prerelease"},
		{"vtest", onMain, ""},
		{"v1.2.4", onSide, ""},
		{"v1.2.3\nx", onMain, ""},
		{"x\nv1.2.3", onMain, ""},
		{"v1.2.3\r", onMain, ""},
		{"v01.2.3", onMain, ""},
		{"v1.02.3", onMain, ""},
		{"v1.2.03", onMain, ""},
		{"v1.2", onMain, ""},
		{"v1.2.3.4", onMain, ""},
		{"v1.2.3-", onMain, ""},
		{"v1.2.3 ", onMain, ""},
		// SemVer pre-release identifiers: none may be empty, and a numeric
		// one may not have a leading zero. An alphanumeric one may.
		{"v1.2.3-rc.1", onMain, "prerelease"},
		{"v1.2.3-rc.0", onMain, "prerelease"},
		{"v1.2.3-0", onMain, "prerelease"},
		{"v1.2.3-0a.01a", onMain, "prerelease"},
		{"v1.2.3-x-y", onMain, "prerelease"},
		{"v1.2.3-rc.01", onMain, ""},
		{"v1.2.3-01", onMain, ""},
		{"v1.2.3-a..b", onMain, ""},
		{"v1.2.3-.a", onMain, ""},
		{"v1.2.3-a.", onMain, ""},
	}
	for i, c := range cases {
		t.Run(strconv.Itoa(i)+"/"+strings.NewReplacer("\n", "NL", "\r", "CR", " ", "_").Replace(c.tag), func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "output")
			cmd := exec.Command("sh", script)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GITHUB_REF_NAME="+c.tag, "GITHUB_SHA="+c.sha, "GITHUB_OUTPUT="+outFile)
			out, err := cmd.Output()
			written, _ := os.ReadFile(outFile)
			if c.wantKind == "" {
				if err == nil {
					t.Fatalf("guard passed %q, output %q", c.tag, out)
				}
				if len(written) != 0 {
					t.Errorf("a rejected tag wrote %q to GITHUB_OUTPUT", written)
				}
				return
			}
			if err != nil {
				t.Fatalf("guard failed %q: %v", c.tag, err)
			}
			if got := strings.TrimSpace(string(out)); got != c.wantKind {
				t.Errorf("stdout %q, want %q", got, c.wantKind)
			}
			if string(written) != "kind="+c.wantKind+"\n" {
				t.Errorf("GITHUB_OUTPUT = %q", written)
			}
		})
	}
}
