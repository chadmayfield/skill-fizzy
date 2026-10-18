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
	"html"
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

// ErrImagePath is matched by errors.Is for an image target fz refuses: one
// that resolves outside the draft's folder and the inbox, or that holds
// -->, < or >. Such an error also matches ErrDraft.
var ErrImagePath = errors.New("image path refused")

// ErrBadValue is matched by errors.Is for a front matter value that fz
// refuses rather than guess at, such as card: 87abc or golden: maybe. Every
// such error also matches ErrDraft.
var ErrBadValue = errors.New("bad front matter value")

type draftError struct{ msg string }

func (e *draftError) Error() string        { return e.msg }
func (e *draftError) Is(target error) bool { return target == ErrDraft }

func errf(format string, args ...any) error {
	return &draftError{fmt.Sprintf(format, args...)}
}

// imageError is an image target fz refuses. It matches ErrImagePath and
// ErrDraft.
type imageError struct{ msg string }

func (e *imageError) Error() string { return e.msg }
func (e *imageError) Is(target error) bool {
	return target == ErrImagePath || target == ErrDraft
}

func imagef(format string, args ...any) error {
	return &imageError{fmt.Sprintf(format, args...)}
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

// commentStart returns where a # comment starts in a front matter line, or
// -1. A comment is a # after whitespace and before whitespace or the end of
// the line. A # inside a quoted value, or inside a quoted item of a [list],
// is part of the value. A quote counts only where a value or list item
// starts and only when the same quote closes it later on the line, so an
// apostrophe in a plain value does not hide a comment.
func commentStart(line string) int {
	afterColon, itemStart := false, false
	var quote rune
	prev := rune(-1)
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case itemStart && (r == '"' || r == '\'') && strings.ContainsRune(line[i+1:], r):
			quote, itemStart = r, false
		case r == '#' && prev >= 0 && isSpace(prev):
			next, _ := utf8.DecodeRuneInString(line[i+1:])
			if i+1 == len(line) || isSpace(next) {
				// The comment takes the whitespace before it too.
				return len(strings.TrimRightFunc(line[:i], isSpace))
			}
			itemStart = false
		case !afterColon && r == ':':
			afterColon, itemStart = true, true
		case afterColon && (r == '[' || r == ','):
			itemStart = true
		case isSpace(r):
		default:
			itemStart = false
		}
		prev = r
	}
	return -1
}

// parseFront reads the front matter lines. It returns each key's value and
// the line it came from, for error messages. The last line for a key wins.
func parseFront(text string) (meta, lines map[string]string, err error) {
	meta, lines = map[string]string{}, map[string]string{}
	for _, raw := range splitLines(text) {
		line := raw
		if i := commentStart(raw); i >= 0 {
			line = raw[:i]
		}
		line = strings.TrimRightFunc(line, isSpace)
		if pyStrip(line) == "" || strings.HasPrefix(strings.TrimLeftFunc(line, isSpace), "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, nil, errf("front matter line has no ':': '%s'", raw)
		}
		v = strings.Trim(pyStrip(v), `"`)
		v = strings.Trim(v, `'`)
		key := strings.ToLower(pyStrip(k))
		meta[key] = v
		lines[key] = pyStrip(raw)
	}
	return meta, lines, nil
}

