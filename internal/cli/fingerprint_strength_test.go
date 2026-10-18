package cli

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// oldSyncedFields is st's synced-fields: value as fz wrote it before the
// title and description hashes grew to 16 hex characters.
func oldSyncedFields(t *testing.T, card string) string {
	t.Helper()
	return regexp.MustCompile(`(title|description)=([0-9a-f]{8})[0-9a-f]{8}`).ReplaceAllString(syncedFields(fpDecode(t, card)), "$1=$2")
}

// A synced-fields: line in the old 8-hex form still names what changed.
func TestChangedFieldsReadsOldForm(t *testing.T) {
	stored := oldSyncedFields(t, fpCard)
	if !regexp.MustCompile(`^title=[0-9a-f]{8} description=[0-9a-f]{8} `).MatchString(stored) {
		t.Fatalf("old form: %q", stored)
	}
	if got := changedFields(stored, fpDecode(t, fpCard)); len(got) != 0 {
		t.Errorf("unchanged card, old form: %q", got)
	}
	changed := strings.Replace(fpCard, `"title":"Fix the gate"`, `"title":"Fix the gate now"`, 1)
	if got := strings.Join(changedFields(stored, fpDecode(t, changed)), ", "); got != "title" {
		t.Errorf("changed title, old form: %q", got)
	}
}

// The skip needs the 16-hex title and description hashes. A line in the
// old 8-hex form never skips, so the next --update sends once.
func TestTextSyncedNeedsLongHashes(t *testing.T) {
	c := fpDecode(t, fpCard)
	if !textSynced(syncedFields(c), c) {
		t.Errorf("new form does not match its own card")
	}
	if textSynced(oldSyncedFields(t, fpCard), c) {
		t.Errorf("old 8-hex form took the skip")
	}
}

// --update --force always sends the title and description, even when the
// draft and the card are as at the last sync.
func TestUpdateForceNeverSkips(t *testing.T) {
	srv := newSyncServer(t)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, syncDraftText))
	if r := syncRun(t, srv, "draft", "apply", "--update", "--force", path); r.code != 0 {
		t.Fatalf("full sync: %+v", r)
	}
	if r := syncRun(t, srv, "draft", "apply", "--update", path); r.code != 0 || !strings.Contains(r.stdout, "#87 title and description unchanged\n") {
		t.Fatalf("plain --update should skip: %+v", r)
	}
	from := len(srv.writes())
	r := syncRun(t, srv, "draft", "apply", "--update", "--force", path)
	if r.code != 0 || !strings.Contains(r.stdout, "#87 title and description sent\n") || countExact(srv.writes()[from:], "PUT /1/cards/87") != 1 {
		t.Errorf("--force skipped: %+v, writes %q", r, srv.writes()[from:])
	}
}

// A 16-hex hash on a field fz writes with 8 cannot be told, and does not
// crash.
func TestChangedFieldsLongHashOnShortField(t *testing.T) {
	stored := regexp.MustCompile(`steps=([0-9a-f]{8})`).ReplaceAllString(syncedFields(fpDecode(t, fpCard)), "steps=${1}00000000")
	if got := changedFields(stored, fpDecode(t, fpCard)); got != nil {
		t.Errorf("got %q, want nil", got)
	}
}
