package markdown

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/markdown/oldmd"
)

// indentedItem reports whether line is a list item that starts with
// whitespace, the only kind of list line whose output can change.
func indentedItem(line string) bool {
	if line == "" || !isSpace([]rune(line)[0]) {
		return false
	}
	if _, ok := bullet(line); ok {
		return true
	}
	_, ok := numbered(line)
	return ok
}

// Input with no ~, no | and no indented list items converts byte for byte
// as the frozen copy does.
func TestToHTMLMatchesOld(t *testing.T) {
	rng := rand.New(rand.NewSource(87))
	tokens := []string{
		"- ", "* ", "+ ", "1. ", "2) ", "10. ", "[ ] ", "[x] ", "[X] ",
		"**", "*", "_", "`", "```", "# ", "## ", "> ", "---", "***",
		"[a](https://example.com/x)", "https://example.com/a?b=1&c", "mailto:", "(", ")", "[", "]",
		"![a](b.jpg)", "a", "b c", " ", "  ", "\t", "<", ">", "&", "\"", "'", "é", "\\", ":", ".", "-", "1", "\x00",
	}
	n := 0
	for n < 20000 {
		var lines []string
		for range rng.Intn(8) + 1 {
			var b strings.Builder
			for range rng.Intn(7) {
				b.WriteString(tokens[rng.Intn(len(tokens))])
			}
			lines = append(lines, b.String())
		}
		skip := false
		for _, l := range lines {
			if indentedItem(l) {
				skip = true
			}
		}
		if skip {
			continue
		}
		n++
		sep := "\n"
		if rng.Intn(5) == 0 {
			sep = "\r\n"
		}
		in := strings.Join(lines, sep)
		if got, want := ToHTML(in), oldmd.ToHTML(in); got != want {
			t.Fatalf("%q:\ngot  %q\nwant %q", in, got, want)
		}
	}
}
