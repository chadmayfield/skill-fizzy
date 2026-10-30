package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/draft"
)

// openImage is os.Open. A test replaces it to count the image reads.
var openImage = os.Open

// syncedDraftKey is the front matter key for draftHash.
const syncedDraftKey = "synced-draft"

// draftHash returns "sha256:" and the first 16 hex characters of the
// SHA-256 of what the draft sends as the title and description: the title,
// the body Markdown and the SHA-256 of each image file's bytes, in image
// order. fz stores it as synced-draft: after a sync, so the next --update
// can skip the uploads and the description when none of them changed.
func draftHash(d *draft.Draft, inbox string) (string, error) {
	images := []string{}
	for _, img := range d.Images() {
		path, err := d.ResolveImage(img, inbox)
		if err != nil {
			return "", err
		}
		h, err := imageHash(path)
		if err != nil {
			return "", err
		}
		images = append(images, h)
	}
	b, _ := json.Marshal(struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Images []string `json:"images"`
	}{d.Title, d.BodyMD, images})
	sum := sha256.Sum256(b)
	return fingerprintPrefix + hex.EncodeToString(sum[:])[:16], nil
}

// imageHash returns the hex SHA-256 of the file at path. It first refuses
// what Upload refuses, a file that is not regular or is over
// fizzy.MaxUploadSize, with the same Refusal, and then streams the file, so
// memory stays small whatever its size.
func imageHash(path string) (string, error) {
	fi, err := fizzy.CheckUpload(path)
	if err != nil {
		return "", err
	}
	f, err := openImage(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, fizzy.MaxUploadSize+1))
	if err != nil {
		return "", err
	}
	if n != fi.Size() {
		return "", fmt.Errorf("%s changed size while fz read it", fizzy.PyRepr(path))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// textSynced reports whether the card's title and description hash to the
// values in stored, a synced-fields: value. A missing or malformed value is
// false, so fz sends them, and so is a title or description hash in the
// 8-hex form an older fz wrote.
func textSynced(stored string, card fizzy.Card) bool {
	have := map[string]string{}
	for _, p := range strings.Fields(stored) {
		if name, hash, ok := strings.Cut(p, "="); ok && fieldHashRE.MatchString(hash) {
			have[name] = hash
		}
	}
	st := newFingerprintState(card)
	return len(have["title"]) == 16 && have["title"] == fieldHash("title", st.Title) &&
		len(have["description"]) == 16 && have["description"] == fieldHash("description", st.DescriptionHTML)
}

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
		st.Tags = append(st.Tags, fizzy.TagTitle(t))
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

// fieldHashRE matches a synced-fields: hash: 8 hex characters, or 16 for
// the title and description since fz wrote them longer.
var fieldHashRE = regexp.MustCompile(`^[0-9a-f]{8}([0-9a-f]{8})?$`)

// longFields are the fields whose synced-fields: hash has 16 hex
// characters, because the --update skip trusts them alone. 8 would let
// someone who can edit the card find a matching text in about 2^32 tries.
var longFields = map[string]bool{"title": true, "description": true}

// fieldHash is the first 8 hex characters of the SHA-256 of v's JSON, or
// the first 16 for a field in longFields.
func fieldHash(name string, v any) string {
	// Marshal cannot fail: v is a string, bool or slice of them.
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	if longFields[name] {
		return hex.EncodeToString(sum[:])[:16]
	}
	return hex.EncodeToString(sum[:])[:8]
}

// syncedFields returns the draft's synced-fields: value for c, one
// name=hash pair per fingerprint field, so a later refusal can name the
// fields that changed. synced: stays the check; this line only explains it.
func syncedFields(c fizzy.Card) string {
	st := newFingerprintState(c)
	parts := make([]string, len(fingerprintFields))
	for i, f := range fingerprintFields {
		parts[i] = f.name + "=" + fieldHash(f.name, f.value(st))
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
		// An 8-hex hash from an older fz is compared with the first 8
		// characters. A hash longer than fz writes for the field cannot
		// be told.
		want := fieldHash(f.name, f.value(st))
		if len(h) > len(want) {
			return nil
		}
		if h != want[:len(h)] {
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
