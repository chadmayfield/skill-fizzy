package markdown

import (
	"strings"
	"testing"
	"time"
)

// Inputs that made the inline parser scan ahead from every position. Each
// is about 1 MB and must convert in under a second, or 10 s under the race detector.
func TestToHTMLLinear(t *testing.T) {
	const size = 1 << 20
	for name, unit := range map[string]string{
		"open brackets":                "[",
		"labels without urls":          "[a]",
		"links without ends":           "[a](https://",
		"stars that never close":       ".*a",
		"underscores that never close": "._a",
		"tildes":                       "~~a",
		"tildes that never close":      "~~a ",
	} {
		in := strings.Repeat(unit, size/len(unit))
		start := time.Now()
		ToHTML(in)
		if d := time.Since(start); d > timeLimit {
			t.Errorf("%s: took %v", name, d)
		}
	}
}

// A wide header over many short rows must not pad every row to full width,
// or a few megabytes of draft would become gigabytes of empty cells.
func TestTablePaddingLinear(t *testing.T) {
	const cols = 1 << 18
	in := strings.Repeat("a|", cols) + "\n" + strings.Repeat("-|", cols) + "\n" + strings.Repeat("|\n", cols)
	start := time.Now()
	out := ToHTML(in)
	if d := time.Since(start); d > timeLimit {
		t.Errorf("took %v", d)
	}
	if len(out) > 20*len(in) {
		t.Errorf("%d bytes in, %d bytes out", len(in), len(out))
	}
}
