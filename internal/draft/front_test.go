package draft

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseFrontText(t *testing.T, front string) (*Draft, error) {
	t.Helper()
	p := writeFile(t, t.TempDir(), "gate.md", "---\nboard: Home\n"+front+"\n---\n# Fix the gate\n")
	return Parse(p)
}

func TestCardValues(t *testing.T) {
	for value, want := range map[string]int{
		"87":     87,
		"#87":    87,
		"'#87'":  87,
		`"87"`:   87,
		"":       0,
		"  87  ": 87,
	} {
		d, err := parseFrontText(t, "card: "+value)
		if err != nil || d.Card != want {
			t.Errorf("card: %q gave %d, %v; want %d", value, d.Card, err, want)
		}
	}
}

func TestCardValueRefused(t *testing.T) {
	// "+87" and "-5" kill mutation M33, which drops the digits check.
	for _, value := range []string{"87.0", "87abc", "99999999999999999999", "+87", "-5", "0", "##87", "#87x", "eighty-seven"} {
		_, err := parseFrontText(t, "card: "+value)
		if !errors.Is(err, ErrBadValue) || !errors.Is(err, ErrDraft) {
			t.Errorf("card: %q: err = %v, want ErrBadValue", value, err)
			continue
		}
		if !strings.Contains(err.Error(), "'card: "+value+"'") {
			t.Errorf("card: %q: the error does not name the line: %v", value, err)
		}
	}
}

func TestGoldenValues(t *testing.T) {
	for value, want := range map[string]bool{
		"true": true, "TRUE": true, "Yes": true, "on": true, "ON": true, "1": true,
		"false": false, "False": false, "no": false, "Off": false, "0": false, "": false,
	} {
		d, err := parseFrontText(t, "golden: "+value)
		if err != nil || d.Golden != want {
			t.Errorf("golden: %q gave %v, %v; want %v", value, d.Golden, err, want)
		}
	}
	for _, value := range []string{"maybe", "y", "2", "truee"} {
		_, err := parseFrontText(t, "golden: "+value)
		if !errors.Is(err, ErrBadValue) || !strings.Contains(err.Error(), "'golden: "+value+"'") {
			t.Errorf("golden: %q: err = %v, want ErrBadValue naming the line", value, err)
		}
	}
}

func TestHashInsideQuotedValue(t *testing.T) {
	for line, want := range map[string]string{
		`board: "Gate # 1"`:           "Gate # 1",
		`board: 'Gate # 1'`:           "Gate # 1",
		`board: "Gate # 1"   # note`:  "Gate # 1",
		`board: Gate # 1`:             "Gate",
		`board: Gate #1`:              "Gate #1",
		`board: "Home"   # c`:         "Home",
		`board: Chad's gate # a note`: "Chad's gate",
	} {
		p := writeFile(t, t.TempDir(), "gate.md", "---\n"+line+"\n---\n# Fix the gate\n")
		d, err := Parse(p)
		if err != nil || d.Board != want {
			t.Errorf("%s: board %q, %v; want %q", line, d.Board, err, want)
		}
	}
}

func TestTagQuotes(t *testing.T) {
	for value, want := range map[string][]string{
		`['buy', 'gate']`:   {"buy", "gate"},
		`["#buy"]`:          {"buy"},
		`"#buy"`:            {"buy"},
		`'buy'`:             {"buy"},
		`['buy']`:           {"buy"},
		`["buy", 'gate']`:   {"buy", "gate"},
		`buy gate`:          {"buy gate"},
		`[#gate, "Hinge"]`:  {"gate", "hinge"},
		`["it's"]`:          {"it's"},
		`['#Buy', "#Gate"]`: {"buy", "gate"},
	} {
		d, err := parseFrontText(t, "tags: "+value)
		if err != nil || !reflect.DeepEqual(d.Tags, want) {
			t.Errorf("tags: %s gave %q, %v; want %q", value, d.Tags, err, want)
		}
	}
}

func TestHeadingsInsideFenceAreNotStructure(t *testing.T) {
	text := "---\nboard: Home\n---\n```text\n# Not the title\n```\n# Fix the gate\n\n```sh\n## Steps\n- [ ] not a step\n```\n\n## Steps\n- [ ] Order the hinge\n"
	d, err := Parse(writeFile(t, t.TempDir(), "gate.md", text))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Fix the gate" {
		t.Errorf("title = %q", d.Title)
	}
	if !reflect.DeepEqual(d.Steps, []Step{{"Order the hinge", false}}) {
		t.Errorf("steps = %v", d.Steps)
	}
	if want := "```text\n# Not the title\n```\n\n```sh\n## Steps\n- [ ] not a step\n```"; d.BodyMD != want {
		t.Errorf("body = %q, want %q", d.BodyMD, want)
	}
}

// Mutation M55: a folder with the image's name is not an image.
func TestResolveImageRejectsFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "hinge.jpg"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := &Draft{Path: filepath.Join(dir, "gate.md")}
	if p, err := d.ResolveImage("hinge.jpg", filepath.Join(dir, "inbox")); err == nil {
		t.Errorf("a folder resolved as an image: %s", p)
	}
}
