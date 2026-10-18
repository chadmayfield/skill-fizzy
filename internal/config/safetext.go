package config

import (
	"strings"
	"unicode"
)

// SafeText makes text from a server or a command safe to print on a
// terminal. It removes escape sequences (CSI, OSC and two-byte ESC forms, in
// their 7-bit and 8-bit spellings) and every other control character. Tabs,
// carriage returns and newlines become spaces. The result is trimmed and cut
// to at most max runes.
func SafeText(s string, max int) string {
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
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if r := []rune(out); len(r) > max {
		out = strings.TrimSpace(string(r[:max]))
	}
	return out
}
