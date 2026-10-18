// Package releasecheck tests the release build in the Makefile and the
// release tag guard.
package releasecheck

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
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

// commitTime is the committer date of the one commit in the test's copy of
// the repo. make dist must stamp it into the binaries and the archives.
var commitTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// archiveNames lists the archives make dist must write, in checksums.txt order.
func archiveNames() []string {
	var names []string
	for _, p := range []string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"} {
		names = append(names, "fz_"+version+"_"+p+".tar.gz")
	}
	sort.Strings(names)
	return names
}

// makeDist runs make dist in dir with env added and moves the result to a
// new folder outside dir, so a second run neither overwrites it nor sees it
// as an untracked file (which would change the vcs.modified stamp).
func makeDist(t *testing.T, dir, label string, env ...string) string {
	t.Helper()
	cmd := exec.Command("make", "dist", "VERSION="+version)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make dist (%s): %v\n%s", label, err, out)
	}
	to := filepath.Join(t.TempDir(), label)
	if err := os.Rename(filepath.Join(dir, "dist"), to); err != nil {
		t.Fatal(err)
	}
	return to
}

// checkArchive reads one archive and checks that every entry is stamped
// with mtime, belongs to uid and gid 0 with no owner names, and comes in
// sorted order, and that gzip stored no file name or time.
func checkArchive(t *testing.T, path string, mtime time.Time) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if zr.Name != "" || !zr.ModTime.IsZero() {
		t.Errorf("%s: gzip header holds name %q and time %v, want neither", filepath.Base(path), zr.Name, zr.ModTime)
	}
	tr := tar.NewReader(zr)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		names = append(names, h.Name)
		if !h.ModTime.Equal(mtime) {
			t.Errorf("%s: %s mtime %v, want %v", filepath.Base(path), h.Name, h.ModTime.UTC(), mtime)
		}
		if h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" {
			t.Errorf("%s: %s owner %d:%d (%q:%q), want 0:0 with no names", filepath.Base(path), h.Name, h.Uid, h.Gid, h.Uname, h.Gname)
		}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("%s: entries not sorted: %v", filepath.Base(path), names)
	}
	return names
}

func TestMakeDist(t *testing.T) {
	if os.Getenv("FZ_TEST_DIST") != "1" {
		t.Skip("set FZ_TEST_DIST=1 to run the slow release build test")
	}
	for _, tool := range []string{"git", "make", "go", "tar", "gzip"} {
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
	// Commit the copy at a known time, so the build date has to come from git.
	gitIn(t, tmp, "init", "-q")
	gitIn(t, tmp, "add", "-A")
	stamp := commitTime.Format(time.RFC3339)
	commit := exec.Command("git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "test")
	commit.Dir = tmp
	commit.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	first := makeDist(t, tmp, "dist1", "SOURCE_DATE_EPOCH=")
	want := archiveNames()
	for _, name := range want {
		names := checkArchive(t, filepath.Join(first, name), commitTime)
		dir := strings.TrimSuffix(name, ".tar.gz")
		wantNames := []string{dir + "/", dir + "/LICENSE", dir + "/README.md", dir + "/fz"}
		if strings.Join(names, " ") != strings.Join(wantNames, " ") {
			t.Errorf("%s holds %v, want %v", name, names, wantNames)
		}
	}

	f, err := os.Open(filepath.Join(first, "checksums.txt"))
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
	verify.Dir = first
	if out, err := verify.CombinedOutput(); err != nil {
		t.Errorf("checksum verify: %v\n%s", err, out)
	}

	// A second build of the same commit must give the same bytes.
	second := makeDist(t, tmp, "dist2", "SOURCE_DATE_EPOCH=")
	sum1, _ := os.ReadFile(filepath.Join(first, "checksums.txt"))
	sum2, _ := os.ReadFile(filepath.Join(second, "checksums.txt"))
	if len(sum1) == 0 || string(sum1) != string(sum2) {
		t.Errorf("two builds of one commit differ:\n%s\n%s", sum1, sum2)
	}

	// SOURCE_DATE_EPOCH, when set, wins over the commit time.
	epoch := time.Date(2025, 10, 9, 8, 53, 20, 0, time.UTC)
	third := makeDist(t, tmp, "dist3", "SOURCE_DATE_EPOCH="+strconv.FormatInt(epoch.Unix(), 10))
	checkArchive(t, filepath.Join(third, want[0]), epoch)
}
