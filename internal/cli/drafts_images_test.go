package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// imageEnv points fz at srv with a home it returns, whose inbox holds a
// drafts folder, and a private file outside both.
func imageEnv(t *testing.T, srv *syncServer) (home, drafts string) {
	t.Helper()
	home = t.TempDir()
	for k, v := range map[string]string{
		"FIZZY_URL": srv.URL, "FIZZY_ACCOUNT": "1", "FIZZY_TOKEN": "T", "FIZZY_CONFIG": "/nonexistent",
		"FIZZY_TOKEN_CMD": "", "FIZZY_INBOX": filepath.Join(home, "inbox"), "FIZZY_RULES_CARD": "", "HOME": home,
	} {
		t.Setenv(k, v)
	}
	drafts = filepath.Join(home, "inbox", "drafts")
	if err := os.MkdirAll(drafts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".ssh", "id_ed25519"), "PRIVATE KEY BYTES")
	return home, drafts
}

// A draft image outside the draft's folder and the inbox is refused with
// exit 2 by check, preview and apply, and apply sends nothing.
func TestDraftImageOutsideRootsRefused(t *testing.T) {
	for _, target := range []string{"~/.ssh/id_ed25519", "../../.ssh/id_ed25519", "ABS", "LINK", "--><b>x</b>"} {
		t.Run(target, func(t *testing.T) {
			srv := newSyncServer(t)
			home, drafts := imageEnv(t, srv)
			switch target {
			case "ABS":
				target = filepath.Join(home, ".ssh", "id_ed25519")
			case "LINK":
				if err := os.Symlink(filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(drafts, "key.png")); err != nil {
					t.Fatal(err)
				}
				target = "key.png"
			}
			p := filepath.Join(drafts, "gate.md")
			writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\n![key]("+target+")\n")
			for _, cmd := range []string{"check", "preview", "apply"} {
				got := runGo([]string{"draft", cmd, p})
				if got.code != 2 || !strings.Contains(got.stdout+got.stderr, "image") {
					t.Errorf("%s: code %d stdout %q stderr %q", cmd, got.code, got.stdout, got.stderr)
				}
			}
			if w := srv.writes(); len(w) != 0 {
				t.Errorf("writes reached the server: %q", w)
			}
		})
	}
}

// An image in the inbox, named from a draft in a folder inside it, is fine.
func TestDraftImageInInbox(t *testing.T) {
	srv := newSyncServer(t)
	home, drafts := imageEnv(t, srv)
	writeFile(t, filepath.Join(home, "inbox", "hinge.jpg"), "jpg")
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\n![a](../hinge.jpg)\n\n![b](hinge.jpg)\n")
	if got := runGo([]string{"draft", "check", p}); got.code != 0 || !strings.Contains(got.stdout, "2 images") {
		t.Errorf("check: %+v", got)
	}
}

// draft check reads no server settings: with no URL or account it still
// checks a draft.
func TestDraftCheckNeedsNoServer(t *testing.T) {
	srv := newSyncServer(t)
	_, drafts := imageEnv(t, srv)
	t.Setenv("FIZZY_URL", "")
	t.Setenv("FIZZY_ACCOUNT", "")
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n")
	if got := runGo([]string{"draft", "check", p}); got.code != 0 || !strings.Contains(got.stdout, "✓") {
		t.Errorf("check: %+v", got)
	}
}

// apply prints the resolved path of each image before it sends the file,
// so the user sees which file left the disk. A create prints one image:
// line per image; an update prints it after the card number.
func TestApplyPrintsImagePaths(t *testing.T) {
	srv := newSyncServer(t)
	srv.uploads = true
	home, drafts := imageEnv(t, srv)
	inboxImg := filepath.Join(home, "inbox", "hinge.jpg")
	writeFile(t, inboxImg, "jpg")
	draftImg := filepath.Join(drafts, "latch.jpg")
	writeFile(t, draftImg, "jpg")
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\n![a](../hinge.jpg)\n\n![b](latch.jpg)\n")
	// The paths printed are the real ones, with symlinks resolved, as the
	// temp folder is on macOS.
	for _, img := range []*string{&inboxImg, &draftImg} {
		real, err := filepath.EvalSymlinks(*img)
		if err != nil {
			t.Fatal(err)
		}
		*img = real
	}
	got := runGo([]string{"draft", "apply", p})
	if got.code != 0 || !strings.HasPrefix(got.stdout, "image: "+inboxImg+"\nimage: "+draftImg+"\n#90 ") {
		t.Errorf("create: %+v", got)
	}
	// Changed bytes, so the update sends both images again.
	writeFile(t, inboxImg, "jpg2")
	got = runGo([]string{"draft", "apply", "--update", "--force", p})
	if got.code != 0 || !strings.Contains(got.stdout, "#90 image: "+inboxImg+"\n#90 image: "+draftImg+"\n") {
		t.Errorf("update: %+v", got)
	}
}
