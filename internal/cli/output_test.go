package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/spf13/cobra"
)

func TestCardLine(t *testing.T) {
	cases := []struct {
		card fizzy.Card
		want string
	}{
		{fizzy.Card{Number: 87, Title: "Fix the gate", Column: &fizzy.Column{Name: "Now"}, Golden: true, Tags: []string{"buy", "waiting"}},
			"#87    [Now] Fix the gate ★ #buy #waiting"},
		{fizzy.Card{Number: 123456, Title: "Hinge", Closed: true, Column: &fizzy.Column{Name: "Now"}}, "#123456 [done] Hinge"},
		{fizzy.Card{Number: 5, Title: "Latch", Tags: []string{}}, "#5     [maybe] Latch"},
		{fizzy.Card{Number: 5, Title: "Latch", Column: &fizzy.Column{}}, "#5     [maybe] Latch"},
	}
	for _, tc := range cases {
		if got := cardLine(tc.card); got != tc.want {
			t.Errorf("cardLine = %q, want %q", got, tc.want)
		}
	}
}

// emitted runs emit on a command with --json set as given.
func emitted(t *testing.T, asJSON bool, data any, text string) string {
	t.Helper()
	root := NewRoot()
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "probe"}
	root.AddCommand(cmd)
	root.SetOut(&out)
	if asJSON {
		if err := root.PersistentFlags().Set("json", "true"); err != nil {
			t.Fatal(err)
		}
	}
	if err := emit(cmd, data, text); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The expected strings below are Python's json.dumps(..., indent=2) output,
// taken from python3 3.9.
func TestEmitBuiltValuesMatchPython(t *testing.T) {
	cases := []struct {
		name string
		data any
		want string
	}{
		{"null id and non-ASCII", obj{{"id", nil}, {"columns", []string{"Later", "Später"}}},
			"{\n  \"id\": null,\n  \"columns\": [\n    \"Later\",\n    \"Sp\\u00e4ter\"\n  ]\n}\n"},
		{"key order kept", obj{{"number", 5}, {"golden", false}}, "{\n  \"number\": 5,\n  \"golden\": false\n}\n"},
		{"empty object", obj{}, "{}\n"},
		{"empty list", rawList[fizzy.Card](nil), "[]\n"},
	}
	for _, tc := range cases {
		if got := emitted(t, true, tc.data, "text"); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEmitRawKeepsServerOrder(t *testing.T) {
	var c fizzy.Card
	if err := json.Unmarshal([]byte(`{"title":"Gate <café>","number":87,"zeta":1.0,"alpha":[]}`), &c); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"title\": \"Gate <caf\\u00e9>\",\n  \"number\": 87,\n  \"zeta\": 1.0,\n  \"alpha\": []\n}\n"
	if got := emitted(t, true, c.Raw(), "text"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	wantList := "[\n  {\n    \"title\": \"Gate <caf\\u00e9>\",\n    \"number\": 87,\n    \"zeta\": 1.0,\n    \"alpha\": []\n  }\n]\n"
	if got := emitted(t, true, rawList([]fizzy.Card{c}), "text"); got != wantList {
		t.Errorf("list: got %q, want %q", got, wantList)
	}
}

func TestEmitText(t *testing.T) {
	if got := emitted(t, false, obj{{"a", 1}}, "card #5 moved"); got != "card #5 moved\n" {
		t.Errorf("got %q", got)
	}
}
