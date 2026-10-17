package releasecheck

import (
	"os"
	"os/exec"
	"path/filepath"
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

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	commit := func(msg string) string {
		gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", msg)
		return gitIn(t, repo, "rev-parse", "HEAD")
	}
	onMain := commit("on main")
	gitIn(t, repo, "branch", "-M", "main")
	// The workflow fetches origin/main; here the ref is set by hand.
	gitIn(t, repo, "update-ref", "refs/remotes/origin/main", onMain)
	gitIn(t, repo, "checkout", "-q", "-b", "side")
	onSide := commit("on a side branch")

	cases := []struct {
		tag, sha string
		wantOut  string
		wantFail bool
	}{
		{"v1.2.3", onMain, "release", false},
		{"v1.2.3-rc1", onMain, "prerelease", false},
		{"vtest", onMain, "", true},
		{"v1.2.4", onSide, "", true},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			cmd := exec.Command("sh", script)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GITHUB_REF_NAME="+c.tag, "GITHUB_SHA="+c.sha)
			out, err := cmd.Output()
			if c.wantFail {
				if err == nil {
					t.Fatalf("guard passed %s, output %q", c.tag, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("guard failed %s: %v", c.tag, err)
			}
			if got := strings.TrimSpace(string(out)); got != c.wantOut {
				t.Errorf("output %q, want %q", got, c.wantOut)
			}
		})
	}
}
