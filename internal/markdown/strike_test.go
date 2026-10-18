package markdown

import "testing"

func TestStrikethrough(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"~~gone~~", "<del>gone</del>"},
		{"a ~~two words~~ b", "a <del>two words</del> b"},
		{"~~a~~ and ~~b~~", "<del>a</del> and <del>b</del>"},
		{"**bold**~~struck~~", "<strong>bold</strong><del>struck</del>"},
		{"~~**both**~~", "<del><strong>both</strong></del>"},
		{"~~*em* and `x`~~", "<del><em>em</em> and <code>x</code></del>"},
		{"~~[hinge](https://example.com/a)~~", `<del><a href="https://example.com/a">hinge</a></del>`},
		{"~~<b>~~", "<del>&lt;b&gt;</del>"},
		{"~a~", "~a~"},
		{"~", "~"},
		{"~~", "~~"},
		{"~~open", "~~open"},
		{"close~~", "close~~"},
		{"~~ spaced~~", "~~ spaced~~"},
		{"~~spaced ~~", "~~spaced ~~"},
		{"~~~three~~~", "~~~three~~~"},
		{"~~~~", "~~~~"},
		{"`~~code~~`", "<code>~~code~~</code>"},
		{"[a](https://example.com/~~x~~)", `<a href="https://example.com/~~x~~">a</a>`},
		{"https://example.com/~~x~~", `<a href="https://example.com/~~x~~">https://example.com/~~x~~</a>`},
	} {
		if got := Inline(c.in); got != c.want {
			t.Errorf("Inline(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStrikethroughBlocks(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"```\n~~code~~\n```", "<pre>~~code~~</pre>"},
		{"## ~~old~~ plan", "<h3><del>old</del> plan</h3>"},
		{"- ~~done~~", "<ul>\n<li><del>done</del></li>\n</ul>"},
		{"~~a\nb~~", "<p>~~a<br>b~~</p>"},
	} {
		if got := ToHTML(c.in); got != c.want {
			t.Errorf("ToHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
