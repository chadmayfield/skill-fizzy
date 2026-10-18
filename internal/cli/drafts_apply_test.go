package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRefusesBadCardValue(t *testing.T) {
	for _, value := range []string{"87abc", "87.0", "+87", "-5", "99999999999999999999"} {
		t.Run(value, func(t *testing.T) {
			srv := newSyncServer(t)
			path := filepath.Join(t.TempDir(), "gate.md")
			text := strings.Replace(syncDraftText, "card: 87\n", "card: "+value+"\n", 1)
			writeFile(t, path, srv.draft(t, text))
			before := syncReadDraft(t, path)
			for _, args := range [][]string{
				{"draft", "apply", path},
				{"draft", "apply", "--update", path},
			} {
				got := syncRun(t, srv, args...)
				if got.code != 2 || !strings.Contains(got.stderr, "front matter line 'card: "+value+"'") {
					t.Errorf("%q: code %d stderr %q", args, got.code, got.stderr)
				}
				if w := srv.writes(); len(w) != 0 {
					t.Errorf("%q: writes reached the server: %q", args, w)
				}
				if syncReadDraft(t, path) != before {
					t.Errorf("%q: the draft changed", args)
				}
			}
		})
	}
}

func TestApplyUpdateAcceptsHashCardAndKeepsTheLine(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, strings.Replace(syncDraftText, "card: 87\n", "card: #87\n", 1)))
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if !syncHas(srv.writes(), "PUT /1/cards/87") {
		t.Errorf("card #87 was not updated: %q", srv.writes())
	}
	for _, w := range srv.writes() {
		if strings.HasPrefix(w, "POST /1/boards/") {
			t.Errorf("a new card was created: %q", srv.writes())
		}
	}
	if text := syncReadDraft(t, path); !strings.Contains(text, "\ncard: #87\n") {
		t.Errorf("the card: line was rewritten:\n%s", text)
	}
}

func TestApplyRefusesBadGoldenValue(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, "---\nboard: Home\ngolden: maybe\n---\n# Oil the hinge\n")
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 2 || !strings.Contains(got.stderr, "front matter line 'golden: maybe'") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

// creates counts the card creates the server saw.
func (s *syncServer) creates() int {
	n := 0
	for _, w := range s.writes() {
		if strings.HasPrefix(w, "POST /1/boards/") && strings.HasSuffix(w, "/cards") {
			n++
		}
	}
	return n
}

func TestApplyNamesOneDraftOnce(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "hinge.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Oil the hinge\n")
	link := filepath.Join(t.TempDir(), "alias.md")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	got := syncRun(t, srv, "draft", "apply", dir+"/", path, dir+"/./hinge.md", link)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if n := srv.creates(); n != 1 {
		t.Errorf("%d creates for one draft: %q", n, srv.writes())
	}
}

const hingeDraft = "---\nboard: Home\ncolumn: Now\ntags: oil\n---\n# Oil the hinge\n\nOnce a year.\n\n## Steps\n- [ ] Buy oil\n"

// TestApplyFailedStepThenRetry is the duplicate-card case: a 500 on the
// first step, then a retry. Exactly one card is created, and the retry
// with --update finishes the steps.
func TestApplyFailedStepThenRetry(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, hingeDraft)
	srv.failNext("POST /1/cards/90/steps", 1)
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 1 {
		t.Fatalf("exit %d, want 1: %s", got.code, got.stderr)
	}
	for _, want := range []string{"card #90", "adding its steps failed", "fz draft apply --update " + path} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, got.stderr)
		}
	}
	text := syncReadDraft(t, path)
	if !strings.Contains(text, "\ncard: 90\n") || !strings.Contains(text, "\nsynced: sha256:") {
		t.Fatalf("draft after the failed apply:\n%s", text)
	}
	// Nothing after the failed step was sent.
	for _, w := range srv.writes() {
		if strings.Contains(w, "/taggings") || strings.Contains(w, "/triage") {
			t.Errorf("sent %q after the failed step", w)
		}
	}

	again := syncRun(t, srv, "draft", "apply", path)
	if again.code != 0 || !strings.Contains(again.stdout, "skip hinge.md: already card #90") {
		t.Errorf("plain retry: %+v", again)
	}
	retry := syncRun(t, srv, "draft", "apply", "--update", path)
	if retry.code != 0 {
		t.Fatalf("retry with --update: exit %d: %s", retry.code, retry.stderr)
	}
	if n := srv.creates(); n != 1 {
		t.Errorf("%d creates, want 1: %q", n, srv.writes())
	}
	steps := 0
	for _, w := range srv.writes() {
		if w == "POST /1/cards/90/steps" {
			steps++
		}
	}
	if steps != 2 || !strings.Contains(retry.stdout, "#90 step added: Buy oil") {
		t.Errorf("step posts = %d, stdout %q", steps, retry.stdout)
	}
}

