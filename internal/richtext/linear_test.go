package richtext

import (
	"strings"
	"testing"
	"time"
)

// A description full of entities must not make each match scan every
// entity. 1 MB must take under a second, or 10 s under the race detector.
func TestReplaceTextLinear(t *testing.T) {
	html := strings.Repeat("&amp;", (1<<20)/5)
	for _, old := range []string{"amp", "&"} {
		start := time.Now()
		ReplaceText(html, old, "x", true)
		if d := time.Since(start); d > timeLimit {
			t.Errorf("%q: took %v", old, d)
		}
	}
}
