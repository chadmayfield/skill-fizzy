package config

import (
	"reflect"
	"testing"
)

// Expected values come from python3 shlex.split.
func TestSplitWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`a 'b c' "d e"`, []string{"a", "b c", "d e"}},
		{"  a   b\t\nc ", []string{"a", "b", "c"}},
		{`a\ b`, []string{"a b"}},
		{`"a\"b"`, []string{`a"b`}},
		{`"a\nb"`, []string{`a\nb`}},
		{`'a\nb'`, []string{`a\nb`}},
		{`a''b`, []string{"ab"}},
		{`''`, []string{""}},
		{`a""`, []string{"a"}},
		{`x'y z'w`, []string{"xy zw"}},
		{`"a b"c`, []string{"a bc"}},
		{`printf ' abc \n'`, []string{"printf", ` abc \n`}},
		{`a#b #c`, []string{"a#b", "#c"}},
		{`"a\\b"`, []string{`a\b`}},
		{``, nil},
	}
	for _, tt := range tests {
		got, err := SplitWords(tt.in)
		if err != nil {
			t.Errorf("SplitWords(%q) error: %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitWords(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestSplitWordsErrors(t *testing.T) {
	for _, in := range []string{`'abc`, `"abc`, `a\`} {
		if _, err := SplitWords(in); err == nil {
			t.Errorf("SplitWords(%q) expected error", in)
		}
	}
}
