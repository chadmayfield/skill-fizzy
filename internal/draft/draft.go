// Package draft reads card drafts: one Markdown file per card.
//
// A draft starts with front matter (board, and optionally column, tags,
// golden, card, synced, synced-fields), then a "# Title" line, then a body.
// The "## Steps" section becomes Fizzy steps and everything else except the
// title becomes the card description. This is a port of the Python fz's draft.py (removed in
// 562a4a9); the whitespace and regular expression rules follow Python's so
// both parsers agree.
package draft

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chadmayfield/skill-fizzy/internal/markdown"
)

// ErrDraft is matched by errors.Is for every problem found in a draft.
var ErrDraft = errors.New("draft error")

type draftError struct{ msg string }

func (e *draftError) Error() string        { return e.msg }
func (e *draftError) Is(target error) bool { return target == ErrDraft }

func errf(format string, args ...any) error {
	return &draftError{fmt.Sprintf(format, args...)}
}

// pySpace is the class of characters Python's str.isspace and \s accept.
const pySpace = `\t\n\v\f\r\x1c-\x1f \x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

var (
	frontRe   = regexp.MustCompile(`(?s)\A---[` + pySpace + `]*\n(.*?)\n---[` + pySpace + `]*\n`)
	headingRe = regexp.MustCompile(`^(#{1,6})[` + pySpace + `]+(.*?)[` + pySpace + `]*$`)
	commentRe = regexp.MustCompile(`[` + pySpace + `]+#(?:[` + pySpace + `]|$)`)
	imageRe   = regexp.MustCompile(`!\[[^\]]*\]\([^)` + pySpace + `]+\)`)
	mdLinkRe  = regexp.MustCompile(`\A\[[^\]]*\]\((https?://[^)` + pySpace + `]+)\)`)
	bareRe    = regexp.MustCompile(`\A(https?://[^` + pySpace + `<>)]+[^` + pySpace + `<>).,;:!?])`)
)

var stepsHeadings = map[string]bool{"steps": true, "tasks": true, "checklist": true}

// Step is one checklist line from a draft.
type Step struct {
	Content string
	Done    bool
}

// KV is one front matter key and value, written in the order given.
type KV struct{ Key, Value string }

// Draft is a parsed card draft. Card is 0 when the draft has no card yet.
type Draft struct {
	Path   string
	Title  string
	Board  string
	Column string
	Tags   []string
	Golden bool
	Card   int
	Synced string
	BodyMD string
	Steps  []Step
	Meta   map[string]string
}

func isSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func pyStrip(s string) string { return strings.TrimFunc(s, isSpace) }

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// splitLines mirrors Python's str.splitlines (no kept ends).
func splitLines(s string) []string {
	var out []string
	start := 0
	rs := []rune(s)
	runeStart := 0
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			out = append(out, string(rs[runeStart:i]))
			if r == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			runeStart = i + 1
		}
	}
	_ = start
	if runeStart < len(rs) {
		out = append(out, string(rs[runeStart:]))
	}
	return out
}

func parseFront(text string) (map[string]string, error) {
	meta := map[string]string{}
	for _, raw := range splitLines(text) {
		line := raw
		if loc := commentRe.FindStringIndex(raw); loc != nil {
			line = raw[:loc[0]]
		}
		line = strings.TrimRightFunc(line, isSpace)
		if pyStrip(line) == "" || strings.HasPrefix(strings.TrimLeftFunc(line, isSpace), "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, errf("front matter line has no ':': '%s'", raw)
		}
		v = strings.Trim(pyStrip(v), `"`)
		v = strings.Trim(v, `'`)
		meta[strings.ToLower(pyStrip(k))] = v
	}
	return meta, nil
}

func splitList(value string) []string {
	value = strings.Trim(pyStrip(value), "[]")
	tags := []string{}
	for _, t := range strings.Split(value, ",") {
		if pyStrip(t) == "" {
			continue
		}
		t = strings.TrimLeft(pyStrip(t), "#")
		t = strings.Trim(pyStrip(t), `"`)
		tags = append(tags, strings.ToLower(t))
	}
	return tags
}

