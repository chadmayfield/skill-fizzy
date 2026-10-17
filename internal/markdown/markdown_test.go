package markdown

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestToHTMLMatchesPython compares ToHTML with the Python converter's output.
// The .html files were written by the Python converter's to_html, which is
// no longer in this repo. Fix ToHTML, not the golden files.
func TestToHTMLMatchesPython(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no testdata/*.md files")
	}
	for _, src := range sources {
		t.Run(filepath.Base(src), func(t *testing.T) {
			md, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(src, ".md") + ".html")
			if err != nil {
				t.Fatalf("missing golden file: %v", err)
			}
			got := ToHTML(string(md))
			if got != string(want) {
				t.Errorf("output differs from Python\n%s", lineDiff(string(want), got))
			}
		})
	}
}

// lineDiff lists the lines that differ, with Python's line first.
func lineDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl || i >= len(w) || i >= len(g) {
			b.WriteString("line ")
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString("\n  python: ")
			b.WriteString(quoteLine(wl, i < len(w)))
			b.WriteString("\n  go:     ")
			b.WriteString(quoteLine(gl, i < len(g)))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func quoteLine(s string, present bool) string {
	if !present {
		return "(missing)"
	}
	return strings.NewReplacer("\x00", `\x00`, "\r", `\r`, "\t", `\t`).Replace(s)
}

func TestMarkdown(t *testing.T) {
	t.Run("basics", func(t *testing.T) {
		h := ToHTML("## Goal\nShip **it** by `Friday`.\n\n- [x] one\n- [ ] two\n\nSee [docs](https://ex.com/a).")
		for _, want := range []string{
			"<h3>Goal</h3>",
			"<strong>it</strong>",
			"<code>Friday</code>",
			"<li>☑ one</li>",
			`<a href="https://ex.com/a">docs</a>`,
		} {
			if !strings.Contains(h, want) {
				t.Errorf("missing %q in %q", want, h)
			}
		}
	})
	t.Run("escapes html", func(t *testing.T) {
		if h := ToHTML("<script>x</script>"); !strings.Contains(h, "&lt;script&gt;") {
			t.Errorf("not escaped: %q", h)
		}
	})
	t.Run("image placeholder", func(t *testing.T) {
		if h := ToHTML("![a photo](pic.jpg)"); !strings.Contains(h, "<!--attach:pic.jpg-->") {
			t.Errorf("no placeholder: %q", h)
		}
	})
}

func TestImageLine(t *testing.T) {
	cases := []struct {
		line, alt, target string
		ok                bool
	}{
		{"![a photo](pic.jpg)", "a photo", "pic.jpg", true},
		{"  ![x](dir/p.png)  \n", "x", "dir/p.png", true},
		{"![](a.jpg)", "", "a.jpg", true},
		{"![a](b c.jpg)", "", "", false},
		{"![a]()", "", "", false},
		{"text ![a](b.jpg)", "", "", false},
		{"![a](b.jpg) text", "", "", false},
		{"![a\nb](c.jpg)", "a\nb", "c.jpg", true},
	}
	for _, c := range cases {
		alt, target, ok := ImageLine(c.line)
		if alt != c.alt || target != c.target || ok != c.ok {
			t.Errorf("ImageLine(%q) = %q, %q, %v; want %q, %q, %v", c.line, alt, target, ok, c.alt, c.target, c.ok)
		}
	}
}

func TestInlineCodePlaceholderOutOfRange(t *testing.T) {
	// Python raises IndexError here; Go leaves the text alone.
	if got := Inline("a\x007\x00b"); got != "a\x007\x00b" {
		t.Errorf("got %q", got)
	}
}
