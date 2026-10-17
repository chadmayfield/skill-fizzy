package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
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
}

// cardFingerprint returns "sha256:" and the first 16 hex characters of the
// SHA-256 of the card's canonical state. fz stores it in a draft's synced:
// line. last_active_at is not used: Fizzy leaves it alone when a description,
// step, tag or golden mark changes.
func cardFingerprint(c fizzy.Card) string {
	st := fingerprintState{
		Title:           c.Title,
		DescriptionHTML: c.DescriptionHTML,
		Steps:           make([]fingerprintStep, 0, len(c.Steps)),
		Tags:            make([]string, 0, len(c.Tags)),
		Golden:          c.Golden,
		Board:           c.Board.ID,
		Closed:          c.Closed,
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
	// Marshal cannot fail: every field is a string, bool or slice of them.
	b, _ := json.Marshal(st)
	sum := sha256.Sum256(b)
	return fingerprintPrefix + hex.EncodeToString(sum[:])[:16]
}

// isFingerprint reports whether a synced: value is one cardFingerprint made.
func isFingerprint(s string) bool { return fingerprintRE.MatchString(s) }

// isTimestamp reports whether a synced: value is a time, which is what fz
// stored there before it used fingerprints.
func isTimestamp(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}
