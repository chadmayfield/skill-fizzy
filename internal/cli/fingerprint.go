package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

const fingerprintPrefix = "sha256:"

var fingerprintRE = regexp.MustCompile(`^sha256:[0-9a-f]{16}$`)

// fingerprintStep is one card step as the fingerprint encodes it.
type fingerprintStep struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Completed bool   `json:"completed"`
}

// fingerprintState is the card state --update can overwrite or a user can
// edit in Fizzy. The field order is fixed, so its JSON is canonical.
type fingerprintState struct {
	Title           string            `json:"title"`
	DescriptionHTML string            `json:"description_html"`
	Steps           []fingerprintStep `json:"steps"`
	Tags            []string          `json:"tags"`
	Golden          bool              `json:"golden"`
	Column          string            `json:"column"`
	Board           string            `json:"board"`
	Closed          bool              `json:"closed"`
	Postponed       bool              `json:"postponed"`
}

// newFingerprintState returns the canonical state of c.
func newFingerprintState(c fizzy.Card) fingerprintState {
	st := fingerprintState{
		Title:           c.Title,
		DescriptionHTML: c.DescriptionHTML,
		Steps:           make([]fingerprintStep, 0, len(c.Steps)),
		Tags:            make([]string, 0, len(c.Tags)),
		Golden:          c.Golden,
		Board:           c.Board.ID,
		Closed:          c.Closed,
		Postponed:       c.Postponed,
	}
	for _, s := range c.Steps {
		st.Steps = append(st.Steps, fingerprintStep{ID: s.ID, Content: s.Content, Completed: s.Completed})
	}
	for _, t := range c.Tags {
		st.Tags = append(st.Tags, tagKey(t))
	}
	sort.Strings(st.Tags)
	if c.Column != nil {
		st.Column = c.Column.ID
	}
	return st
}

// cardFingerprint returns "sha256:" and the first 16 hex characters of the
// SHA-256 of the card's canonical state. Postponed is in it because Maybe?
// and Not now both have no column. fz stores it in a draft's synced:
// line. last_active_at is not used: Fizzy leaves it alone when a description,
// step, tag or golden mark changes.
func cardFingerprint(c fizzy.Card) string {
	// Marshal cannot fail: every field is a string, bool or slice of them.
	b, _ := json.Marshal(newFingerprintState(c))
	sum := sha256.Sum256(b)
	return fingerprintPrefix + hex.EncodeToString(sum[:])[:16]
}

// fingerprintField is one part of the fingerprint state, by the name a
// refusal message uses for it.
type fingerprintField struct {
	name  string
	value func(fingerprintState) any
}

// fingerprintFields lists the parts of the state in the order
// synced-fields: and refusal messages use.
var fingerprintFields = []fingerprintField{
	{"title", func(s fingerprintState) any { return s.Title }},
	{"description", func(s fingerprintState) any { return s.DescriptionHTML }},
	{"steps", func(s fingerprintState) any { return s.Steps }},
	{"tags", func(s fingerprintState) any { return s.Tags }},
	{"golden", func(s fingerprintState) any { return s.Golden }},
	{"column", func(s fingerprintState) any { return s.Column }},
	{"board", func(s fingerprintState) any { return s.Board }},
	{"closed", func(s fingerprintState) any { return s.Closed }},
	{"postponed", func(s fingerprintState) any { return s.Postponed }},
}

var fieldHashRE = regexp.MustCompile(`^[0-9a-f]{8}$`)

// fieldHash is the first 8 hex characters of the SHA-256 of v's JSON.
func fieldHash(v any) string {
	// Marshal cannot fail: v is a string, bool or slice of them.
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:8]
}

// syncedFields returns the draft's synced-fields: value for c, one
// name=hash pair per fingerprint field, so a later refusal can name the
// fields that changed. synced: stays the check; this line only explains it.
func syncedFields(c fizzy.Card) string {
	st := newFingerprintState(c)
	parts := make([]string, len(fingerprintFields))
	for i, f := range fingerprintFields {
		parts[i] = f.name + "=" + fieldHash(f.value(st))
	}
	return strings.Join(parts, " ")
}

// changedFields returns the names of the fields whose hash in stored, a
// synced-fields: value, differs from card. It returns nil when stored is
// missing, malformed or lacks a field, because then fz cannot tell. Names
// it does not know, from a newer fz, are ignored.
func changedFields(stored string, card fizzy.Card) []string {
	have := map[string]string{}
	for _, p := range strings.Fields(stored) {
		name, hash, ok := strings.Cut(p, "=")
		if !ok || !fieldHashRE.MatchString(hash) {
			return nil
		}
		have[name] = hash
	}
	st := newFingerprintState(card)
	var changed []string
	for _, f := range fingerprintFields {
		h, ok := have[f.name]
		if !ok {
			return nil
		}
		if h != fieldHash(f.value(st)) {
			changed = append(changed, f.name)
		}
	}
	return changed
}

// isFingerprint reports whether a synced: value is one cardFingerprint made.
func isFingerprint(s string) bool { return fingerprintRE.MatchString(s) }

// isTimestamp reports whether a synced: value is a time, which is what fz
// stored there before it used fingerprints.
func isTimestamp(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}