func TestApplyFailureAfterCreateNamesTheCard(t *testing.T) {
	for call, what := range map[string]string{
		"POST /1/cards/90/taggings": "adding its tags failed",
		"POST /1/cards/90/triage":   "moving it to Now failed",
		"POST /1/cards/90/goldness": "marking it golden failed",
	} {
		t.Run(call, func(t *testing.T) {
			srv := newSyncServer(t)
			srv.edit(func(c *cardState) {})
			path := filepath.Join(t.TempDir(), "hinge.md")
			writeFile(t, path, strings.Replace(hingeDraft, "tags: oil\n", "tags: oil\ngolden: true\n", 1))
			srv.mu.Lock()
			srv.columns = `[{"id":"c-now","name":"Now"}]`
			srv.mu.Unlock()
			srv.failNext(call, 1)
			got := syncRun(t, srv, "draft", "apply", path)
			if got.code != 1 || !strings.Contains(got.stderr, "card #90") || !strings.Contains(got.stderr, what) ||
				!strings.Contains(got.stderr, "fz draft apply --update "+path) {
				t.Errorf("code %d stderr %q", got.code, got.stderr)
			}
			if !strings.Contains(syncReadDraft(t, path), "\ncard: 90\n") {
				t.Errorf("no card: line")
			}
		})
	}
}

func TestApplyRefusesUnwritableDraft(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "hinge.md")
	writeFile(t, path, hingeDraft)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 1 || !strings.Contains(got.stderr, "hinge.md") || !strings.Contains(got.stderr, "cannot write card:") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}

	// A read-only folder blocks the temp file the rewrite needs.
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(locked, "gate.md")
	writeFile(t, other, hingeDraft)
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	ok := filepath.Join(dir, "latch.md")
	writeFile(t, ok, "---\nboard: Home\n---\n# Fix the latch\n")
	got = syncRun(t, srv, "draft", "apply", ok, other)
	if got.code != 1 || !strings.Contains(got.stderr, "cannot write card:") {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestApplyNoLocationIsAnError(t *testing.T) {
	srv := newSyncServer(t)
	srv.mu.Lock()
	srv.noLocation = true
	srv.mu.Unlock()
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, hingeDraft)
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 1 || !strings.Contains(got.stderr, "no card number") || strings.Contains(got.stdout+got.stderr, "dry-run") {
		t.Errorf("code %d stdout %q stderr %q", got.code, got.stdout, got.stderr)
	}
	if w := srv.writes(); len(w) != 1 || !strings.HasSuffix(w[0], "/cards") {
		t.Errorf("writes after the create: %q", w)
	}
}

func TestApplyBatchStopsAndSummarizes(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\nboard: Home\n---\n# Fix the gate\n")
	writeFile(t, filepath.Join(dir, "b.md"), hingeDraft)
	writeFile(t, filepath.Join(dir, "c.md"), "---\nboard: Home\n---\n# Fix the latch\n")
	srv.failNext("POST /1/cards/90/steps", 1)
	got := syncRun(t, srv, "draft", "apply", dir)
	if got.code != 1 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	for _, want := range []string{"a.md: card #90\n", "b.md: card #90, not finished\n", "c.md: not reached\n"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("summary lacks %q:\n%s", want, got.stderr)
		}
	}
	if n := srv.creates(); n != 2 {
		t.Errorf("%d creates in the first run, want 2", n)
	}
	if again := syncRun(t, srv, "draft", "apply", dir); again.code != 0 {
		t.Fatalf("second run: %+v", again)
	}
	if n := srv.creates(); n != 3 {
		t.Errorf("%d creates for 3 drafts, want 3: %q", n, srv.writes())
	}
}

// TestApplyUpdatePartialFailure checks a --update that fails after some
// writes went through: those writes are printed, synced: is recorded from
// a fresh read, and the next --update needs no --force and sends the rest.
func TestApplyUpdatePartialFailure(t *testing.T) {
	srv := newSyncServer(t)
	srv.afterWrite = map[string]func(*cardState){
		"PUT /1/cards/87": func(c *cardState) { c.desc = "<p>Gate latches.</p><p>Sent by fz.</p>" },
	}
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	srv.failNext("POST /1/cards/87/steps", 1)
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 1 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if got.stdout != "#87 title and description sent\n" {
		t.Errorf("stdout = %q", got.stdout)
	}
	for _, want := range []string{"card #87", "HTTP 500", "fz draft apply --update " + path} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, got.stderr)
		}
	}
	if text := syncReadDraft(t, path); !strings.Contains(text, "\nsynced: "+srv.fingerprint(t)+"\n") {
		t.Errorf("synced: is not the fresh fingerprint %s:\n%s", srv.fingerprint(t), text)
	}
	retry := syncRun(t, srv, "draft", "apply", "--update", path)
	if retry.code != 0 || !strings.Contains(retry.stdout, "#87 step added: Order the hinge\n") {
		t.Errorf("retry without --force: %+v", retry)
	}
}

// TestApplyUpdateRefusalRecordsNothing checks that a refusal before any
// write never re-records synced:, which would turn the guard off.
func TestApplyUpdateRefusalRecordsNothing(t *testing.T) {
	srv := newSyncServer(t)
	srv.columns = `[{"id":"c-done","name":"Done"}]`
	path := filepath.Join(t.TempDir(), "gate.md")
	text := srv.draft(t, strings.Replace(syncDraftText, "column: Maybe?\n", "column: Done\n", 1))
	writeFile(t, path, text)
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 2 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes: %q", w)
	}
	if syncReadDraft(t, path) != text {
		t.Error("a refused update changed the draft")
	}
}

func TestApplyHelpDoesNotPairPruneWithForce(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"draft", "apply"})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(cmd.Example, "\n") {
		if strings.Contains(line, "--prune") && strings.Contains(line, "--force") {
			t.Errorf("example pairs --prune with --force: %q", line)
		}
	}
}
