// Package richtext makes small edits to Fizzy rich text (HTML) without a
// parser.
//
// Fizzy returns description_html wrapped in <div class="action-text-content">.
// Sending that back as it is would nest the content one div deeper on every
// edit, so the wrapper is stripped first. Text is compared in its escaped
// form, because that is how it sits in the HTML.
package richtext

import (
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

// ReplaceText swaps old for new in html. Both are plain text and are escaped
// before matching. More than one match is an error unless all is true, so a
// short phrase inside a longer word does not change by accident.
func ReplaceText(html, old, new string, all bool) (string, error) {
	o, n := escapeText(old), escapeText(new)
	count := strings.Count(html, o)
	if count == 0 {
		return "", fmt.Errorf("%q not found in the description (text that spans formatting cannot be matched)", old)
	}
	if count > 1 && !all {
		return "", fmt.Errorf("%q appears %d times; use a longer phrase, or --all", old, count)
	}
	return strings.ReplaceAll(html, o, n), nil
}
