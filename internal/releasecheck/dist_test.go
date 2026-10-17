// Package releasecheck tests the release build in the Makefile.
package releasecheck

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const version = "v0.0.0-test"

// copyTracked copies the files git tracks (as they are on disk now) from the
// repository at top into dst, so the test never writes to the checkout.
func copyTracked(t *testing.T, top, dst string) {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--exclude-standard")
	cmd.Dir = top
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		src := filepath.Join(top, rel)
		data, err := os.ReadFile(src)
		if err != nil {
			continue // deleted in the work tree
		}
		to := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMakeDist(t *testing.T) {
	if os.Getenv("FZ_TEST_DIST") != "1" {
		t.Skip("set FZ_TEST_DIST=1 to run the slow release build test")
	}
	for _, tool := range []string{"git", "make", "go", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available", tool)
		}
	}
	topOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skip("not inside a git work tree")
	}
	tmp := t.TempDir()
	copyTracked(t, strings.TrimSpace(string(topOut)), tmp)

	cmd := exec.Command("make", "dist", "VERSION="+version)
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make dist: %v\n%s", err, out)
	}

	dist := filepath.Join(tmp, "dist")
	var want []string
	for _, p := range []string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"} {
		name := "fz_" + version + "_" + p + ".tar.gz"
		want = append(want, name)
		if _, err := os.Stat(filepath.Join(dist, name)); err != nil {
			t.Errorf("missing archive: %v", err)
		}
		list, err := exec.Command("tar", "-tzf", filepath.Join(dist, name)).Output()
		if err != nil {
			t.Errorf("tar -tzf %s: %v", name, err)
			continue
		}
		dir := strings.TrimSuffix(name, ".tar.gz")
		for _, f := range []string{"fz", "LICENSE", "README.md"} {
			if !strings.Contains(string(list), dir+"/"+f+"\n") {
				t.Errorf("%s lacks %s/%s", name, dir, f)
			}
		}
	}

	sort.Strings(want)
	f, err := os.Open(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	line := regexp.MustCompile(`^[0-9a-f]{64}  (\S+\.tar\.gz)$`)
	var got []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := line.FindStringSubmatch(sc.Text())
		if m == nil {
			t.Errorf("bad checksums.txt line: %q", sc.Text())
			continue
		}
		got = append(got, m[1])
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("checksums.txt lists %v, want %v", got, want)
	}

	verify := exec.Command("sh", "-c", "if command -v sha256sum >/dev/null 2>&1; then sha256sum -c checksums.txt; else shasum -a 256 -c checksums.txt; fi")
	verify.Dir = dist
	if out, err := verify.CombinedOutput(); err != nil {
		t.Errorf("checksum verify: %v\n%s", err, out)
	}
}
