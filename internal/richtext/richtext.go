// Package richtext makes small edits to Fizzy rich text (HTML) without a
// parser.
//
// Fizzy returns description_html wrapped in <div class="action-text-content">.
// Sending that back as it is would nest the content one div deeper on every
// edit, so the wrapper is stripped first. Text is compared in its escaped
// form, because that is how it sits in the HTML.
package richtext

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var wrapper = regexp.MustCompile(`(?s)\A\s*<div class="action-text-content">\s*(.*?)\s*</div>\s*\z`)

// UnwrapActionText strips the action-text wrapper div and the whitespace
// around the inner HTML. Text without the wrapper is only trimmed.
func UnwrapActionText(html string) string {
	if m := wrapper.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	return strings.TrimSpace(html)
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// escapeText escapes &, < and >, like Python's html.escape(s, quote=False).
// Quotes stay as they are, because Fizzy stores them unescaped in text.
func escapeText(s string) string { return escaper.Replace(s) }

// tag matches one comment or one HTML tag. A comment runs to its closing
// -->, and a tag runs to the first > outside a quoted attribute value, so a
// > inside a comment or a quoted attribute does not end it early.
var tag = regexp.MustCompile(`<!--[\s\S]*?-->|<(?:[^>"']|"[^"]*"|'[^']*')*>`)

// entity matches a character reference such as &gt; or &#39;.
var entity = regexp.MustCompile(`&[#a-zA-Z0-9]+;`)

// findMatches returns the start of each non-overlapping occurrence of o in
// s, left to right, skipping any occurrence that starts or ends inside an
// entity. A phrase that covers a whole entity, such as &amp;, still matches.
func findMatches(s, o string) []int {
	ents := entity.FindAllStringIndex(s, -1)
	inside := func(pos int) bool {
		for _, e := range ents {
			if pos > e[0] && pos < e[1] {
				return true
			}
		}
		return false
	}
	var out []int
	for from := 0; from <= len(s)-len(o); {
		i := strings.Index(s[from:], o)
		if i < 0 {
			break
		}
		i += from
		if inside(i) || inside(i+len(o)) {
			from = i + 1
			continue
		}
		out = append(out, i)
		from = i + len(o)
	}
	return out
}

// ReplaceText swaps old for new in the text of html, never inside a tag, a comment,
// an entity or an attribute, so a link's href stays as it is. Both are plain text and
// are escaped before matching. A match cannot span a tag. More than one
// match is an error unless all is true, so a short phrase inside a longer
// word does not change by accident.
func ReplaceText(html, old, new string, all bool) (string, error) {
	if old == "" {
		return "", errors.New("the phrase to replace is empty")
	}
	o, n := escapeText(old), escapeText(new)
	tags := tag.FindAllStringIndex(html, -1)
	// texts holds the text between tags, in order; texts[i] comes before
	// tags[i].
	texts := make([]string, 0, len(tags)+1)
	at := 0
	for _, t := range tags {
		texts = append(texts, html[at:t[0]])
		at = t[1]
	}
	texts = append(texts, html[at:])
	count := 0
	for _, s := range texts {
		count += len(findMatches(s, o))
	}
	if count == 0 {
		return "", fmt.Errorf("%q not found in the description (text that spans formatting cannot be matched)", old)
	}
	if count > 1 && !all {
		return "", fmt.Errorf("%q appears %d times; use a longer phrase, or --all", old, count)
	}
	var b strings.Builder
	for i, s := range texts {
		at := 0
		for _, m := range findMatches(s, o) {
			b.WriteString(s[at:m])
			b.WriteString(n)
			at = m + len(o)
		}
		b.WriteString(s[at:])
		if i < len(tags) {
			b.WriteString(html[tags[i][0]:tags[i][1]])
		}
	}
	return b.String(), nil
}
