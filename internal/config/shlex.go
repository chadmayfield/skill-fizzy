package config

import (
	"errors"
	"strings"
)

// SplitWords splits s into words the way Python's shlex.split does in POSIX
// mode: whitespace separates words, single quotes are literal, double quotes
// allow a backslash before a double quote or backslash, and outside quotes a
// backslash escapes the next character. It returns an error for an unclosed
// quote or a trailing backslash.
func SplitWords(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	const (
		plain = iota
		single
		double
	)
	state := plain
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch state {
		case single:
			if c == '\'' {
				state = plain
			} else {
				cur.WriteRune(c)
			}
		case double:
			switch {
			case c == '"':
				state = plain
			case c == '\\':
				if i+1 >= len(rs) {
					return nil, errors.New("no closing quotation")
				}
				if n := rs[i+1]; n == '"' || n == '\\' {
					cur.WriteRune(n)
					i++
				} else {
					cur.WriteRune(c)
				}
			default:
				cur.WriteRune(c)
			}
		default:
			switch {
			case c == ' ' || c == '\t' || c == '\n' || c == '\r':
				if inWord {
					words = append(words, cur.String())
					cur.Reset()
					inWord = false
				}
			case c == '\'':
				state = single
				inWord = true
			case c == '"':
				state = double
				inWord = true
			case c == '\\':
				if i+1 >= len(rs) {
					return nil, errors.New("no escaped character")
				}
				i++
				cur.WriteRune(rs[i])
				inWord = true
			default:
				cur.WriteRune(c)
				inWord = true
			}
		}
	}
	if state != plain {
		return nil, errors.New("no closing quotation")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}
