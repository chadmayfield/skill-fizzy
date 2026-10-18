package markdown

import "testing"

func TestTables(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"basic", "| Part | Qty |\n|---|---|\n| hinge | 2 |\n| latch | 1 |",
			"<table><thead><tr><th>Part</th><th>Qty</th></tr></thead><tbody>" +
				"<tr><td>hinge</td><td>2</td></tr><tr><td>latch</td><td>1</td></tr></tbody></table>"},
		{"no outer pipes and alignment", "Part | Qty\n:--- | ---:\nhinge | 2",
			"<table><thead><tr><th>Part</th><th>Qty</th></tr></thead><tbody>" +
				"<tr><td>hinge</td><td>2</td></tr></tbody></table>"},
		{"centered", "| a |\n| :-: |\n| b |",
			"<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>b</td></tr></tbody></table>"},
		{"header only", "| a | b |\n| - | - |",
			"<table><thead><tr><th>a</th><th>b</th></tr></thead></table>"},
		{"empty cell", "| a | b |\n|---|---|\n|  | x |",
			"<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td></td><td>x</td></tr></tbody></table>"},
		{"escaped pipe", "| cmd | note |\n|---|---|\n| `a \\| b` | x \\| y |",
			"<table><thead><tr><th>cmd</th><th>note</th></tr></thead><tbody>" +
				"<tr><td><code>a | b</code></td><td>x | y</td></tr></tbody></table>"},
		{"short row padded, long row cut", "| a | b |\n|---|---|\n| 1 |\n| 1 | 2 | 3 |",
			"<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody>" +
				"<tr><td>1</td><td></td></tr><tr><td>1</td><td>2</td></tr></tbody></table>"},
		{"inline markup in cells", "| **a** | ~~b~~ |\n|---|---|\n| *c* | [d](https://example.com/d) |",
			"<table><thead><tr><th><strong>a</strong></th><th><del>b</del></th></tr></thead><tbody>" +
				`<tr><td><em>c</em></td><td><a href="https://example.com/d">d</a></td></tr></tbody></table>`},
		{"cells are escaped", "| <b> |\n|---|\n| a & b |",
			"<table><thead><tr><th>&lt;b&gt;</th></tr></thead><tbody><tr><td>a &amp; b</td></tr></tbody></table>"},
		{"ends at a blank line", "| a |\n|---|\n| b |\n\n| c |",
			"<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>b</td></tr></tbody></table>\n<p>| c |</p>"},
		{"ends at a line without a pipe", "| a |\n|---|\n| b |\nafter",
			"<table><thead><tr><th>a</th></tr></thead><tbody><tr><td>b</td></tr></tbody></table>\n<p>after</p>"},
		{"after a paragraph", "intro\n| a |\n|---|",
			"<p>intro</p>\n<table><thead><tr><th>a</th></tr></thead></table>"},
		{"after a list", "- item\n| a |\n|---|",
			"<ul>\n<li>item</li>\n</ul>\n<table><thead><tr><th>a</th></tr></thead></table>"},
		{"ends at an image line", "| a |\n|---|\n![a|b](pic.jpg)",
			"<table><thead><tr><th>a</th></tr></thead></table>\n<!--attach:pic.jpg-->"},
	} {
		if got := ToHTML(c.in); got != c.want {
			t.Errorf("%s: ToHTML(%q)\ngot  %q\nwant %q", c.name, c.in, got, c.want)
		}
	}
}

// Text that only looks like part of a table renders as it did before tables.
func TestNotTables(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"one line", "| a | b |", "<p>| a | b |</p>"},
		{"no delimiter row", "| a | b |\n| c | d |", "<p>| a | b |<br>| c | d |</p>"},
		{"delimiter row alone", "|---|---|", "<p>|---|---|</p>"},
		{"cell counts differ", "| a | b |\n|---|", "<p>| a | b |<br>|---|</p>"},
		{"bad delimiter cell", "| a |\n| -x- |", "<p>| a |<br>| -x- |</p>"},
		{"delimiter without a pipe", "| a |\n---", "<p>| a |</p>\n<hr>"},
		{"header without a pipe", "a\n|---|", "<p>a<br>|---|</p>"},
		{"header is a list item", "- a | b\n|---|---|", "<ul>\n<li>a | b</li>\n</ul>\n<p>|---|---|</p>"},
		{"table inside a list", "- item\n  | a | b |\n  |---|---|",
			"<ul>\n<li>item</li>\n</ul>\n<p>| a | b |<br>|---|---|</p>"},
		{"inside a code block", "```\n| a |\n|---|\n```", "<pre>| a |\n|---|</pre>"},
		{"escaped pipe only", "a \\| b\n|---|", "<p>a \\| b<br>|---|</p>"},
	} {
		if got := ToHTML(c.in); got != c.want {
			t.Errorf("%s: ToHTML(%q)\ngot  %q\nwant %q", c.name, c.in, got, c.want)
		}
	}
}
