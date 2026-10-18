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
