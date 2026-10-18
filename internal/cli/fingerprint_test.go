package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

const fpCard = `{"number":87,"title":"Fix the gate","board":{"id":"b1","name":"Home"},` +
	`"column":{"id":"c1","name":"Next"},"closed":false,"postponed":false,"golden":false,"tags":["buy","build"],` +
	`"description_html":"<p>Gate latches.</p>","last_active_at":"2026-10-17T10:00:00Z","steps":[` +
	`{"id":"03abc123def456ghi789jkl0mn","content":"Measure the gap","completed":false},` +
	`{"id":"03old000000000000000000000","content":"Order the hinge","completed":true}]}`

func fpDecode(t *testing.T, s string) fizzy.Card {
	t.Helper()
	var c fizzy.Card
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFingerprintFormatAndStable(t *testing.T) {
	a, b := cardFingerprint(fpDecode(t, fpCard)), cardFingerprint(fpDecode(t, fpCard))
	if a != b {
		t.Fatalf("same card, two fingerprints: %q %q", a, b)
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{16}$`).MatchString(a) {
		t.Errorf("fingerprint %q is not sha256: plus 16 hex characters", a)
	}
}

func TestFingerprintIgnoresKeyOrderAndUnwatchedFields(t *testing.T) {
	reordered := `{"steps":[` +
		`{"completed":false,"content":"Measure the gap","id":"03abc123def456ghi789jkl0mn"},` +
		`{"content":"Order the hinge","id":"03old000000000000000000000","completed":true}],` +
		`"tags":["buy","build"],"golden":false,"description_html":"<p>Gate latches.</p>",` +
		`"column":{"name":"Next","id":"c1"},"board":{"name":"Home","id":"b1"},"postponed":false,"closed":false,` +
		`"title":"Fix the gate","number":87,"last_active_at":"2026-10-18T09:00:00Z","url":"https://fizzy.example.com/1/cards/87"}`
	if a, b := cardFingerprint(fpDecode(t, fpCard)), cardFingerprint(fpDecode(t, reordered)); a != b {
		t.Errorf("key order or last_active_at changed the fingerprint: %q %q", a, b)
	}
}

func TestFingerprintNormalizesTags(t *testing.T) {
	other := strings.Replace(fpCard, `"tags":["buy","build"]`, `"tags":["build","#Buy"]`, 1)
	if a, b := cardFingerprint(fpDecode(t, fpCard)), cardFingerprint(fpDecode(t, other)); a != b {
		t.Errorf("tag order or spelling changed the fingerprint: %q %q", a, b)
	}
}

func TestFingerprintChangesWithEachField(t *testing.T) {
	base := cardFingerprint(fpDecode(t, fpCard))
	for name, edit := range map[string][2]string{
		"title":       {`"title":"Fix the gate"`, `"title":"Fix the gate now"`},
		"description": {`<p>Gate latches.</p>`, `<p>Gate latches shut.</p>`},
		"step tick":   {`"completed":false`, `"completed":true`},
		"step text":   {`"Measure the gap"`, `"Measure the gap twice"`},
		"step id":     {`03old000000000000000000000`, `03new000000000000000000000`},
		"step added":  {`"steps":[`, `"steps":[{"id":"03add000000000000000000000","content":"Oil it","completed":false},`},
		"tag added":   {`"tags":["buy","build"]`, `"tags":["buy","build","hinge"]`},
		"tag removed": {`"tags":["buy","build"]`, `"tags":["buy"]`},
		"golden":      {`"golden":false`, `"golden":true`},
		"column":      {`"column":{"id":"c1","name":"Next"}`, `"column":{"id":"c2","name":"Next"}`},
		"maybe":       {`"column":{"id":"c1","name":"Next"}`, `"column":null`},
		"board":       {`"board":{"id":"b1"`, `"board":{"id":"b2"`},
		"closed":      {`"closed":false`, `"closed":true`},
		"postponed":   {`"postponed":false`, `"postponed":true`},
	} {
		changed := strings.Replace(fpCard, edit[0], edit[1], 1)
		if changed == fpCard {
			t.Fatalf("%s: edit did not apply", name)
		}
		if got := cardFingerprint(fpDecode(t, changed)); got == base {
			t.Errorf("%s: fingerprint did not change", name)
		}
	}
	// Swapping two steps is a change too: the list is ordered.
	swapped := strings.Replace(fpCard,
		`{"id":"03abc123def456ghi789jkl0mn","content":"Measure the gap","completed":false},`+
			`{"id":"03old000000000000000000000","content":"Order the hinge","completed":true}`,
		`{"id":"03old000000000000000000000","content":"Order the hinge","completed":true},`+
			`{"id":"03abc123def456ghi789jkl0mn","content":"Measure the gap","completed":false}`, 1)
	if cardFingerprint(fpDecode(t, swapped)) == base {
		t.Errorf("step order did not change the fingerprint")
	}
}

// The fingerprint value is stored in drafts, so it must never change for
// the same card. This value was captured before synced-fields: existed.
func TestFingerprintPinned(t *testing.T) {
	if got := cardFingerprint(fpDecode(t, fpCard)); got != "sha256:d0222dca45771598" {
		t.Errorf("fingerprint = %q", got)
	}
}

func TestSyncedFieldsFormat(t *testing.T) {
	got := syncedFields(fpDecode(t, fpCard))
	if !regexp.MustCompile(`^title=[0-9a-f]{16} description=[0-9a-f]{16} steps=[0-9a-f]{8} tags=[0-9a-f]{8} ` +
		`golden=[0-9a-f]{8} column=[0-9a-f]{8} board=[0-9a-f]{8} closed=[0-9a-f]{8} postponed=[0-9a-f]{8}$`).MatchString(got) {
		t.Errorf("synced-fields = %q", got)
	}
}

func TestChangedFieldsNamesEachField(t *testing.T) {
	stored := syncedFields(fpDecode(t, fpCard))
	for want, edit := range map[string][2]string{
		"title":       {`"title":"Fix the gate"`, `"title":"Fix the gate now"`},
		"description": {`<p>Gate latches.</p>`, `<p>Gate latches shut.</p>`},
		"steps":       {`"completed":false`, `"completed":true`},
		"tags":        {`"tags":["buy","build"]`, `"tags":["buy"]`},
		"golden":      {`"golden":false`, `"golden":true`},
		"column":      {`"column":{"id":"c1","name":"Next"}`, `"column":null`},
		"board":       {`"board":{"id":"b1"`, `"board":{"id":"b2"`},
		"closed":      {`"closed":false`, `"closed":true`},
		"postponed":   {`"postponed":false`, `"postponed":true`},
		"description, steps": {`"description_html":"<p>Gate latches.</p>","last_active_at":"2026-10-17T10:00:00Z","steps":[`,
			`"description_html":"<p>New.</p>","last_active_at":"2026-10-17T10:00:00Z","steps":[{"id":"03add000000000000000000000","content":"Oil it","completed":false},`},
	} {
		changed := strings.Replace(fpCard, edit[0], edit[1], 1)
		if changed == fpCard {
			t.Fatalf("%s: edit did not apply", want)
		}
		if got := strings.Join(changedFields(stored, fpDecode(t, changed)), ", "); got != want {
			t.Errorf("changed %q, want %q", got, want)
		}
	}
	// Tag spelling is not a change, as in the fingerprint.
	same := strings.Replace(fpCard, `"tags":["buy","build"]`, `"tags":["build","#Buy"]`, 1)
	if got := changedFields(stored, fpDecode(t, same)); len(got) != 0 {
		t.Errorf("tag spelling: %q", got)
	}
}

// When the stored line is missing, malformed or incomplete, fz cannot say
// what changed, so changedFields returns nothing.
func TestChangedFieldsUnknown(t *testing.T) {
	card := fpDecode(t, strings.Replace(fpCard, `"golden":false`, `"golden":true`, 1))
	full := syncedFields(fpDecode(t, fpCard))
	for name, stored := range map[string]string{
		"missing":       "",
		"garbage":       "not a list",
		"bad hash":      strings.Replace(full, "golden=", "golden=zz", 1),
		"field missing": strings.Replace(full, " closed=", " closedx=", 1),
	} {
		if got := changedFields(stored, card); len(got) != 0 {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A field fz does not know, from a newer fz, is ignored.
	if got := changedFields(full+" color=0123abcd", card); strings.Join(got, ",") != "golden" {
		t.Errorf("extra field: %q", got)
	}
}
