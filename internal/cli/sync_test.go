package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/draft"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

const syncDraftText = `---
board: Home
column: Maybe?
tags: buy
card: 87
synced: 2026-10-17T10:00:00Z
---
# Fix the gate

## Goal
Gate latches.

## Steps
- [x] Measure the gap
- [ ] Order the hinge
`

const (
	stepMeasure = "03abc123def456ghi789jkl0mn"
	stepOld     = "03old000000000000000000000"
	stepDone    = "03done00000000000000000000"
)

// syncCard is card 87 as the fake server returns it. lastActive replaces
// its last_active_at.
func syncCard(lastActive string) string {
	return `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},"closed":false,"golden":false,` +
		`"tags":["buy"],"last_active_at":"` + lastActive + `","column":null,"steps":[` +
		`{"id":"` + stepMeasure + `","content":"Measure the gap","completed":false},` +
		`{"id":"` + stepOld + `","content":"Old step","completed":false},` +
		`{"id":"` + stepDone + `","content":"Done elsewhere","completed":true}]}`
}

// syncServer serves card 87 and the boards list, accepts every write, and
// records each request as "METHOD /path".
type syncServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string
}

func newSyncServer(t *testing.T, lastActive string) *syncServer {
	t.Helper()
	s := &syncServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /1/cards/{n}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("n") {
		case "87":
			io.WriteString(w, syncCard(lastActive))
		case "90":
			io.WriteString(w, `{"number":90,"title":"Oil the hinge","board":{"id":"b1","name":"Home"},"last_active_at":"2026-10-17T12:00:00Z","tags":[],"steps":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("GET /1/boards", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
	})
	mux.HandleFunc("POST /1/boards/{id}/cards", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", s.URL+"/1/cards/90.json")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /1/cards/{n}/steps", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", s.URL+"/1/cards/87/steps/03new000000000000000000000.json")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls = append(s.calls, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// writes returns the non-GET requests the server saw.
func (s *syncServer) writes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, c := range s.calls {
		if !strings.HasPrefix(c, "GET ") {
			out = append(out, c)
		}
	}
	return out
}

func syncFixture(t *testing.T, lastActive, text string) (*syncServer, *fizzy.Fizzy, *config.Config, *draft.Draft) {
	t.Helper()
	srv := newSyncServer(t, lastActive)
	dir := t.TempDir()
	cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T", Inbox: dir, LogPath: filepath.Join(dir, "actions.log")}
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, text)
	d, err := draft.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return srv, &fizzy.Fizzy{C: client.New(cfg, false, io.Discard)}, cfg, d
}

func syncHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestSyncAddsTicksAndKeepsExtras(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T10:00:00Z", syncDraftText)
	report, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	w := srv.writes()
	for _, want := range []string{"PUT /1/cards/87", "POST /1/cards/87/steps", "PUT /1/cards/87/steps/" + stepMeasure} {
		if !syncHas(w, want) {
			t.Errorf("missing write %q in %q", want, w)
		}
	}
	if syncHas(w, "DELETE /1/cards/87/steps/"+stepOld) {
		t.Errorf("deleted a step without --prune: %q", w)
	}
	if !syncHas(report, "step kept, not in draft: Old step") {
		t.Errorf("report does not mention the extra step: %q", report)
	}
}

func TestSyncPruneDeletesOnlyUnticked(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T10:00:00Z", syncDraftText)
	if _, err := syncDraft(fz, d, cfg.InboxPath(), true, false); err != nil {
		t.Fatal(err)
	}
	var deletes []string
	for _, c := range srv.writes() {
		if strings.HasPrefix(c, "DELETE ") {
			deletes = append(deletes, c)
		}
	}
	if want := []string{"DELETE /1/cards/87/steps/" + stepOld}; strings.Join(deletes, ",") != strings.Join(want, ",") {
		t.Errorf("deletes = %q, want %q", deletes, want)
	}
}

func TestSyncRefusesWhenCardChanged(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T11:00:00Z", syncDraftText)
	_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	want := "gate.md: card #87 changed in Fizzy since the last sync; check it, then rerun with --force"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestSyncForceOverridesChangedCard(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T11:00:00Z", syncDraftText)
	if _, err := syncDraft(fz, d, cfg.InboxPath(), false, true); err != nil {
		t.Fatal(err)
	}
	if !syncHas(srv.writes(), "PUT /1/cards/87") {
		t.Errorf("--force sent no update: %q", srv.writes())
	}
}

func TestSyncMissingSyncedNeedsForce(t *testing.T) {
	text := strings.Replace(syncDraftText, "synced: 2026-10-17T10:00:00Z\n", "", 1)
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T10:00:00Z", text)
	_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	want := "gate.md: no synced: line, so changes made in Fizzy cannot be detected; rerun with --force"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestSyncPruneRemovesMissingTags(t *testing.T) {
	text := strings.Replace(syncDraftText, "tags: buy\n", "tags: hinge\n", 1)
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T10:00:00Z", text)
	report, err := syncDraft(fz, d, cfg.InboxPath(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range srv.writes() {
		if c == "POST /1/cards/87/taggings" {
			n++
		}
	}
	// The fake card always reports "buy", so adding hinge and removing buy
	// are one toggle each.
	if n != 2 || !syncHas(report, "tag added: hinge") || !syncHas(report, "tag removed: buy") {
		t.Errorf("taggings = %d, report = %q", n, report)
	}
}

func TestSyncMovesColumnAndGolden(t *testing.T) {
	text := strings.Replace(syncDraftText, "column: Maybe?\n", "column: Not now\ngolden: true\n", 1)
	srv, fz, cfg, d := syncFixture(t, "2026-10-17T10:00:00Z", text)
	report, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	w := srv.writes()
	if !syncHas(w, "POST /1/cards/87/not_now") || !syncHas(w, "POST /1/cards/87/goldness") {
		t.Errorf("writes = %q", w)
	}
	if !syncHas(report, "moved to Not now") || !syncHas(report, "golden on") {
		t.Errorf("report = %q", report)
	}
}

// runApply runs fz against srv with the given arguments.
func syncRun(t *testing.T, srv *syncServer, args ...string) result {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"FIZZY_URL": srv.URL, "FIZZY_ACCOUNT": "1", "FIZZY_TOKEN": "T", "FIZZY_CONFIG": "/nonexistent",
		"FIZZY_TOKEN_CMD": "", "FIZZY_INBOX": filepath.Join(home, "inbox"), "FIZZY_RULES_CARD": "", "HOME": home,
	} {
		t.Setenv(k, v)
	}
	return runGo(args)
}

func syncReadDraft(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyUpdateRecordsSynced(t *testing.T) {
	srv := newSyncServer(t, "2026-10-17T10:00:00Z")
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, syncDraftText)
	got := syncRun(t, srv, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "#87 title and description sent\n") || !strings.Contains(got.stdout, "#87 step added: Order the hinge\n") {
		t.Errorf("stdout = %q", got.stdout)
	}
	text := syncReadDraft(t, path)
	if strings.Count(text, "synced:") != 1 || !strings.Contains(text, "synced: 2026-10-17T10:00:00Z\n") {
		t.Errorf("draft after update:\n%s", text)
	}
}

func TestApplyUpdateDryRunWritesNothing(t *testing.T) {
	srv := newSyncServer(t, "2026-10-17T10:00:00Z")
	path := filepath.Join(t.TempDir(), "gate.md")
	text := strings.Replace(syncDraftText, "synced: 2026-10-17T10:00:00Z\n", "", 1)
	writeFile(t, path, text)
	got := syncRun(t, srv, "--dry-run", "draft", "apply", "--update", "--force", "--prune", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("dry-run reached the server: %q", w)
	}
	for _, want := range []string{
		"[dry-run] PUT " + srv.URL + "/1/cards/87 ",
		"[dry-run] POST " + srv.URL + "/1/cards/87/steps ",
		"[dry-run] PUT " + srv.URL + "/1/cards/87/steps/" + stepMeasure + " ",
		"[dry-run] DELETE " + srv.URL + "/1/cards/87/steps/" + stepOld + " ",
	} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, got.stderr)
		}
	}
	if syncReadDraft(t, path) != text {
		t.Errorf("dry-run changed the draft")
	}
}

func TestApplyWithoutUpdateSkips(t *testing.T) {
	srv := newSyncServer(t, "2026-10-17T10:00:00Z")
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, syncDraftText)
	got := syncRun(t, srv, "draft", "apply", path)
	if want := "skip gate.md: already card #87 (use --update to push changes)\n"; got.code != 0 || got.stdout != want {
		t.Errorf("got code %d stdout %q, want %q", got.code, got.stdout, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestApplyNewCardRecordsCardAndSynced(t *testing.T) {
	srv := newSyncServer(t, "2026-10-17T10:00:00Z")
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n")
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	want := "---\nboard: Home\ncard: 90\nsynced: 2026-10-17T12:00:00Z\n---\n# Oil the hinge\n\nOnce a year.\n"
	if text := syncReadDraft(t, path); text != want {
		t.Errorf("draft after apply:\n%q\nwant\n%q", text, want)
	}
}

func TestApplyPruneAndForceNeedUpdate(t *testing.T) {
	srv := newSyncServer(t, "2026-10-17T10:00:00Z")
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, syncDraftText)
	for _, flag := range []string{"--prune", "--force"} {
		got := syncRun(t, srv, "draft", "apply", flag, path)
		if got.code != 2 || !strings.Contains(got.stderr, flag+" needs --update") {
			t.Errorf("%s: code %d stderr %q", flag, got.code, got.stderr)
		}
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.calls) != 0 {
		t.Errorf("a usage error reached the server: %q", srv.calls)
	}
}

func TestApplyHasUpdateFlags(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"draft", "apply"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"update", "prune", "force"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("draft apply has no --%s", f)
		}
	}
	if !strings.Contains(cmd.Example, "fz draft apply --update gate.md") {
		t.Errorf("example missing: %q", cmd.Example)
	}
	var b bytes.Buffer
	cmd.SetOut(&b)
	_ = cmd.Help()
	if !strings.Contains(b.String(), "--update") {
		t.Errorf("help does not list --update")
	}
}
