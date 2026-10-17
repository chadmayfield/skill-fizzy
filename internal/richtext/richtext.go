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

// tag matches one HTML tag, from < to the next >.
var tag = regexp.MustCompile(`<[^>]*>`)

// ReplaceText swaps old for new in the text of html, never inside a tag or
// its attributes, so a link's href stays as it is. Both are plain text and
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
		count += strings.Count(s, o)
	}
	if count == 0 {
		return "", fmt.Errorf("%q not found in the description (text that spans formatting cannot be matched)", old)
	}
	if count > 1 && !all {
		return "", fmt.Errorf("%q appears %d times; use a longer phrase, or --all", old, count)
	}
	var b strings.Builder
	for i, s := range texts {
		b.WriteString(strings.ReplaceAll(s, o, n))
		if i < len(tags) {
			b.WriteString(html[tags[i][0]:tags[i][1]])
		}
	}
	return b.String(), nil
}
