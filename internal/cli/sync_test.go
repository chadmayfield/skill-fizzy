package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
synced: FINGERPRINT
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

// card90 is the card the fake server creates.
const card90 = `{"number":90,"title":"Oil the hinge","board":{"id":"b1","name":"Home"},"description_html":"<p>Once a year.</p>","last_active_at":"2026-10-17T12:00:00Z","tags":[],"steps":[]}`

// cardState is what the fake server reports for cards 87 and 88. Tests
// change it to stand for an edit made in Fizzy.
type cardState struct {
	title, desc, tags, column string
	golden, measureDone       bool
	postponed                 bool
}

func defaultCardState() cardState {
	return cardState{title: "Fix the gate", desc: "<p>Gate latches.</p>", tags: `["buy"]`, column: "null"}
}

// syncCard is card n as the fake server returns it.
func syncCard(n int, st cardState) string {
	return `{"number":` + strconv.Itoa(n) + `,"title":` + strconv.Quote(st.title) + `,"board":{"id":"b1","name":"Home"},"closed":false,` +
		`"golden":` + strconv.FormatBool(st.golden) + `,"postponed":` + strconv.FormatBool(st.postponed) + `,"description_html":` + strconv.Quote(st.desc) + `,` +
		`"tags":` + st.tags + `,"last_active_at":"2026-10-17T10:00:00Z","column":` + st.column + `,"steps":[` +
		`{"id":"` + stepMeasure + `","content":"Measure the gap","completed":` + strconv.FormatBool(st.measureDone) + `},` +
		`{"id":"` + stepOld + `","content":"Old step","completed":false},` +
		`{"id":"` + stepDone + `","content":"Done elsewhere","completed":true}]}`
}

// syncServer serves cards 87 and 88 (same content) and 90, and the boards
// list, accepts every write, and records each request as "METHOD /path".
// Writes do not change what it serves.
type syncServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls []string
	state cardState
}

