// Package markdown converts card-draft Markdown to the HTML Fizzy stores.
//
// It is a port of the Python fz's markdown.py (removed in 562a4a9) and must
// produce the same bytes.
// It supports what card drafts need and nothing more: ATX headings,
// paragraphs, bullet and numbered lists (one level), task-list items, fenced
// code blocks, blockquotes, links, bold, italic, inline code, horizontal
// rules, and image lines. Fizzy sanitizes rich text on its side as well.
//
// A line that is only ![alt](target) becomes the placeholder
// <!--attach:target-->, which the draft layer replaces with an uploaded file.
//
// The Python version uses regular expressions with lookbehind and lookahead,
// which Go's regexp package does not have, so the matching here is written
// out by hand. Each function names the Python pattern it replaces. Character
// classes follow Python 3 str patterns: \s is isSpace, \w is isWord and \d is
// unicode.IsDigit.
package markdown

import (
	"html"
	"strconv"
	"strings"
	"unicode"
)

// ToHTML converts a draft body to HTML.
func ToHTML(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var out, para []string
	listTag := "" // "ul" or "ol"

	flushPara := func() {
		if len(para) > 0 {
			parts := make([]string, len(para))
			for i, p := range para {
				parts[i] = Inline(p)
			}
			out = append(out, "<p>"+strings.Join(parts, "<br>")+"</p>")
			para = para[:0]
		}
	}
	closeList := func() {
		if listTag != "" {
			out = append(out, "</"+listTag+">")
			listTag = ""
		}
	}

	for i := 0; i < len(lines); {
		line := lines[i]
		stripped := strip(line)

		if strings.HasPrefix(stripped, "```") {
			flushPara()
			closeList()
			var code []string
			i++
			for i < len(lines) && !strings.HasPrefix(strip(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			out = append(out, "<pre>"+escape(strings.Join(code, "\n"), true)+"</pre>")
			i++
			continue
		}

		if stripped == "" {
			flushPara()
			closeList()
			i++
			continue
		}

		if _, target, ok := ImageLine(line); ok {
			flushPara()
			closeList()
			out = append(out, "<!--attach:"+target+"-->")
			i++
			continue
		}

		if hashes, text, ok := heading(stripped); ok {
			flushPara()
			closeList()
			// The card title is the h1, so sections start at h2.
			level := strconv.Itoa(min(hashes+1, 4))
			out = append(out, "<h"+level+">"+Inline(text)+"</h"+level+">")
			i++
			continue
		}

		if isRule(stripped) {
			flushPara()
			closeList()
			out = append(out, "<hr>")
			i++
			continue
		}

		if strings.HasPrefix(stripped, ">") {
			flushPara()
			closeList()
			var quote []string
			for i < len(lines) && strings.HasPrefix(strip(lines[i]), ">") {
				quote = append(quote, strip(strip(lines[i])[1:]))
				i++
			}
			out = append(out, "<blockquote>"+Inline(strings.Join(quote, " "))+"</blockquote>")
			continue
		}

		item, isBullet := bullet(line)
		if !isBullet {
			var isNumbered bool
			item, isNumbered = numbered(line)
			if !isNumbered {
				closeList()
				para = append(para, stripped)
				i++
				continue
			}
		}
		flushPara()
		tag := "ol"
		if isBullet {
			tag = "ul"
		}
		if listTag != tag {
			closeList()
			out = append(out, "<"+tag+">")
			listTag = tag
		}
		if checked, text, ok := task(item); ok {
			box := "☐"
			if checked {
				box = "☑"
			}
			out = append(out, "<li>"+box+" "+Inline(text)+"</li>")
		} else {
			out = append(out, "<li>"+Inline(item)+"</li>")
		}
		i++
	}

	flushPara()
	closeList()
	return strings.Join(out, "\n")
}

// ImageLine reports whether line is only an image, ![alt](target), and
// returns its parts. Python: ^\s*!\[([^\]]*)\]\(([^)\s]+)\)\s*$
func ImageLine(line string) (alt, target string, ok bool) {
	r := []rune(line)
	i := skipSpace(r, 0)
	if i+1 >= len(r) || r[i] != '!' || r[i+1] != '[' {
		return "", "", false
	}
	altStart := i + 2
	j := altStart
	for j < len(r) && r[j] != ']' {
		j++
	}
	if j+1 >= len(r) || r[j+1] != '(' {
		return "", "", false
	}
	tStart := j + 2
	k := tStart
	for k < len(r) && r[k] != ')' && !isSpace(r[k]) {
		k++
	}
	if k == tStart || k >= len(r) || r[k] != ')' {
		return "", "", false
	}
	if skipSpace(r, k+1) != len(r) {
		return "", "", false
	}
	return string(r[altStart:j]), string(r[tStart:k]), true
}

// heading matches ^(#{1,6})\s+(.*?)\s*#*\s*$ on a stripped line.
func heading(s string) (hashes int, text string, ok bool) {
	r := []rune(s)
	n := 0
	for n < len(r) && r[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(r) || !isSpace(r[n]) {
		return 0, "", false
	}
	start := skipSpace(r, n)
	// The lazy group ends where the longest tail of the form \s*#*\s* begins.
	end := len(r)
	for end > start && isSpace(r[end-1]) {
		end--
	}
	for end > start && r[end-1] == '#' {
		end--
	}
	for end > start && isSpace(r[end-1]) {
		end--
	}
	return n, string(r[start:end]), true
}

// isRule matches ^\s*([-*_])(\s*\1){2,}\s*$: three or more of the same
// marker, with any whitespace between them.
func isRule(s string) bool {
	var marker rune
	count := 0
	for _, c := range s {
		if isSpace(c) {
			continue
		}
		if count == 0 {
			if c != '-' && c != '*' && c != '_' {
				return false
			}
			marker = c
		} else if c != marker {
			return false
		}
		count++
	}
	return count >= 3
}

// bullet matches ^\s*[-*+]\s+(.*)$ and returns the group.
func bullet(line string) (string, bool) {
	r := []rune(line)
	i := skipSpace(r, 0)
	if i >= len(r) || (r[i] != '-' && r[i] != '*' && r[i] != '+') {
		return "", false
	}
	return afterSpace(r, i+1)
}

// numbered matches ^\s*\d+[.)]\s+(.*)$ and returns the group.
func numbered(line string) (string, bool) {
	r := []rune(line)
	i := skipSpace(r, 0)
	d := i
	for d < len(r) && unicode.IsDigit(r[d]) {
		d++
	}
	if d == i || d >= len(r) || (r[d] != '.' && r[d] != ')') {
		return "", false
	}
	return afterSpace(r, d+1)
}

// afterSpace matches \s+(.*)$ at r[i:] and returns the group. The input
// never holds a newline, so (.*) takes the rest.
func afterSpace(r []rune, i int) (string, bool) {
	if i >= len(r) || !isSpace(r[i]) {
		return "", false
	}
	return string(r[skipSpace(r, i):]), true
}

// task matches ^\[([ xX])\]\s+(.*)$ on a list item.
func task(item string) (checked bool, text string, ok bool) {
	r := []rune(item)
	if len(r) < 3 || r[0] != '[' || r[2] != ']' || (r[1] != ' ' && r[1] != 'x' && r[1] != 'X') {
		return false, "", false
	}
	text, ok = afterSpace(r, 3)
	return r[1] != ' ', text, ok
}

// Inline escapes text, then applies inline code, links, bold and italic.
func Inline(text string) string {
	var codes []string
	text = stashCode(text, &codes)
	text = escape(text, false)
	text = links(text)
	text = bareURLs(text, &codes)
	text = bold(text)
	text = starEm(text)
	text = underscoreEm(text)
	return restoreCode(text, codes)
}

// stashCode replaces `([^`]+)` with \x00N\x00 and keeps the escaped
// <code> element in codes.
func stashCode(text string, codes *[]string) string {
	r := []rune(text)
	var b strings.Builder
	for i := 0; i < len(r); {
		if r[i] == '`' {
			j := i + 1
			for j < len(r) && r[j] != '`' {
				j++
			}
			if j < len(r) && j > i+1 {
				*codes = append(*codes, "<code>"+escape(string(r[i+1:j]), true)+"</code>")
				b.WriteString("\x00" + strconv.Itoa(len(*codes)-1) + "\x00")
				i = j + 1
				continue
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

// restoreCode replaces \x00(\d+)\x00 with the stashed code. Python raises
// IndexError on an index it never stashed, which only happens when the input
// itself holds NUL bytes; Go leaves that text as it is. Python's \d also takes
// non-ASCII digits there; Go takes ASCII only, again reachable only from NULs
// in the input.
func restoreCode(text string, codes []string) string {
	if !strings.Contains(text, "\x00") {
		return text
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(text, 0)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		j := i + 1
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j > i+1 && j < len(text) && text[j] == 0 {
			if n, err := strconv.Atoi(text[i+1 : j]); err == nil && n < len(codes) {
				b.WriteString(text[:i])
				b.WriteString(codes[n])
				text = text[j+1:]
				continue
			}
		}
		b.WriteString(text[:i+1])
		text = text[i+1:]
	}
}

// links replaces [text](https?://...|mailto:...) with an anchor. Python:
// \[([^\]]+)\]\((https?://[^)\s]+|mailto:[^)\s]+)\)
func links(text string) string {
	r := []rune(text)
	// next[k] is the index of the first ] at or after k, and stop[k] the
	// first ) or space, or len(r). linkAt reads them instead of scanning
	// ahead from every [, which made a line of brackets quadratic.
	next, stop := make([]int, len(r)+1), make([]int, len(r)+1)
	next[len(r)], stop[len(r)] = len(r), len(r)
	for k := len(r) - 1; k >= 0; k-- {
		next[k], stop[k] = next[k+1], stop[k+1]
		if r[k] == ']' {
			next[k] = k
		}
		if r[k] == ')' || isSpace(r[k]) {
			stop[k] = k
		}
	}
	var b strings.Builder
	for i := 0; i < len(r); {
		if r[i] == '[' {
			if end, label, url, ok := linkAt(r, i, next, stop); ok {
				b.WriteString(`<a href="` + hrefFrom(url, true) + `">` + label + "</a>")
				i = end
				continue
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

func linkAt(r []rune, i int, next, stop []int) (end int, label, url string, ok bool) {
	j := next[i+1]
	if j == i+1 || j+1 >= len(r) || r[j+1] != '(' {
		return 0, "", "", false
	}
	start := j + 2
	p := schemeEnd(r, start, true)
	if p < 0 {
		return 0, "", "", false
	}
	q := stop[p]
	if q == p || q >= len(r) || r[q] != ')' {
		return 0, "", "", false
	}
	return q + 1, string(r[i+1 : j]), string(r[start:q]), true
}

// schemeEnd returns the index after "http://", "https://" or, when
// mailto is set, "mailto:" at r[i:], or -1.
func schemeEnd(r []rune, i int, mailto bool) int {
	switch {
	case hasPrefix(r, i, "https://"):
		return i + len("https://")
	case hasPrefix(r, i, "http://"):
		return i + len("http://")
	case mailto && hasPrefix(r, i, "mailto:"):
		return i + len("mailto:")
	}
	return -1
}

func hasPrefix(r []rune, i int, prefix string) bool {
	for _, c := range prefix {
		if i >= len(r) || r[i] != c {
			return false
		}
		i++
	}
	return true
}

// hrefFrom turns a URL taken from text that Inline already escaped into an
// href value with each & escaped exactly once. It undoes Inline's escape to
// get the draft's own text, reads a written &amp; as &, and escapes the
// result once. quote also escapes quotes.
func hrefFrom(escaped string, quote bool) string {
	raw := html.UnescapeString(escaped)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	return escape(raw, quote)
}

// bareURLs links a URL that is not already inside an anchor. Python:
// (?<!["=>])\b(https?://[^\s<]+[^\s<.,;:!?)])
// Unlike Python, the URL also ends at a quote and at an escaped < or >,
// because Python's pattern let a " close the href and add an attribute.
//
// It also stashes each anchor's opening tag, and each bare URL's whole
// anchor, in codes the way stashCode does, so bold and emphasis never run
// inside a URL. A link label stays in the text and still gets them.
func bareURLs(text string, codes *[]string) string {
	r := []rune(text)
	var b strings.Builder
	stash := func(s string) {
		*codes = append(*codes, restoreCode(s, *codes))
		b.WriteString("\x00" + strconv.Itoa(len(*codes)-1) + "\x00")
	}
	for i := 0; i < len(r); {
		// Text is escaped by now, so a raw < is only ever a tag that links
		// wrote, and its href holds no raw quote.
		if hasPrefix(r, i, `<a href="`) {
			if end := indexFrom(r, i, `">`); end >= 0 {
				stash(string(r[i : end+2]))
				i = end + 2
				continue
			}
		}
		if r[i] == 'h' && (i == 0 || (r[i-1] != '"' && r[i-1] != '=' && r[i-1] != '>' && !isWord(r[i-1]))) {
			if p := schemeEnd(r, i, false); p >= 0 {
				q := p
				// A URL ends at whitespace, a quote, or a < or > (escaped by
				// now, or a tag that links added), so it cannot close the
				// href attribute.
				for q < len(r) && !isSpace(r[q]) && !strings.ContainsRune("<\"'", r[q]) &&
					!hasPrefix(r, q, "&lt;") && !hasPrefix(r, q, "&gt;") {
					q++
				}
				// The last character may not be trailing punctuation, and
				// [^\s<]+ needs at least one character before it.
				e := q - 1
				for e > p && strings.ContainsRune(".,;:!?)", r[e]) {
					e--
				}
				if e > p {
					url := string(r[i : e+1])
					stash(`<a href="` + hrefFrom(url, true) + `">` + url + "</a>")
					i = e + 1
					continue
				}
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

// indexFrom returns the index of sub in r at or after i, or -1.
func indexFrom(r []rune, i int, sub string) int {
	for ; i < len(r); i++ {
		if hasPrefix(r, i, sub) {
			return i
		}
	}
	return -1
}

// bold replaces \*\*(.+?)\*\* with <strong>.
func bold(text string) string {
	r := []rune(text)
	var b strings.Builder
scan:
	for i := 0; i < len(r); {
		if r[i] == '*' && i+1 < len(r) && r[i+1] == '*' {
			for k := i + 2; k < len(r) && r[k] != '\n'; k++ {
				if k+2 < len(r) && r[k+1] == '*' && r[k+2] == '*' {
					b.WriteString("<strong>" + string(r[i+2:k+1]) + "</strong>")
					i = k + 3
					continue scan
				}
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

// starEm replaces (?<![*\w])\*(?!\s)(.+?)(?<!\s)\*(?![*\w]) with <em>.
func starEm(text string) string {
	return emphasis(text, '*', func(c rune) bool { return c == '*' || isWord(c) })
}

// underscoreEm replaces (?<!\w)_(?!\s)(.+?)(?<!\s)_(?!\w) with <em>.
func underscoreEm(text string) string {
	return emphasis(text, '_', isWord)
}

// emphasis wraps marker-delimited runs in <em>. A marker opens when the
// character before it is not blocked and the one after is not whitespace. It
// closes at the first later marker that follows a non-space character and is
// not followed by a blocked character. The run may not cross a newline.
func emphasis(text string, marker rune, blocked func(rune) bool) string {
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

// escape matches Python's html.escape: &, < and > always, and " and ' when
// quote is set.
func escape(s string, quote bool) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c == '&':
			b.WriteString("&amp;")
		case c == '<':
			b.WriteString("&lt;")
		case c == '>':
			b.WriteString("&gt;")
		case c == '"' && quote:
			b.WriteString("&quot;")
		case c == '\'' && quote:
			b.WriteString("&#x27;")
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// strip matches Python's str.strip() with no arguments.
func strip(s string) string {
	return strings.TrimFunc(s, isSpace)
}

func skipSpace(r []rune, i int) int {
	for i < len(r) && isSpace(r[i]) {
		i++
	}
	return i
}

// isSpace matches \s in a Python 3 str pattern, which is also the set
// str.strip() removes. It is wider than unicode.IsSpace: it includes the
// file, group, record and unit separators U+001C to U+001F.
func isSpace(c rune) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', 0x1c, 0x1d, 0x1e, 0x1f, ' ',
		0x85, 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return c >= 0x2000 && c <= 0x200a
}

// isWord matches \w in a Python 3 str pattern: str.isalnum() or "_".
func isWord(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsNumber(c)
}
