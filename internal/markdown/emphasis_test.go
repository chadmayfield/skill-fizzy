package markdown

import (
	"math/rand"
	"strings"
	"testing"
)

// emphasisByScan is the emphasis code before closers were precomputed. It
// scans from every opener to the end of the line. The test keeps it to
// show the faster code gives the same output.
func emphasisByScan(text string, marker rune, blocked func(rune) bool) string {
	r := []rune(text)
	var b strings.Builder
scan:
	for i := 0; i < len(r); {
		if r[i] == marker && (i == 0 || !blocked(r[i-1])) && i+1 < len(r) && !isSpace(r[i+1]) {
			for k := i + 1; k < len(r) && r[k] != '\n'; k++ {
				j := k + 1
				if j < len(r) && r[j] == marker && !isSpace(r[k]) && (j+1 >= len(r) || !blocked(r[j+1])) {
					b.WriteString("<em>" + string(r[i+1:j]) + "</em>")
					i = j + 1
					continue scan
				}
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

// The precomputed closers give byte for byte what the scan gave.
func TestEmphasisMatchesScan(t *testing.T) {
	rng := rand.New(rand.NewSource(87))
	alphabet := []rune("**__ a.\n\tbé")
	star := func(c rune) bool { return c == '*' || isWord(c) }
	for n := 0; n < 50000; n++ {
		in := make([]rune, rng.Intn(24))
		for i := range in {
			in[i] = alphabet[rng.Intn(len(alphabet))]
		}
		s := string(in)
		if got, want := emphasis(s, '*', star), emphasisByScan(s, '*', star); got != want {
			t.Fatalf("star %q: got %q, want %q", s, got, want)
		}
		if got, want := emphasis(s, '_', isWord), emphasisByScan(s, '_', isWord); got != want {
			t.Fatalf("underscore %q: got %q, want %q", s, got, want)
		}
	}
}
