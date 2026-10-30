package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/chadmayfield/skill-fizzy/fizzy"
)

// draftHash stats each image before it reads it, and refuses a file over
// fizzy.MaxUploadSize with Upload's refusal, without reading it into
// memory.
func TestDraftHashRefusesHugeImage(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.jpg")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(400 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	p := filepath.Join(dir, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\n![x](big.jpg)\n")
	d, err := parseDraft(p)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err = draftHash(d, dir)
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 32<<20 {
		t.Errorf("draftHash allocated %d bytes for a 400 MiB file", grew)
	}
	if !fizzy.IsRefusal(err) || !strings.Contains(err.Error(), "is 419430400 bytes; fz uploads files up to 100 MiB") {
		t.Errorf("err = %v, want Upload's size refusal", err)
	}
}

// draftHash streams an image of legal size, and its hash is the one a
// whole-file read gives.
func TestDraftHashStreamsImage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "hinge.jpg")
	writeFile(t, img, strings.Repeat("hinge ", 1<<20))
	p := filepath.Join(dir, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\n![x](hinge.jpg)\n")
	d, err := parseDraft(p)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	h1, err := draftHash(d, dir)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 2<<20 {
		t.Errorf("draftHash allocated %d bytes for a 6 MiB file", grew)
	}
	h2, err := draftHash(d, dir)
	if err != nil || h1 != h2 || !fingerprintRE.MatchString(h1) {
		t.Errorf("hashes %q, %q, %v", h1, h2, err)
	}
}

// One --update run hashes each image once.
func TestUpdateHashesImageOnce(t *testing.T) {
	srv := newSyncServer(t)
	srv.uploads = true
	home, drafts := imageEnv(t, srv)
	img := filepath.Join(home, "inbox", "hinge.jpg")
	writeFile(t, img, "jpg bytes")
	p := filepath.Join(drafts, "gate.md")
	writeFile(t, p, "---\nboard: Home\n---\n# Fix the gate\n\nSwap the hinge.\n\n![a](../hinge.jpg)\n")
	if r := runGo([]string{"draft", "apply", p}); r.code != 0 {
		t.Fatalf("create: %+v", r)
	}
	var mu sync.Mutex
	opens := 0
	old := openImage
	t.Cleanup(func() { openImage = old })
	openImage = func(path string) (*os.File, error) {
		mu.Lock()
		opens++
		mu.Unlock()
		return old(path)
	}
	if r := runGo([]string{"draft", "apply", "--update", p}); r.code != 0 {
		t.Fatalf("update: %+v", r)
	}
	if opens != 1 {
		t.Errorf("the image was hashed %d times in one run, want 1", opens)
	}
}