// parseStep reads "- [ ] text" (bullet -, * or +). Python: ^\s*[-*+]\s+\[([ xX])\]\s+(.*\S)\s*$
func parseStep(line string) (content string, done, ok bool) {
	s := strings.TrimLeftFunc(line, isSpace)
	if s == "" || !strings.ContainsRune("-*+", rune(s[0])) {
		return "", false, false
	}
	s = s[1:]
	rest := strings.TrimLeftFunc(s, isSpace)
	if len(rest) == len(s) || len(rest) < 3 || rest[0] != '[' || rest[2] != ']' || !strings.ContainsRune(" xX", rune(rest[1])) {
		return "", false, false
	}
	mark := rest[1]
	after := rest[3:]
	body := strings.TrimLeftFunc(after, isSpace)
	if len(body) == len(after) {
		return "", false, false
	}
	body = strings.TrimRightFunc(body, isSpace)
	if body == "" || strings.ContainsRune(body, '\n') {
		return "", false, false
	}
	return body, mark == 'x' || mark == 'X', true
}

// Parse reads and validates a draft file.
func Parse(path string) (*Draft, error) {
	src, err := readSource(path)
	if err != nil {
		return nil, err
	}
	text := src.text
	m := frontRe.FindStringSubmatchIndex(text)
	if m == nil {
		return nil, errf("%s: missing front matter (--- board: ... ---)", path)
	}
	meta, err := parseFront(text[m[2]:m[3]])
	if err != nil {
		return nil, err
	}
	rest := text[m[1]:]
	if meta["board"] == "" {
		return nil, errf("%s: front matter needs 'board:'", path)
	}

	var title string
	haveTitle := false
	var body []string
	steps := []Step{}
	inSteps := false
	for _, line := range strings.Split(rest, "\n") {
		h := headingRe.FindStringSubmatch(line)
		if h != nil && len(h[1]) == 1 && !haveTitle {
			title, haveTitle = h[2], true
			continue
		}
		if h != nil && len(h[1]) == 2 {
			inSteps = stepsHeadings[strings.ToLower(pyStrip(h[2]))]
			if inSteps {
				continue
			}
		}
		if inSteps {
			if c, done, ok := parseStep(line); ok {
				steps = append(steps, Step{c, done})
			} else if pyStrip(line) != "" {
				return nil, errf("%s: lines under '## Steps' must be '- [ ] task' (got %s)", path, pyRepr(pyStrip(line)))
			}
			continue
		}
		body = append(body, line)
	}
	if !haveTitle || title == "" {
		return nil, errf("%s: needs a '# Title' line after the front matter", path)
	}

	column := meta["column"]
	if column == "" {
		column = "Maybe?"
	}
	card := 0
	if c := meta["card"]; c != "" && isASCIIDigits(c) {
		if n, err := strconv.Atoi(c); err == nil {
			card = n
		}
	}
	golden := false
	if g, ok := meta["golden"]; ok {
		switch strings.ToLower(g) {
		case "true", "yes", "1":
			golden = true
		}
	}
	return &Draft{
		Path:   path,
		Title:  pyStrip(title),
		Board:  meta["board"],
		Column: column,
		Tags:   splitList(meta["tags"]),
		Golden: golden,
		Card:   card,
		Synced: meta["synced"],
		BodyMD: pyStrip(strings.Join(body, "\n")),
		Steps:  steps,
		Meta:   meta,
	}, nil
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// pyRepr approximates Python's repr for a string.
func pyRepr(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		default:
			b.WriteString(fmt.Sprintf(`\x%02x`, r))
		}
	}
	b.WriteByte(q)
	return b.String()
}

