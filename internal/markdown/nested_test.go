package markdown

import (
	"strings"
	"testing"
)

func TestNestedLists(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"two levels", "- gate\n  - hinge\n- latch", strings.Join([]string{
			"<ul>", "<li>gate<ul>", "<li>hinge</li>", "</ul></li>", "<li>latch</li>", "</ul>",
		}, "\n")},
		{"three levels", "- a\n  - b\n    - c", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b<ul>", "<li>c</li>", "</ul></li>", "</ul></li>", "</ul>",
		}, "\n")},
		{"depth 4 clamps to 3", "- a\n  - b\n    - c\n      - d\n    - e", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b<ul>", "<li>c</li>", "<li>d</li>", "<li>e</li>", "</ul></li>", "</ul></li>", "</ul>",
		}, "\n")},
		{"back to level two", "- a\n  - b\n    - c\n  - d\n- e", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b<ul>", "<li>c</li>", "</ul></li>", "<li>d</li>", "</ul></li>", "<li>e</li>", "</ul>",
		}, "\n")},
		{"ol in ul", "- a\n  1. one\n  2. two\n- b", strings.Join([]string{
			"<ul>", "<li>a<ol>", "<li>one</li>", "<li>two</li>", "</ol></li>", "<li>b</li>", "</ul>",
		}, "\n")},
		{"ul in ol needs the content column", "1. a\n   - sub\n2. b", strings.Join([]string{
			"<ol>", "<li>a<ul>", "<li>sub</li>", "</ul></li>", "<li>b</li>", "</ol>",
		}, "\n")},
		{"ul then ol at level two", "- a\n  - b\n  1. c", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b</li>", "</ul>", "<ol>", "<li>c</li>", "</ol></li>", "</ul>",
		}, "\n")},
		{"nested tasks", "- [ ] gate\n  - [x] hinge\n  - [ ] latch", strings.Join([]string{
			"<ul>", "<li>☐ gate<ul>", "<li>☑ hinge</li>", "<li>☐ latch</li>", "</ul></li>", "</ul>",
		}, "\n")},
		{"short of the content column", "1. a\n  - b", strings.Join([]string{
			"<ol>", "<li>a</li>", "</ol>", "<ul>", "<li>b</li>", "</ul>",
		}, "\n")},
		{"one space is short", "- a\n - b", strings.Join([]string{
			"<ul>", "<li>a</li>", "<li>b</li>", "</ul>",
		}, "\n")},
		{"tab does not nest", "- a\n\t- b", strings.Join([]string{
			"<ul>", "<li>a</li>", "<li>b</li>", "</ul>",
		}, "\n")},
		{"tab after a nested item", "- a\n  - b\n\t- c", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b</li>", "</ul></li>", "<li>c</li>", "</ul>",
		}, "\n")},
		{"blank line ends the list", "- a\n\n  - b", strings.Join([]string{
			"<ul>", "<li>a</li>", "</ul>", "<ul>", "<li>b</li>", "</ul>",
		}, "\n")},
		{"paragraph closes every level", "- a\n  - b\nend", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b</li>", "</ul></li>", "</ul>", "<p>end</p>",
		}, "\n")},
		{"wide marker gap", "-   a\n    - b", strings.Join([]string{
			"<ul>", "<li>a<ul>", "<li>b</li>", "</ul></li>", "</ul>",
		}, "\n")},
	} {
		if got := ToHTML(c.in); got != c.want {
			t.Errorf("%s: ToHTML(%q)\ngot  %q\nwant %q", c.name, c.in, got, c.want)
		}
	}
}
