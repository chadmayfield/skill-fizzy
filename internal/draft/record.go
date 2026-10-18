package draft

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrLinked is matched by errors.Is when a draft has more than one hard
// link. fz rewrites a draft by renaming a new file over it, which would
// leave the other links with the old text, so it refuses.
var ErrLinked = errors.New("the draft has more than one hard link, and fz would split them by rewriting it; copy it to a file of its own")

// MaxFileBytes is the most fz reads from a draft or a --description-file:
// 4 MiB. A larger file is refused before it is read, so a huge file cannot
// hold up check, preview or apply.
const MaxFileBytes = 4 << 20

// ErrTooLarge is matched by errors.Is for a file over MaxFileBytes. Such
// an error also matches ErrDraft.
var ErrTooLarge = errors.New("file too large")

type tooLargeError struct{ msg string }

func (e *tooLargeError) Error() string { return e.msg }
func (e *tooLargeError) Is(target error) bool {
	return target == ErrTooLarge || target == ErrDraft
}

// ReadCapped reads a whole file of at most MaxFileBytes. A larger file is
// an ErrTooLarge error that names the path and the cap. A path that is not
// a regular file, such as a named pipe, is an ErrDraft error. The open does
// not block, so a pipe with no writer cannot hold fz.
func ReadCapped(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errf("%s is not a regular file, so fz does not read it", path)
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxFileBytes {
		return nil, &tooLargeError{fmt.Sprintf("%s is larger than 4 MiB; fz reads at most 4 MiB", path)}
	}
	return raw, nil
}

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
	raw, err := ReadCapped(path)
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

// RecordFields sets key: value lines in the front matter, replacing any line
// with the same key. Every other byte stays as it was, BOM and line endings
// included. The file is replaced atomically, so a failed write leaves it whole.
func RecordFields(path string, fields ...KV) error {
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	return rewriteFront(path, keys, fields)
}

// RecordFieldsDropping removes the front matter lines whose key is in drop,
// any case, then writes fields as RecordFields does.
func RecordFieldsDropping(path string, drop []string, fields ...KV) error {
	keys := append([]string(nil), drop...)
	for _, f := range fields {
		keys = append(keys, f.Key)
	}
	return rewriteFront(path, keys, fields)
}

// rewriteFront drops every front matter line whose key is in drop, any
// case, and writes fields as RecordFields does.
func rewriteFront(path string, drop []string, fields []KV) error {
	src, err := readSource(path)
	if err != nil {
		return err
	}
	m := frontRe.FindStringSubmatchIndex(src.text)
	if m == nil {
		return errf("%s: missing front matter", path)
	}
	start, end := src.rawAt[m[2]], src.rawAt[m[3]]
	// dropped reads a line's key the way parseFront does, so "creating :"
	// is the creating key here too.
	dropped := func(piece string) bool {
		k, _, ok := strings.Cut(trimEnding(piece), ":")
		if !ok {
			return false
		}
		key := strings.ToLower(pyStrip(k))
		for _, d := range drop {
			if key == strings.ToLower(d) {
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
			if !dropped(piece) {
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

// RecordCard writes card: n into the draft's front matter, in place of a
// creating: line.
func RecordCard(path string, n int) error {
	return rewriteFront(path, []string{"card", CreatingKey}, []KV{{"card", strconv.Itoa(n)}})
}

// CreatingKey is the front matter key draft apply writes, with the time,
// just before it sends a card create. It is replaced by card: when the
// create succeeds. A draft that still has it may already have its card.
const CreatingKey = "creating"

// RecordCreating writes creating: at into the draft's front matter.
func RecordCreating(path, at string) error {
	return RecordFields(path, KV{CreatingKey, at})
}

// ClearCreating removes the creating: line from the draft's front matter.
func ClearCreating(path string) error {
	return rewriteFront(path, []string{CreatingKey}, nil)
}
