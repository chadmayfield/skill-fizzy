package fizzy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// PyIndent re-indents JSON the way Python's json.dumps(obj, indent=2) prints
// it: two spaces, key order kept, empty
// containers as [] and {}, non-ASCII escaped as \uXXXX, and floats written as
// Python's repr. Integers keep their digits. json.Indent alone differs on
// escaping and float text.
func PyIndent(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out bytes.Buffer
	if err := pyValue(dec, &out, 0); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after JSON value")
	}
	return out.Bytes(), nil
}

func pyValue(dec *json.Decoder, out *bytes.Buffer, depth int) error {
	tok, err := dec.Token()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return pyToken(dec, out, depth, tok)
}

func pyToken(dec *json.Decoder, out *bytes.Buffer, depth int, tok json.Token) error {
	pad := func(d int) { out.WriteString("\n" + strings.Repeat("  ", d)) }
	switch t := tok.(type) {
	case json.Delim:
		closer := byte(']')
		if t == '{' {
			closer = '}'
		}
		out.WriteRune(rune(t))
		first := true
		for dec.More() {
			if !first {
				out.WriteByte(',')
			}
			first = false
			pad(depth + 1)
			if t == '{' {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				ks, ok := k.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				out.WriteString(pyString(ks) + ": ")
			}
			if err := pyValue(dec, out, depth+1); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		if !first {
			pad(depth)
		}
		out.WriteByte(closer)
	case string:
		out.WriteString(pyString(t))
	case json.Number:
		out.WriteString(pyNumber(t.String()))
	case bool:
		out.WriteString(strconv.FormatBool(t))
	case nil:
		out.WriteString("null")
	}
	return nil
}

// pyNumber writes a JSON number as Python would after a load and dump: ints
// unchanged, floats as repr (1.0, 1e-05, 1e+16).
func pyNumber(s string) string {
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	e := strconv.FormatFloat(f, 'e', -1, 64)
	exp, _ := strconv.Atoi(e[strings.IndexByte(e, 'e')+1:])
	if exp >= -4 && exp < 16 {
		r := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(r, ".") {
			r += ".0"
		}
		return r
	}
	return e
}

// pyString quotes s like json.dumps with ensure_ascii: only space through ~
// stays literal, everything else is escaped.
func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		case r >= 0x10000:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, r1, r2)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pyRepr formats a string like Python's repr: single quotes, unless the
// string holds a single quote and no double quote.
func pyRepr(s string) string {
	q := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		q = '"'
	}
	var b strings.Builder
	b.WriteRune(q)
	for _, r := range s {
		switch {
		case r == q || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == ' ' || (r != unicode.ReplacementChar && unicode.IsPrint(r)):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteRune(q)
	return b.String()
}

// pyIntTuple formats ints as a Python tuple, for example (3, 7, 11).
func pyIntTuple(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
