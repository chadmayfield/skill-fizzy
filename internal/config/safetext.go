package config

import (
	"strings"
	"unicode"
)

// SafeText makes text from a server or a command safe to print on a
// terminal, on one line. It cleans s as TerminalText does, with tabs,
// carriage returns and newlines turned into spaces. The result is trimmed
// and cut to at most max runes.
func SafeText(s string, max int) string {
	out := strings.TrimSpace(clean(s, spaceBreaks))
	if r := []rune(out); len(r) > max {
		out = strings.TrimSpace(string(r[:max]))
	}
	return out
}

// TerminalText makes server text safe to print on a terminal. It removes
// escape sequences (CSI, OSC and two-byte ESC forms, in their 7-bit and
// 8-bit spellings), every control character but newline and tab, the
// line and paragraph separators U+2028 and U+2029, and the Unicode format
// characters (category Cf), such as bidi overrides and isolates, zero-width
// characters and the BOM. A carriage return is removed, so a line cannot be
// overwritten.
func TerminalText(s string) string { return clean(s, keepBreaks) }

// TerminalLine cleans s as TerminalText does and removes newlines and tabs
// too, for text that must stay on its line, such as a card title.
func TerminalLine(s string) string { return clean(s, dropBreaks) }

// breaks says what clean does with tabs, newlines and carriage returns.
type breaks int

const (
	keepBreaks  breaks = iota // keep tabs and newlines, drop carriage returns
	dropBreaks                // drop all three
	spaceBreaks               // turn each into a space
)

// clean is the one function behind SafeText, TerminalText and TerminalLine.
func clean(s string, mode breaks) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '[', r == 0x9b:
			// CSI: parameter and intermediate bytes, then one final byte.
			if r == 0x1b {
				i++
			}
			for i+1 < len(rs) {
				i++
				if rs[i] >= 0x40 && rs[i] <= 0x7e {
					break
				}
			}
		case r == 0x1b && i+1 < len(rs) && strings.ContainsRune("]PX^_", rs[i+1]), r == 0x9d, r == 0x90, r == 0x9e, r == 0x9f:
			// OSC, DCS, SOS, PM and APC run to BEL or the string terminator.
			if r == 0x1b {
				i++
			}
			for i+1 < len(rs) {
				i++
				if rs[i] == 0x07 || rs[i] == 0x9c {
					break
				}
				if rs[i] == 0x1b && i+1 < len(rs) && rs[i+1] == '\\' {
					i++
					break
				}
			}
		case r == 0x1b:
			// Two-byte escape: drop ESC and the byte after it.
			i++
		case r == '\t' || r == '\n' || r == '\r':
			switch {
			case mode == spaceBreaks:
				b.WriteRune(' ')
			case mode == keepBreaks && r != '\r':
				b.WriteRune(r)
			}
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == '\u2028', r == '\u2029':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
