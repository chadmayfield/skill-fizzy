package cli

import (
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

// pyRepr matches Python's repr() for the strings fz quotes in messages.
func TestPyRepr(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"gate", `'gate'`},
		{"Bob's gate", `"Bob's gate"`},
		{`Bob's "gate"`, `'Bob\'s "gate"'`},
		{`say "hi"`, `'say "hi"'`},
		{"a\\b", `'a\\b'`},
		{"a\nb\tc\rd", `'a\nb\tc\rd'`},
		{"\x1b[2K", `'\x1b[2K'`},
		{"\u00ad", `'\xad'`},
		{"\u2028", `'\u2028'`},
		{"\U0001F600", "'\U0001F600'"},
		{"\U000E0001", `'\U000e0001'`},
		// A real U+FFFD is printable, so Python prints it as it is.
		{"\uFFFD", "'\uFFFD'"},
		// A byte that is not UTF-8 reaches Python as a surrogate escape.
		{"a\xffb", `'a\udcffb'`},
	} {
		if got := pyRepr(tc.in); got != tc.want {
			t.Errorf("pyRepr(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := fizzy.PyRepr(tc.in); got != tc.want {
			t.Errorf("fizzy.PyRepr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
