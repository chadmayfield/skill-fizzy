package markdown

import (
	"strings"
	"testing"
)

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

// TestBareURLCannotLeaveHref checks that a bare URL ends at a quote, < or
// >, so its text cannot close the href and add an attribute such as an
// event handler.
func TestBareURLCannotLeaveHref(t *testing.T) {
	for in, want := range map[string]string{
		`http://x.example/"onmouseover=alert(1)//`: `<p><a href="http://x.example/">http://x.example/</a>"onmouseover=alert(1)//</p>`,
		`http://x.example/'onclick=alert(1)`:       `<p><a href="http://x.example/">http://x.example/</a>'onclick=alert(1)</p>`,
		`http://x.example/<b>x</b>`:                `<p><a href="http://x.example/">http://x.example/</a>&lt;b&gt;x&lt;/b&gt;</p>`,
		`http://x.example/a>b`:                     `<p><a href="http://x.example/a">http://x.example/a</a>&gt;b</p>`,
		`see http://x.example/a"b and more`:        `<p>see <a href="http://x.example/a">http://x.example/a</a>"b and more</p>`,
	} {
		got := ToHTML(in)
		if got != want {
			t.Errorf("ToHTML(%q)\n got  %s\n want %s", in, got, want)
		}
	}
}

// TestNoEmphasisInsideURL checks that * and _ in a URL stay as they are,
// in the href and in a bare URL's text, while a link label still gets
// emphasis.
func TestNoEmphasisInsideURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.example/*x*/y":           `<p><a href="https://a.example/*x*/y">https://a.example/*x*/y</a></p>`,
		"https://a.example/_x_/y":           `<p><a href="https://a.example/_x_/y">https://a.example/_x_/y</a></p>`,
		"https://a.example/**x**/y":         `<p><a href="https://a.example/**x**/y">https://a.example/**x**/y</a></p>`,
		"[see](https://a.example/_a_/b)":    `<p><a href="https://a.example/_a_/b">see</a></p>`,
		"[see](https://a.example/*a*/b)":    `<p><a href="https://a.example/*a*/b">see</a></p>`,
		"[**gate**](https://a.example/x)":   `<p><a href="https://a.example/x"><strong>gate</strong></a></p>`,
		"*oil* https://a.example/_x_ _now_": `<p><em>oil</em> <a href="https://a.example/_x_">https://a.example/_x_</a> <em>now</em></p>`,
	} {
		if got := ToHTML(in); got != want {
			t.Errorf("ToHTML(%q)\n got  %s\n want %s", in, got, want)
		}
	}
}

// A URL ends where an inline code span starts, so the code's <code> tag
// never lands inside an href.
func TestURLEndsAtCodeSpan(t *testing.T) {
	for in, want := range map[string]string{
		"See https://example.com/a`b`": `<p>See <a href="https://example.com/a">https://example.com/a</a><code>b</code></p>`,
		"[x](https://example.com/`b`)": `<p>[x](<a href="https://example.com/">https://example.com/</a><code>b</code>)</p>`,
		"[x](https://example.com/a)`b`": `<p><a href="https://example.com/a">x</a><code>b</code></p>`,
	} {
		got := ToHTML(in)
		if got != want {
			t.Errorf("ToHTML(%q)\n got %s\nwant %s", in, got, want)
		}
		if strings.Contains(got, `href="https://example.com/a<code>`) || strings.Contains(got, `/<code>b</code>"`) {
			t.Errorf("ToHTML(%q) put code in an href: %s", in, got)
		}
	}
}