func newSyncServer(t *testing.T) *syncServer {
	t.Helper()
	s := &syncServer{state: defaultCardState()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /1/cards/{n}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("n") {
		case "87", "88":
			n, _ := strconv.Atoi(r.PathValue("n"))
			s.mu.Lock()
			st := s.state
			s.mu.Unlock()
			io.WriteString(w, syncCard(n, st))
		case "90":
			io.WriteString(w, card90)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("GET /1/boards", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"id":"b1","name":"Home"},{"id":"b2","name":"Garage"}]`)
	})
	mux.HandleFunc("POST /1/rails/active_storage/direct_uploads", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{}`)
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

// edit changes the served card, the way an edit in Fizzy would.
func (s *syncServer) edit(f func(*cardState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.state)
}

// fingerprintOf decodes card JSON the way the client does and fingerprints it.
func fingerprintOf(t *testing.T, cardJSON string) string {
	t.Helper()
	var c fizzy.Card
	if err := json.Unmarshal([]byte(cardJSON), &c); err != nil {
		t.Fatal(err)
	}
	return cardFingerprint(c)
}

// fingerprint is the fingerprint of the card the server serves now. It reads
// the state directly, so it adds no request to calls.
func (s *syncServer) fingerprint(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	return fingerprintOf(t, syncCard(87, st))
}

// draft fills a FINGERPRINT placeholder in text with the served card's
// fingerprint, so the draft is in sync.
func (s *syncServer) draft(t *testing.T, text string) string {
	t.Helper()
	return strings.Replace(text, "FINGERPRINT", s.fingerprint(t), 1)
}

// Fizzy-side edits that move last_active_at on the server (title) and ones
// that do not (the rest). The guard must catch both.
var fizzyEdits = map[string]func(*cardState){
	"description": func(c *cardState) { c.desc = "<p>Gate latches. Oiled in Fizzy.</p>" },
	"step tick":   func(c *cardState) { c.measureDone = true },
	"tag":         func(c *cardState) { c.tags = `["buy","hinge"]` },
	"golden":      func(c *cardState) { c.golden = true },
	"title":       func(c *cardState) { c.title = "Fix the gate today" },
	"column":      func(c *cardState) { c.column = `{"id":"c1","name":"Next","color":{"name":"Blue","value":"#00f"}}` },
	// Maybe? and Not now both report column null; only postponed differs.
	"not now": func(c *cardState) { c.postponed = true },
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

// syncFixture writes text as gate.md, with any FINGERPRINT placeholder
// filled so the draft is in sync with the served card.
func syncFixture(t *testing.T, text string) (*syncServer, *fizzy.Fizzy, *config.Config, *draft.Draft) {
	t.Helper()
	srv := newSyncServer(t)
	dir := t.TempDir()
	cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T", Inbox: dir, LogPath: filepath.Join(dir, "actions.log")}
	path := filepath.Join(dir, "gate.md")
	writeFile(t, path, srv.draft(t, text))
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
	srv, fz, cfg, d := syncFixture(t, syncDraftText)
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
	srv, fz, cfg, d := syncFixture(t, syncDraftText)
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

func TestSyncRefusesFizzySideEdits(t *testing.T) {
	for name, edit := range fizzyEdits {
		t.Run(name, func(t *testing.T) {
			srv, fz, cfg, d := syncFixture(t, syncDraftText)
			srv.edit(edit)
			_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
			want := "gate.md: card #87 changed in Fizzy since the last sync; check it, then rerun with --force"
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
			if w := srv.writes(); len(w) != 0 {
				t.Errorf("writes reached the server: %q", w)
			}
		})
	}
}

func TestSyncForceOverridesChangedCard(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, syncDraftText)
	srv.edit(fizzyEdits["description"])
	if _, err := syncDraft(fz, d, cfg.InboxPath(), false, true); err != nil {
		t.Fatal(err)
	}
	if !syncHas(srv.writes(), "PUT /1/cards/87") {
		t.Errorf("--force sent no update: %q", srv.writes())
	}
}

func TestSyncMissingSyncedNeedsForce(t *testing.T) {
	text := strings.Replace(syncDraftText, "synced: FINGERPRINT\n", "", 1)
	srv, fz, cfg, d := syncFixture(t, text)
	_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	want := "gate.md: no synced: line, so changes made in Fizzy cannot be detected; rerun with --force"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestSyncUnknownSyncedNeedsForce(t *testing.T) {
	for value, want := range map[string]string{
		"2026-10-17T10:00:00Z":     "gate.md: synced: is a time written by an older fz, so changes made in Fizzy cannot be detected; rerun with --force",
		"2026-10-17T10:00:00.000Z": "gate.md: synced: is a time written by an older fz, so changes made in Fizzy cannot be detected; rerun with --force",
		"sha256:12345":             "gate.md: synced: is not a card fingerprint, so changes made in Fizzy cannot be detected; rerun with --force",
		"yesterday":                "gate.md: synced: is not a card fingerprint, so changes made in Fizzy cannot be detected; rerun with --force",
	} {
		text := strings.Replace(syncDraftText, "FINGERPRINT", value, 1)
		srv, fz, cfg, d := syncFixture(t, text)
		_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
		if err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", value, err, want)
		}
		if w := srv.writes(); len(w) != 0 {
			t.Errorf("%s: writes reached the server: %q", value, w)
		}
		if _, err := syncDraft(fz, d, cfg.InboxPath(), false, true); err != nil {
			t.Errorf("%s: --force: %v", value, err)
		}
	}
}

func TestSyncPruneRemovesMissingTags(t *testing.T) {
	text := strings.Replace(syncDraftText, "tags: buy\n", "tags: hinge\n", 1)
	srv, fz, cfg, d := syncFixture(t, text)
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
	srv, fz, cfg, d := syncFixture(t, text)
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

func TestSyncLaneTellsNotNowFromMaybe(t *testing.T) {
	for _, tc := range []struct {
		name      string
		column    string
		postponed bool
		move      string // the write expected, or "" for none
	}{
		{"not now stays", "Not now", true, ""},
		{"not-now spelling stays", "not-now", true, ""},
		{"maybe to not now", "Not now", false, "POST /1/cards/87/not_now"},
		{"not now to maybe", "Maybe?", true, "DELETE /1/cards/87/triage"},
		{"maybe stays", "Maybe?", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSyncServer(t)
			srv.edit(func(c *cardState) { c.postponed = tc.postponed })
			dir := t.TempDir()
			cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T", Inbox: dir, LogPath: filepath.Join(dir, "actions.log")}
			path := filepath.Join(dir, "gate.md")
			text := strings.Replace(syncDraftText, "column: Maybe?\n", "column: "+tc.column+"\n", 1)
			writeFile(t, path, srv.draft(t, text))
			d, err := draft.Parse(path)
			if err != nil {
				t.Fatal(err)
			}
			fz := &fizzy.Fizzy{C: client.New(cfg, false, io.Discard)}
			report, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
			if err != nil {
				t.Fatal(err)
			}
			var moves []string
			for _, w := range srv.writes() {
				if strings.HasSuffix(w, "/not_now") || strings.HasSuffix(w, "/triage") {
					moves = append(moves, w)
				}
			}
			moved := false
			for _, line := range report {
				moved = moved || strings.HasPrefix(line, "moved to ")
			}
			if tc.move == "" {
				if len(moves) != 0 || moved {
					t.Errorf("sent a move for a card already there: %q, report %q", moves, report)
				}
			} else if len(moves) != 1 || moves[0] != tc.move || !moved {
				t.Errorf("moves = %q, want [%q]; report %q", moves, tc.move, report)
			}
		})
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
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	// An old timestamp, so the first run needs --force and the record step
	// has to replace it.
	writeFile(t, path, strings.Replace(syncDraftText, "FINGERPRINT", "2026-10-01T00:00:00Z", 1))
	got := syncRun(t, srv, "draft", "apply", "--update", "--force", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	if !strings.Contains(got.stdout, "#87 title and description sent\n") || !strings.Contains(got.stdout, "#87 step added: Order the hinge\n") {
		t.Errorf("stdout = %q", got.stdout)
	}
	text := syncReadDraft(t, path)
	if strings.Count(text, "synced:") != 1 || !strings.Contains(text, "synced: "+srv.fingerprint(t)+"\n") {
		t.Errorf("draft after update:\n%s", text)
	}

	// Nothing changed in Fizzy, so the next update needs no --force.
	if got := syncRun(t, srv, "draft", "apply", "--update", path); got.code != 0 {
		t.Fatalf("second update: exit %d: %s", got.code, got.stderr)
	}

	// A description edit in Fizzy, which leaves last_active_at alone, blocks
	// the one after, and the refusal names it from synced-fields:.
	srv.edit(fizzyEdits["description"])
	got = syncRun(t, srv, "draft", "apply", "--update", path)
	want := "fz: gate.md: card #87 changed in Fizzy since the last sync (description); check it, then rerun with --force\n"
	if got.code != 1 || got.stderr != want {
		t.Errorf("after a Fizzy edit: code %d stderr %q", got.code, got.stderr)
	}
}

func TestApplyUpdatePreflightRefusesFizzySideEdits(t *testing.T) {
	for _, name := range []string{"description", "step tick", "tag", "golden", "not now"} {
		t.Run(name, func(t *testing.T) {
			srv := newSyncServer(t)
			path := filepath.Join(t.TempDir(), "gate.md")
			text := srv.draft(t, syncDraftText)
			writeFile(t, path, text)
			srv.edit(fizzyEdits[name])
			got := syncRun(t, srv, "draft", "apply", "--update", path)
			want := "fz: gate.md: card #87 changed in Fizzy since the last sync; check it, then rerun with --force\n"
			if got.code != 1 || got.stderr != want || got.stdout != "" {
				t.Errorf("code %d stdout %q stderr %q", got.code, got.stdout, got.stderr)
			}
			if w := srv.writes(); len(w) != 0 {
				t.Errorf("writes reached the server: %q", w)
			}
			if syncReadDraft(t, path) != text {
				t.Errorf("a refused update changed the draft")
			}
		})
	}
}

func TestApplyUpdateDryRunWritesNothing(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	text := strings.Replace(syncDraftText, "synced: FINGERPRINT\n", "", 1)
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
	for _, line := range strings.Split(strings.TrimSpace(got.stdout), "\n") {
		if !strings.HasPrefix(line, "#87 dry-run: nothing sent; would ") && !strings.Contains(line, " kept, not in draft: ") && !strings.Contains(line, "column left alone") {
			t.Errorf("dry-run line claims a change: %q", line)
		}
	}
	if !strings.Contains(got.stdout, "#87 dry-run: nothing sent; would send the title and description\n") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestApplyWithoutUpdateSkips(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	got := syncRun(t, srv, "draft", "apply", path)
	if want := "skip gate.md: already card #87 (use --update to push changes)\n"; got.code != 0 || got.stdout != want {
		t.Errorf("got code %d stdout %q, want %q", got.code, got.stdout, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestApplyNewCardRecordsCardAndSynced(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "hinge.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n")
	got := syncRun(t, srv, "draft", "apply", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	var c90 fizzy.Card
	if err := json.Unmarshal([]byte(card90), &c90); err != nil {
		t.Fatal(err)
	}
	want := "---\nboard: Home\ncard: 90\nsynced: " + fingerprintOf(t, card90) + "\nsynced-fields: " + syncedFields(c90) +
		"\n---\n# Oil the hinge\n\nOnce a year.\n"
	if text := syncReadDraft(t, path); text != want {
		t.Errorf("draft after apply:\n%q\nwant\n%q", text, want)
	}
}

func TestApplyPruneAndForceNeedUpdate(t *testing.T) {
	srv := newSyncServer(t)
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

func TestSyncPruneKeepsTagWrittenDifferently(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, syncDraftText)
	// "#Buy" normalizes to the same tag as "buy", so the fingerprint, and
	// with it the guard, does not see a change.
	srv.edit(func(c *cardState) { c.tags = `["#Buy"]` })
	report, err := syncDraft(fz, d, cfg.InboxPath(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range report {
		if strings.HasPrefix(line, "tag removed") {
			t.Errorf("pruned a tag the draft has: %q", report)
		}
	}
	// One GET for the sync and one for AddTag. A third would be RemoveTag
	// reading the card before toggling "#Buy" off.
	srv.mu.Lock()
	gets := 0
	for _, c := range srv.calls {
		if c == "GET /1/cards/87" {
			gets++
		}
	}
	srv.mu.Unlock()
	if gets != 2 {
		t.Errorf("GET /1/cards/87 sent %d times, want 2: prune tried to remove a tag the draft has", gets)
	}
}

// conflictDraft is syncDraftText for card n with a synced: fingerprint the
// fake server's card does not have.
func conflictDraft(n int) string {
	return strings.NewReplacer("card: 87", "card: "+strconv.Itoa(n), "FINGERPRINT", "sha256:0000000000000000").Replace(syncDraftText)
}

func TestApplyUpdateReportsEveryConflictAndWritesNothing(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), conflictDraft(87))
	writeFile(t, filepath.Join(dir, "b.md"), conflictDraft(88))
	got := syncRun(t, srv, "draft", "apply", "--update", dir)
	want := "fz: a.md: card #87 changed in Fizzy since the last sync; check it, then rerun with --force\n" +
		"fz: b.md: card #88 changed in Fizzy since the last sync; check it, then rerun with --force\n"
	if got.code != 1 || got.stderr != want || got.stdout != "" {
		t.Errorf("code %d\nstdout %q\nstderr %q\nwant stderr %q", got.code, got.stdout, got.stderr, want)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestApplyUpdateConflictBlocksNewCards(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n")
	text := strings.Replace(syncDraftText, "synced: FINGERPRINT\n", "", 1)
	writeFile(t, filepath.Join(dir, "b.md"), text)
	got := syncRun(t, srv, "draft", "apply", "--update", dir)
	want := "fz: b.md: no synced: line, so changes made in Fizzy cannot be detected; rerun with --force\n"
	if got.code != 1 || got.stderr != want {
		t.Errorf("code %d stderr %q", got.code, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
	if strings.Contains(syncReadDraft(t, filepath.Join(dir, "a.md")), "card:") {
		t.Errorf("the new draft was recorded as created")
	}
}

func TestApplyUpdateConflictDryRunWritesNothing(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\nboard: Home\n---\n# Oil the hinge\n\nOnce a year.\n")
	writeFile(t, filepath.Join(dir, "b.md"), conflictDraft(87))
	got := syncRun(t, srv, "--dry-run", "draft", "apply", "--update", dir)
	if got.code != 1 || strings.Contains(got.stderr, "[dry-run]") || got.stdout != "" {
		t.Errorf("code %d stdout %q stderr %q", got.code, got.stdout, got.stderr)
	}
	if w := srv.writes(); len(w) != 0 {
		t.Errorf("writes reached the server: %q", w)
	}
}

func TestApplyUpdateForceSkipsGuardForAll(t *testing.T) {
	srv := newSyncServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), conflictDraft(87))
	writeFile(t, filepath.Join(dir, "b.md"), conflictDraft(88))
	got := syncRun(t, srv, "draft", "apply", "--update", "--force", dir)
	if got.code != 0 {
		t.Fatalf("code %d stderr %q", got.code, got.stderr)
	}
	w := srv.writes()
	if !syncHas(w, "PUT /1/cards/87") || !syncHas(w, "PUT /1/cards/88") {
		t.Errorf("writes = %q", w)
	}
}

// After an apply has recorded synced-fields:, a refusal names what changed.
func TestApplyUpdateNamesChangedFields(t *testing.T) {
	for edit, want := range map[string]string{
		"description": "description",
		"step tick":   "steps",
		"tag":         "tags",
		"golden":      "golden",
		"title":       "title",
		"column":      "column",
		"not now":     "postponed",
	} {
		t.Run(edit, func(t *testing.T) {
			srv := newSyncServer(t)
			path := filepath.Join(t.TempDir(), "gate.md")
			writeFile(t, path, srv.draft(t, syncDraftText))
			if got := syncRun(t, srv, "draft", "apply", "--update", path); got.code != 0 {
				t.Fatalf("first update: %+v", got)
			}
			text := syncReadDraft(t, path)
			if strings.Count(text, "\nsynced-fields: ") != 1 {
				t.Fatalf("no synced-fields: line:\n%s", text)
			}
			srv.edit(fizzyEdits[edit])
			got := syncRun(t, srv, "draft", "apply", "--update", path)
			msg := "fz: gate.md: card #87 changed in Fizzy since the last sync (" + want + "); check it, then rerun with --force\n"
			if got.code != 1 || got.stderr != msg {
				t.Errorf("code %d stderr %q\nwant %q", got.code, got.stderr, msg)
			}
		})
	}
}

func TestSyncFailedUploadWritesNothingElse(t *testing.T) {
	text := strings.Replace(syncDraftText, "board: Home", "board: Garage", 1)
	text = strings.Replace(text, "Gate latches.\n", "Gate latches.\n\n![the gap](gap.png)\n", 1)
	srv, fz, cfg, d := syncFixture(t, text)
	writeFile(t, filepath.Join(filepath.Dir(d.Path), "gap.png"), "png")
	_, err := syncDraft(fz, d, cfg.InboxPath(), false, false)
	if err == nil {
		t.Fatal("an incomplete upload reply did not fail the sync")
	}
	if !strings.Contains(err.Error(), "direct upload reply has no direct_upload.url") {
		t.Errorf("error = %q, want the incomplete-reply error", err)
	}
	w := srv.writes()
	if len(w) != 1 || w[0] != "POST /1/rails/active_storage/direct_uploads" {
		t.Errorf("writes other than the upload: %q", w)
	}
}

func TestSyncReportsExtraCardTagWithoutPrune(t *testing.T) {
	srv, fz, cfg, d := syncFixture(t, syncDraftText)
	srv.edit(func(st *cardState) { st.tags = `["buy","extra"]` })
	report, err := syncDraft(fz, d, cfg.InboxPath(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !syncHas(report, "tag kept, not in draft: extra") {
		t.Errorf("report does not mention the extra tag: %q", report)
	}
	if syncHas(report, "tag kept, not in draft: buy") {
		t.Errorf("reported a draft tag as extra: %q", report)
	}
}
