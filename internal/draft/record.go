package draft

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrLinked is matched by errors.Is when a draft has more than one hard
// link. fz rewrites a draft by renaming a new file over it, which would
// leave the other links with the old text, so it refuses.
var ErrLinked = errors.New("the draft has more than one hard link, and fz would split them by rewriting it; copy it to a file of its own")

// bom is the UTF-8 byte order mark. Parse skips it and RecordFields keeps it.
const bom = "\xef\xbb\xbf"

// source is a draft file as read from disk, and the text Parse reads from it.
type source struct {
	raw  []byte
	text string // raw without a leading BOM, with every \r\n and lone \r turned into \n
	// rawAt[i] is the offset in raw of text[i], and rawAt[len(text)] is
	// len(raw), so a range of text maps back to the bytes it came from.
	rawAt []int
}

// readSource reads a file the way Python's read_text does, with universal
// newlines, and keeps the bytes so a rewrite can leave them alone.
func readSource(path string) (*source, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	i := 0
	if bytes.HasPrefix(raw, []byte(bom)) {
		i = len(bom)
	}
	var b strings.Builder
	at := make([]int, 0, len(raw)+1)
	for ; i < len(raw); i++ {
		at = append(at, i)
		if raw[i] == '\r' {
			b.WriteByte('\n')
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			continue
		}
		b.WriteByte(raw[i])
	}
	at = append(at, len(raw))
	return &source{raw: raw, text: b.String(), rawAt: at}, nil
}

// lineEnd returns the line ending of the first line of b: "\r\n", "\r" or
// "\n". A file with no line ending gets "\n".
func lineEnd(b []byte) string {
	i := bytes.IndexAny(b, "\r\n")
	switch {
	case i < 0:
		return "\n"
	case b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n':
		return "\r\n"
	case b[i] == '\r':
		return "\r"
	}
	return "\n"
}

// physicalLines splits b at \r\n, \n and a lone \r. Each line keeps its
// ending, and the last one has none.
func physicalLines(b []byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\n':
			out = append(out, string(b[start:i+1]))
			start = i + 1
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			out = append(out, string(b[start:i+1]))
			start = i + 1
		}
	}
	return append(out, string(b[start:]))
}

// subLines splits a line without its ending at the other separators that
// Python's splitlines also breaks on, such as \f and \v. Each piece keeps the
// separator after it.
func subLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		switch r {
		case '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			end := i + len(string(r))
			out = append(out, s[start:end])
			start = end
		}
	}
	return append(out, s[start:])
}

// trimEnding removes the line ending from s, or the separator subLines kept.
func trimEnding(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool {
		switch r {
		case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			return true
		}
		return false
	})
}

// RecordFields writes key: value lines into the front matter. An existing line
// for the same key (any case) is dropped, the rest keep their order, blank
// lines included, and the new lines go after the last line with content, in
// the order given. Every other byte of the file
// stays as it was, including a BOM and the file's line endings, which the new
// lines use too. The file is replaced by a rename, so a failed write leaves
// the old draft whole. A symlink is followed and stays a symlink.
func RecordFields(path string, fields ...KV) error {
	src, err := readSource(path)
	if err != nil {
		return err
	}
	m := frontRe.FindStringSubmatchIndex(src.text)
	if m == nil {
		return errf("%s: missing front matter", path)
	}
	start, end := src.rawAt[m[2]], src.rawAt[m[3]]
	drop := func(piece string) bool {
		lower := strings.ToLower(pyStrip(trimEnding(piece)))
		for _, f := range fields {
			if strings.HasPrefix(lower, strings.ToLower(f.Key)+":") {
				return true
			}
		}
		return false
	}
	var kept []string
	for _, line := range physicalLines(src.raw[start:end]) {
		body := trimEnding(line)
		ending := line[len(body):]
		var b strings.Builder
		for _, piece := range subLines(body) {
			if !drop(piece) {
				b.WriteString(piece)
			}
		}
		if b.Len() == 0 && body != "" {
			continue
		}
		kept = append(kept, b.String()+ending)
	}
	// The new lines go after the last line with content, so blank lines,
	// such as one before the closing ---, stay where they are.
	k := -1
	for i, line := range kept {
		if pyStrip(trimEnding(line)) != "" {
			k = i
		}
	}
	seq := append([]string(nil), kept[:k+1]...)
	for _, f := range fields {
		seq = append(seq, f.Key+": "+f.Value)
	}
	seq = append(seq, kept[k+1:]...)
	// Every line but the last ends with its own ending, or the file's. The
	// last has none, because the bytes after the front matter start with
	// the ending before the closing ---.
	eol := lineEnd(src.raw)
	var out bytes.Buffer
	out.Write(src.raw[:start])
	for i, line := range seq {
		e := endingOf(line)
		switch {
		case i == len(seq)-1:
			line = strings.TrimSuffix(line, e)
		case e == "":
			line += eol
		}
		out.WriteString(line)
	}
	out.Write(src.raw[end:])
	return writeAtomic(path, out.Bytes())
}

// endingOf returns the \r\n, \n or \r that s ends with, or "".
func endingOf(s string) string {
	switch {
	case strings.HasSuffix(s, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(s, "\n"):
		return "\n"
	case strings.HasSuffix(s, "\r"):
		return "\r"
	}
	return ""
}

// writeAtomic replaces the file at path, or the file a symlink there points
// to, with data: it writes a temp file in the same folder, syncs it, gives
// it the old file's mode, and its owner and group where the system allows,
// and renames it over the old file. A file with more than one hard link is
// refused with ErrLinked. ACLs and extended attributes are not copied.
func writeAtomic(path string, data []byte) (err error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(real)
	if err != nil {
		return err
	}
	if linkCount(st) > 1 {
		return fmt.Errorf("%s: %w", path, ErrLinked)
	}
	f, err := os.CreateTemp(filepath.Dir(real), "."+filepath.Base(real)+".fz-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = keepOwner(f, st); err != nil {
		return err
	}
	if err = f.Chmod(st.Mode().Perm()); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), real); err != nil {
		return err
	}
	// Sync the folder so the rename itself survives a crash. Some systems
	// cannot sync a folder, so a failure here is not an error.
	if dir, derr := os.Open(filepath.Dir(real)); derr == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}

// CheckWritable fails unless RecordFields could rewrite the draft at path:
// it must be a regular file (a symlink to one is fine) with one hard link,
// that fz can open for writing, in a folder where fz can make the temp
// file. A file with more than one hard link fails with ErrLinked. It
// changes nothing.
func CheckWritable(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(real)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if linkCount(st) > 1 {
		return ErrLinked
	}
	f, err := os.OpenFile(real, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	f.Close()
	tmp, err := os.CreateTemp(filepath.Dir(real), "."+filepath.Base(real)+".fz-*")
	if err != nil {
		return err
	}
	tmp.Close()
	return os.Remove(tmp.Name())
}

// RecordCard writes card: n into the draft's front matter.
func RecordCard(path string, n int) error {
	return RecordFields(path, KV{"card", strconv.Itoa(n)})
}
