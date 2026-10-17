package richtext

import (
	"strings"
	"testing"
)

func TestUnwrapActionText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"live server shape", "<div class=\"action-text-content\">\n  <h2>Goal</h2>\n<p>x</p>\n</div>\n", "<h2>Goal</h2>\n<p>x</p>"},
		{"no whitespace", `<div class="action-text-content"><p>x</p></div>`, "<p>x</p>"},
		{"surrounding whitespace", "\n  <div class=\"action-text-content\">\r\n\t<p>x</p>\r\n</div>  \n", "<p>x</p>"},
		{"nested div kept", `<div class="action-text-content"><div>in</div></div>`, "<div>in</div>"},
		{"not wrapped", "<p>x</p>", "<p>x</p>"},
		{"not wrapped, padded", "  <p>x</p>\n", "<p>x</p>"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		if got := UnwrapActionText(tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEscapeText(t *testing.T) {
	if got, want := escapeText(`A & B < C > D "q" 'a'`), `A &amp; B &lt; C &gt; D "q" 'a'`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplaceText(t *testing.T) {
	got, err := ReplaceText("<p>50% off</p>", "50% off", "up to 50% off", false)
	if err != nil || got != "<p>up to 50% off</p>" {
		t.Errorf("replace once: %q, %v", got, err)
	}
	got, err = ReplaceText("<p>A &amp; B</p>", "A & B", "A < B", false)
	if err != nil || got != "<p>A &lt; B</p>" {
		t.Errorf("escapes: %q, %v", got, err)
	}
	got, err = ReplaceText(`<p>say "hi"</p>`, `"hi"`, `'yo'`, false)
	if err != nil || got != `<p>say 'yo'</p>` {
		t.Errorf("quotes stay raw: %q, %v", got, err)
	}
	if _, err = ReplaceText("<p>x</p>", "y", "z", false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing: %v", err)
	}
}

func TestReplaceCountsSubstrings(t *testing.T) {
	html := "<p>Hinge H1 and H1S</p>"
	if _, err := ReplaceText(html, "H1", "H1S", false); err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Fatalf("want 2 times error, got %v", err)
	}
	got, err := ReplaceText(html, "Hinge H1 ", "Hinge H1S ", false)
	if err != nil || got != "<p>Hinge H1S and H1S</p>" {
		t.Errorf("longer phrase: %q, %v", got, err)
	}
	got, err = ReplaceText("<p>a a</p>", "a", "b", true)
	if err != nil || got != "<p>b b</p>" {
		t.Errorf("all: %q, %v", got, err)
	}
}