// unquote removes one matching pair of single or double quotes around s.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func splitList(value string) []string {
	value = strings.Trim(pyStrip(value), "[]")
	tags := []string{}
	for _, t := range strings.Split(value, ",") {
		if pyStrip(t) == "" {
			continue
		}
		t = pyStrip(unquote(pyStrip(t)))
		t = strings.TrimLeft(t, "#")
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
	meta, lines, err := parseFront(text[m[2]:m[3]])
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
	inSteps, inFence := false, false
	for _, line := range strings.Split(rest, "\n") {
		// Inside a fenced code block, as markdown.ToHTML reads one, a
		// heading is code, not the title or the Steps section.
		if !inSteps && strings.HasPrefix(pyStrip(line), "```") {
			inFence = !inFence
			body = append(body, line)
			continue
		}
		if inFence {
			body = append(body, line)
			continue
		}
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
	card, err := cardNumber(meta["card"])
	if err != nil {
		return nil, badValue(path, lines["card"], err.Error())
	}
	golden := false
	switch strings.ToLower(meta["golden"]) {
	case "true", "yes", "on", "1":
		golden = true
	case "", "false", "no", "off", "0":
	default:
		return nil, badValue(path, lines["golden"], "golden: must be true or false (yes, no, on, off, 1 and 0 also work)")
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

// cardNumber reads a card: value: a card number such as 87, or #87 the way
// Fizzy writes it. An empty value means the draft has no card yet.
func cardNumber(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	digits := strings.TrimPrefix(v, "#")
	if isASCIIDigits(digits) {
		if n, err := strconv.Atoi(digits); err == nil && n > 0 {
			return n, nil
		}
	}
	return 0, errors.New("card: must be a card number such as 87 or #87")
}

// valueError is a front matter value fz will not guess at. It is a draft
// error, and the CLI refuses it with exit 2.
type valueError struct{ msg string }

func (e *valueError) Error() string { return e.msg }
func (e *valueError) Is(target error) bool {
	return target == ErrDraft || target == ErrBadValue
}

func badValue(path, line, why string) error {
	return &valueError{fmt.Sprintf("%s: front matter line '%s': %s", path, line, why)}
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

// blankCode returns text with fenced code blocks and inline code spans
// turned into spaces, so Links finds no URL inside code. Offsets and line
// breaks stay where they were.
func blankCode(text string) string {
	lines := strings.Split(text, "\n")
	inFence := false
	for i, line := range lines {
		fence := strings.HasPrefix(pyStrip(line), "```")
		if inFence || fence {
			lines[i] = strings.Repeat(" ", len(line))
			if fence {
				inFence = !inFence
			}
			continue
		}
		b := []byte(line)
		for j := 0; j < len(b); j++ {
			if b[j] != '`' {
				continue
			}
			k := strings.IndexByte(line[j+1:], '`')
			if k <= 0 {
				continue
			}
			for x := j; x <= j+1+k; x++ {
				b[x] = ' '
			}
			j += k + 1
		}
		lines[i] = string(b)
	}
	return strings.Join(lines, "\n")
}

// Links returns the distinct http(s) URLs in the body, in order of first
// use, leaving out any URL inside code.
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
	text := blankCode(d.BodyMD)
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
// It finds them as markdown.ToHTML does, so the files fz uploads are the
// ones the description embeds: a line inside a code fence is code, and
// only \n ends a line.
func (d *Draft) Images() []string { return markdown.ImageTargets(d.BodyMD) }

// CheckImages fails if an image would not embed, if its target is refused
// (see ResolveImage), or if its file is missing.
func (d *Draft) CheckImages(inbox string) error {
	for _, line := range markdown.TextLines(d.BodyMD) {
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

// CheckImagePaths fails with an ErrImagePath error if any image target is
// refused. A missing file is not an error here, so draft preview can show a
// draft whose images are not there yet.
func (d *Draft) CheckImagePaths(inbox string) error {
	for _, img := range d.Images() {
		if _, err := d.ResolveImage(img, inbox); errors.Is(err, ErrImagePath) {
			return err
		}
	}
	return nil
}

// ResolveImage finds an image file and returns its real path. A target is
// a path relative to the draft's folder, tried first, or to the inbox, and
// it must stay inside one of the two after symlinks are resolved. An
// absolute path, a ~ path, a path whose .. leaves both folders, a symlink
// out of them, and a target holding -->, < or > are refused with an
// ErrImagePath error. A missing file is an ErrDraft error.
func (d *Draft) ResolveImage(target, inbox string) (string, error) {
	if strings.Contains(target, "-->") || strings.ContainsAny(target, "<>") {
		return "", imagef("image %s: the path must not hold -->, < or >", pyRepr(target))
	}
	if filepath.IsAbs(target) || target == "~" || strings.HasPrefix(target, "~/") || filepath.VolumeName(target) != "" {
		return "", imagef("image %s: give a path relative to the draft's folder or the inbox; fz does not attach an absolute or ~ path", pyRepr(target))
	}
	type root struct{ abs, real string }
	var roots []root
	for _, dir := range []string{filepath.Dir(d.Path), inbox} {
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		real := abs
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			real = r
		}
		roots = append(roots, root{abs, real})
	}
	inside := func(p string, real bool) bool {
		for _, r := range roots {
			base := r.abs
			if real {
				base = r.real
			}
			if rel, err := filepath.Rel(base, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	var candidates []string
	for _, r := range roots {
		c := filepath.Join(r.abs, target)
		if !inside(c, false) {
			continue
		}
		candidates = append(candidates, c)
		st, err := os.Stat(c)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		real, err := filepath.EvalSymlinks(c)
		if err != nil {
			continue
		}
		if !inside(real, true) {
			return "", imagef("image %s: it leads to %s, outside the draft's folder and the inbox", pyRepr(target), real)
		}
		return real, nil
	}
	if len(candidates) == 0 {
		return "", imagef("image %s: the path leaves the draft's folder and the inbox", pyRepr(target))
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
			out = strings.ReplaceAll(out, placeholder, "<p>[image: "+html.EscapeString(target)+"]</p>")
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
