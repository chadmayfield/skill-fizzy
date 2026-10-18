package config

import (
	"strings"
	"testing"
)

func TestSafeText(t *testing.T) {
	cases := map[string]string{
		"plain gate":                          "plain gate",
		"  \n hinge \t":                       "hinge",
		"a\x1b[2Jb":                           "ab",
		"a\x1b[1;31mred\x1b[0m":               "ared",
		"\x1b]0;pwned\x07title":               "title",
		"\x1b]8;;https://example.com\x1b\\ok": "ok",
		"x\u009b2Jy":                          "xy",
		"x\u009d0;t\u009cy":                   "xy",
		"bell\x07 nul\x00 del\x7f":            "bell nul del",
		"line1\r\nline2":                      "line1  line2",
		"\x1bcreset":                          "reset",
		"trailing esc\x1b":                    "trailing esc",
		"café 👍":                              "café 👍",
	}
	for in, want := range cases {
		if got := SafeText(in, 300); got != want {
			t.Errorf("SafeText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SafeText(strings.Repeat("é", 400), 300); got != strings.Repeat("é", 300) {
		t.Errorf("cap: got %d runes", len([]rune(got)))
	}
}

// TerminalText keeps newlines and tabs and removes everything else a
// terminal could act on. TerminalLine removes newlines and tabs too.
func TestTerminalText(t *testing.T) {
	ls := "\xe2\x80\xa8" // U+2028
	cases := []struct{ in, text, line string }{
		{"plain gate", "plain gate", "plain gate"},
		{"two\nlines\tand a tab", "two\nlines\tand a tab", "twolinesand a tab"},
		{"Fix the gate\x1b[2K\r#87    [Now] Harmless", "Fix the gate#87    [Now] Harmless", "Fix the gate#87    [Now] Harmless"},
		{"copy \x1b]52;c;Zm9v\x07me", "copy me", "copy me"},
		{"st \x1b]8;;https://example.com\x1b\\ok", "st ok", "st ok"},
		{"up\x1b[1A\x1b[2Kdown", "updown", "updown"},
		{"x\xc2\x9b2Jy", "xy", "xy"},
		{"bell\x07 nul\x00 del\x7f nel\xc2\x85", "bell nul del nel", "bell nul del nel"},
		{"crlf\r\nend", "crlf\nend", "crlfend"},
		{"sep" + ls + "end", "sepend", "sepend"},
		{"  spaces kept  ", "  spaces kept  ", "  spaces kept  "},
		{"trailing esc\x1b", "trailing esc", "trailing esc"},
		{"caf\xc3\xa9 \xf0\x9f\x91\x8d", "caf\xc3\xa9 \xf0\x9f\x91\x8d", "caf\xc3\xa9 \xf0\x9f\x91\x8d"},
	}
	for _, tc := range cases {
		if got := TerminalText(tc.in); got != tc.text {
			t.Errorf("TerminalText(%q) = %q, want %q", tc.in, got, tc.text)
		}
		if got := TerminalLine(tc.in); got != tc.line {
			t.Errorf("TerminalLine(%q) = %q, want %q", tc.in, got, tc.line)
		}
	}
}

// Unicode format characters (category Cf) are dropped: bidi overrides and
// isolates could make a name read differently from the one fz acts on, and
// zero-width characters hide text.
func TestCleanDropsFormatCharacters(t *testing.T) {
	for _, r := range []rune{
		0x202a, 0x202b, 0x202c, 0x202d, 0x202e, // bidi embeddings and overrides
		0x2066, 0x2067, 0x2068, 0x2069, // bidi isolates
		0x200b, 0x200c, 0x200d, 0x200e, 0x200f, // zero width characters and marks
		0x2060, 0xfeff, 0x00ad, // word joiner, BOM, soft hyphen
	} {
		in := "gate" + string(r) + "hinge"
		for name, f := range map[string]func(string) string{
			"TerminalText": TerminalText,
			"TerminalLine": TerminalLine,
			"SafeText":     func(s string) string { return SafeText(s, 300) },
		} {
			if got := f(in); got != "gatehinge" {
				t.Errorf("%s(%q) = %q, want %q", name, in, got, "gatehinge")
			}
		}
	}
}