// Links returns the distinct http(s) URLs in the body, in order of first use.
// The Python pattern is
//
//	\[[^\]]*\]\((https?://[^)\s]+)\)|(?<![("])\b(https?://[^\s<>)]+[^\s<>).,;:!?])
//
// and RE2 has no lookbehind, so the scan below applies it by hand.
func (d *Draft) Links() []string {
	found := []string{}
	seen := map[string]bool{}
	add := func(u string) {
		if !seen[u] {
			seen[u] = true
			found = append(found, u)
		}
	}
	text := d.BodyMD
	for i := 0; i < len(text); {
		if m := mdLinkRe.FindStringSubmatch(text[i:]); m != nil {
			add(m[1])
			i += len(m[0])
			continue
		}
		if text[i] == 'h' {
			prev, _ := utf8.DecodeLastRuneInString(text[:i])
			if i == 0 || (prev != '(' && prev != '"' && !isWord(prev)) {
				if m := bareRe.FindStringSubmatch(text[i:]); m != nil {
					add(m[1])
					i += len(m[0])
					continue
				}
			}
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	return found
}

// Images returns the targets of image lines; only these become attachments.
func (d *Draft) Images() []string {
	out := []string{}
	for _, line := range splitLines(d.BodyMD) {
		if _, target, ok := markdown.ImageLine(line); ok {
			out = append(out, target)
		}
	}
	return out
}

// CheckImages fails if an image would not embed or its file is missing.
func (d *Draft) CheckImages(inbox string) error {
	for _, line := range splitLines(d.BodyMD) {
		if imageRe.MatchString(line) {
			if _, _, ok := markdown.ImageLine(line); !ok {
				return errf("image must be on a line by itself to embed: %s", pyRepr(pyStrip(line)))
			}
		}
	}
	for _, img := range d.Images() {
		if _, err := d.ResolveImage(img, inbox); err != nil {
			return err
		}
	}
	return nil
}

// ResolveImage finds an image file: relative paths try the draft's folder,
// then the inbox.
func (d *Draft) ResolveImage(target, inbox string) (string, error) {
	p := target
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	var candidates []string
	if filepath.IsAbs(p) {
		candidates = []string{filepath.Clean(p)}
	} else {
		candidates = []string{filepath.Join(filepath.Dir(d.Path), p), filepath.Join(inbox, p)}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c, nil
		}
	}
	return "", errf("image not found: %s (looked in %s)", target, strings.Join(candidates, ", "))
}

// DescriptionHTML renders the body, swapping each image placeholder for an
// attachment when sgids has the target, or for a text note when it does not.
func (d *Draft) DescriptionHTML(sgids map[string]string) string {
	out := markdown.ToHTML(d.BodyMD)
	for _, target := range d.Images() {
		placeholder := "<!--attach:" + target + "-->"
		if sgid, ok := sgids[target]; ok {
			out = strings.ReplaceAll(out, placeholder, `<action-text-attachment sgid="`+sgid+`"></action-text-attachment>`)
		} else {
			out = strings.ReplaceAll(out, placeholder, "<p>[image: "+target+"]</p>")
		}
	}
	return out
}

// CardStep is a step already on a card.
type CardStep struct {
	ID, Content string
	Completed   bool
}

// StepPlan is what to change on a card so its steps match a draft.
type StepPlan struct {
	Add   []Step
	Tick  []string
	Extra []CardStep
}

// PlanSteps matches draft steps to card steps by text, one card step per draft
// step, so two draft steps with the same text claim two card steps.
func PlanSteps(wanted []Step, existing []CardStep) StepPlan {
	pool := append([]CardStep(nil), existing...)
	var plan StepPlan
	for _, w := range wanted {
		idx := -1
		for i, s := range pool {
			if s.Content == w.Content {
				idx = i
				break
			}
		}
		if idx < 0 {
			plan.Add = append(plan.Add, w)
			continue
		}
		match := pool[idx]
		pool = append(pool[:idx], pool[idx+1:]...)
		// A tick made in Fizzy is newer than the draft, so never untick.
		if w.Done && !match.Completed {
			plan.Tick = append(plan.Tick, match.ID)
		}
	}
	plan.Extra = pool
	return plan
}
