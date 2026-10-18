package markdown

import "testing"

// TestHrefEscapesAmpersandOnce checks that every href holds & as &amp;
// exactly once, for a labeled link and a bare URL alike, and that a URL
// written with &amp; in the draft gets the same href as one written with &.
func TestHrefEscapesAmpersandOnce(t *testing.T) {
	for in, want := range map[string]string{
		"[a](http://example.com/a?b=1&c=2)":     `<p><a href="http://example.com/a?b=1&amp;c=2">a</a></p>`,
		"[a](http://example.com/a?b=1&amp;c=2)": `<p><a href="http://example.com/a?b=1&amp;c=2">a</a></p>`,
		"http://example.com/a?b=1&c=2":          `<p><a href="http://example.com/a?b=1&amp;c=2">http://example.com/a?b=1&amp;c=2</a></p>`,
		"http://example.com/a?b=1&amp;c=2":      `<p><a href="http://example.com/a?b=1&amp;c=2">http://example.com/a?b=1&amp;amp;c=2</a></p>`,
	} {
		if got := ToHTML(in); got != want {
			t.Errorf("ToHTML(%q)\n got  %s\n want %s", in, got, want)
		}
	}
}
