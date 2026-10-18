package draft

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// draftIn writes a draft with body into dir and parses it.
func draftIn(t *testing.T, dir, body string) *Draft {
	t.Helper()
	d, err := Parse(writeFile(t, dir, "gate.md", "---\nboard: Home\n---\n# Fix the gate\n\n"+body+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Images, CheckImages and ToHTML split lines and track code fences the same
// way, so the images fz uploads are the ones the description embeds.
func TestImagesFollowToHTMLLines(t *testing.T) {
	ls := string(rune(0x2028))
	dir := t.TempDir()
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"line separator inside an image line", "![a" + ls + "b](x.png)", []string{"x.png"}},
		{"vertical tab before an image", "text\v![a](x.png)", []string{}},
		{"image line in a code fence", "```\n![shot](missing.png)\n```", []string{}},
		{"image after a code fence", "```\ncode\n```\n![shot](x.png)", []string{"x.png"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := draftIn(t, dir, tc.body)
			if got := d.Images(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Images() = %q, want %q", got, tc.want)
			}
			html := d.DescriptionHTML(nil)
			for _, img := range tc.want {
				if !strings.Contains(html, "[image: "+img+"]") {
					t.Errorf("the description does not embed %s: %s", img, html)
				}
			}
			if strings.Contains(html, "<!--attach:") {
				t.Errorf("a placeholder was left in %s", html)
			}
		})
	}
	// An image line in a fence is code, so a missing file there is fine.
	if err := draftIn(t, dir, "```\n![shot](missing.png)\n```").CheckImages(dir); err != nil {
		t.Errorf("fenced image: %v", err)
	}
}

// An image target with -->, < or > is refused before anything is sent: in
// the placeholder it could end the comment and leave raw HTML.
func TestImageTargetMarkupRefused(t *testing.T) {
	ls := string(rune(0x2028))
	dir := t.TempDir()
	for _, target := range []string{"--><img/src/onerror=alert`1`>", "a<b.png", "a>b.png", "x-->.png"} {
		for _, body := range []string{"![a](" + target + ")", "![a" + ls + "b](" + target + ")"} {
			writeFile(t, dir, strings.NewReplacer("/", "_").Replace(target), "x")
			err := draftIn(t, dir, body).CheckImages(dir)
			if !errors.Is(err, ErrImagePath) || !errors.Is(err, ErrDraft) {
				t.Errorf("%q: %v", body, err)
			}
		}
	}
}

// An image resolves only inside the draft's folder or the inbox, after
// symlinks are resolved. Absolute paths, ~ paths and .. that leaves both
// are refused.
func TestImagePathRoots(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	drafts := filepath.Join(inbox, "drafts")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{drafts, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, drafts, "here.png", "x")
	writeFile(t, inbox, "shared.png", "x")
	writeFile(t, outside, "key", "PRIVATE")
	if err := os.Symlink(filepath.Join(outside, "key"), filepath.Join(drafts, "link.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(inbox, "shared.png"), filepath.Join(drafts, "ok-link.png")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, "home.png", "x")
	d := draftIn(t, drafts, "body")
	for _, target := range []string{"here.png", "shared.png", "../shared.png", "ok-link.png"} {
		if _, err := d.ResolveImage(target, inbox); err != nil {
			t.Errorf("%s: %v", target, err)
		}
	}
	for _, target := range []string{
		filepath.Join(outside, "key"), "~/home.png", "~", "../../outside/key", "../../../" + strings.TrimPrefix(filepath.Join(outside, "key"), "/"), "link.png",
	} {
		_, err := d.ResolveImage(target, inbox)
		if !errors.Is(err, ErrImagePath) {
			t.Errorf("%s: %v, want a refusal", target, err)
		}
	}
	if _, err := d.ResolveImage("nope.png", inbox); err == nil || errors.Is(err, ErrImagePath) || !errors.Is(err, ErrDraft) {
		t.Errorf("missing file: %v", err)
	}
}
