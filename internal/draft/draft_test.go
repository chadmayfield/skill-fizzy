package draft

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sample = `---
board: Garden
column: Next
tags: build, buy
---
# Fix the gate

## Links & media
- [Guide](https://example.com/guide)
![rear](rear.jpg)

## Steps
- [ ] Measure
- [x] Order
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseAndRecord(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "rear.md", sample)
	writeFile(t, dir, "rear.jpg", "\xff\xd8")
	d, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Fix the gate" || d.Board != "Garden" || d.Column != "Next" {
		t.Fatalf("got %q %q %q", d.Title, d.Board, d.Column)
	}
	if !reflect.DeepEqual(d.Tags, []string{"build", "buy"}) {
		t.Fatalf("tags %v", d.Tags)
	}
	if !reflect.DeepEqual(d.Steps, []Step{{"Measure", false}, {"Order", true}}) {
		t.Fatalf("steps %v", d.Steps)
	}
	if !reflect.DeepEqual(d.Links(), []string{"https://example.com/guide"}) {
		t.Fatalf("links %v", d.Links())
	}
	got, err := d.ResolveImage("rear.jpg", filepath.Join(dir, "inbox"))
	if err != nil || got != filepath.Join(dir, "rear.jpg") {
		t.Fatalf("resolve %q %v", got, err)
	}
	html := d.DescriptionHTML(map[string]string{"rear.jpg": "SG"})
	if !strings.Contains(html, `<action-text-attachment sgid="SG">`) {
		t.Fatalf("no attachment in %s", html)
	}
	if strings.Contains(html, "Measure") {
		t.Fatal("steps leaked into the description")
	}
	if err := RecordCard(p, 42); err != nil {
		t.Fatal(err)
	}
	d2, err := Parse(p)
	if err != nil || d2.Card != 42 {
		t.Fatalf("card %d %v", d2.Card, err)
	}
}

func TestDescriptionHTMLWithoutSGID(t *testing.T) {
	dir := t.TempDir()
	d, err := Parse(writeFile(t, dir, "rear.md", sample))
	if err != nil {
		t.Fatal(err)
	}
	if html := d.DescriptionHTML(nil); !strings.Contains(html, "<p>[image: rear.jpg]</p>") {
		t.Fatalf("got %s", html)
	}
}

func TestImageInListItemIsRejected(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "x.md", "---\nboard: B\n---\n# T\n\n## Links & media\n- ![a photo](pic.jpg)\n")
	writeFile(t, dir, "pic.jpg", "\xff\xd8")
	d, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Images()) != 0 {
		t.Fatalf("images %v", d.Images())
	}
	err = d.CheckImages(dir)
	if err == nil || !strings.Contains(err.Error(), "on a line by itself") || !errors.Is(err, ErrDraft) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckImagesMissingFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "x.md", "---\nboard: B\n---\n# T\n\n![a](nope.jpg)\n")
	d, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	err = d.CheckImages(filepath.Join(dir, "inbox"))
	if err == nil || !strings.Contains(err.Error(), "image not found: nope.jpg") || !errors.Is(err, ErrDraft) {
		t.Fatalf("got %v", err)
	}
	writeFile(t, dir, "nope.jpg", "x")
	if err := d.CheckImages(filepath.Join(dir, "inbox")); err != nil {
		t.Fatal(err)
	}
}

func TestResolveImageFallsBackToInbox(t *testing.T) {
	dir := t.TempDir()
	inbox := filepath.Join(dir, "inbox")
	if err := os.Mkdir(inbox, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, inbox, "a.jpg", "x")
	d := &Draft{Path: filepath.Join(dir, "x.md")}
	got, err := d.ResolveImage("a.jpg", inbox)
	if err != nil || got != filepath.Join(inbox, "a.jpg") {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestRequiresBoardAndTitle(t *testing.T) {
	dir := t.TempDir()
	for _, text := range []string{"---\ncolumn: Now\n---\n# T\n", "---\nboard: B\n---\nno title\n"} {
		p := writeFile(t, dir, "x.md", text)
		if _, err := Parse(p); !errors.Is(err, ErrDraft) {
			t.Fatalf("%q: got %v", text, err)
		}
	}
}

func TestBadStepLineRejected(t *testing.T) {
	p := writeFile(t, t.TempDir(), "x.md", "---\nboard: B\n---\n# T\n## Steps\n- not a checkbox\n")
	if _, err := Parse(p); !errors.Is(err, ErrDraft) {
		t.Fatalf("got %v", err)
	}
}

func TestParseMissingFile(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "absent.md"))
	if err == nil || errors.Is(err, ErrDraft) {
		t.Fatalf("want a plain read error, got %v", err)
	}
}

type goldenStep struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

type golden struct {
	Error  bool              `json:"error"`
	Title  string            `json:"title"`
	Board  string            `json:"board"`
	Column string            `json:"column"`
	Tags   []string          `json:"tags"`
	Golden bool              `json:"golden"`
	Card   *int              `json:"card"`
	BodyMD string            `json:"body_md"`
	Steps  []goldenStep      `json:"steps"`
	Links  []string          `json:"links"`
	Images []string          `json:"images"`
	Meta   map[string]string `json:"meta"`
}

func readGolden(t *testing.T, md string) golden {
	t.Helper()
	raw, err := os.ReadFile(strings.TrimSuffix(md, ".md") + ".json")
	if err != nil {
		t.Fatalf("missing golden file: %v", err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// The .json files were written from the Python draft parser, which is no
// longer in this repo. Fix Parse, not the golden files.
func TestGoldenFromPython(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("no testdata/*.md files (%v)", err)
	}
	for _, src := range sources {
		t.Run(filepath.Base(src), func(t *testing.T) {
			want := readGolden(t, src)
			d, err := Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			card := 0
			if want.Card != nil {
				card = *want.Card
			}
			steps := []Step{}
			for _, s := range want.Steps {
				steps = append(steps, Step{s.Content, s.Done})
			}
			checks := []struct {
				name      string
				got, want any
			}{
				{"title", d.Title, want.Title},
				{"board", d.Board, want.Board},
				{"column", d.Column, want.Column},
				{"tags", nonNil(d.Tags), nonNil(want.Tags)},
				{"golden", d.Golden, want.Golden},
				{"card", d.Card, card},
				{"body_md", d.BodyMD, want.BodyMD},
				{"steps", d.Steps, steps},
				{"links", nonNil(d.Links()), nonNil(want.Links)},
				{"images", nonNil(d.Images()), nonNil(want.Images)},
				{"meta", d.Meta, want.Meta},
			}
			for _, c := range checks {
				if !reflect.DeepEqual(c.got, c.want) {
					t.Errorf("%s: got %#v, want %#v", c.name, c.got, c.want)
				}
			}
		})
	}
}

func TestGoldenErrors(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("testdata", "errors", "*.md"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("no testdata/errors/*.md files (%v)", err)
	}
	for _, src := range sources {
		t.Run(filepath.Base(src), func(t *testing.T) {
			if !readGolden(t, src).Error {
				t.Fatal("golden does not record an error")
			}
			if _, err := Parse(src); !errors.Is(err, ErrDraft) {
				t.Fatalf("got %v, want ErrDraft", err)
			}
		})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func TestPlanSteps(t *testing.T) {
	wanted := []Step{{"Measure x", false}, {"Order hinge", false}}
	existing := []CardStep{
		{"S1", "Measure x", false},
		{"S2", "Old step", false},
		{"S3", "Done elsewhere", true},
	}
	plan := PlanSteps(wanted, existing)
	if !reflect.DeepEqual(plan.Add, []Step{{"Order hinge", false}}) {
		t.Errorf("add %v", plan.Add)
	}
	if len(plan.Tick) != 0 {
		t.Errorf("tick %v", plan.Tick)
	}
	if !reflect.DeepEqual(plan.Extra, []CardStep{existing[1], existing[2]}) {
		t.Errorf("extra %v", plan.Extra)
	}
}

func TestPlanStepsTicksDraftDone(t *testing.T) {
	plan := PlanSteps([]Step{{"Measure", true}}, []CardStep{{"S1", "Measure", false}})
	if !reflect.DeepEqual(plan.Tick, []string{"S1"}) || len(plan.Add) != 0 || len(plan.Extra) != 0 {
		t.Errorf("plan %+v", plan)
	}
}

func TestPlanStepsNeverUnticks(t *testing.T) {
	plan := PlanSteps([]Step{{"Measure", false}}, []CardStep{{"S1", "Measure", true}})
	if len(plan.Add) != 0 || len(plan.Tick) != 0 || len(plan.Extra) != 0 {
		t.Errorf("plan %+v", plan)
	}
}

func TestPlanStepsDuplicateText(t *testing.T) {
	wanted := []Step{{"Check", true}, {"Check", true}}
	existing := []CardStep{{"S1", "Check", false}, {"S2", "Check", false}}
	plan := PlanSteps(wanted, existing)
	if !reflect.DeepEqual(plan.Tick, []string{"S1", "S2"}) || len(plan.Add) != 0 || len(plan.Extra) != 0 {
		t.Errorf("plan %+v", plan)
	}
}

func TestPlanStepsDoesNotMutateInput(t *testing.T) {
	existing := []CardStep{{"S1", "A", false}, {"S2", "B", false}}
	PlanSteps([]Step{{"A", false}}, existing)
	if existing[0].ID != "S1" || existing[1].ID != "S2" {
		t.Errorf("input changed: %v", existing)
	}
}

func TestRecordFieldsReplacesKeys(t *testing.T) {
	p := writeFile(t, t.TempDir(), "x.md", "---\nboard: B\nCard: 5\nnote: keep me\n---\n# T\n\nBody.\n")
	if err := RecordFields(p, KV{"synced", "2026-10-17T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordFields(p, KV{"synced", "2026-10-18T11:30:00Z"}, KV{"card", "9"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	want := "---\nboard: B\nnote: keep me\nsynced: 2026-10-18T11:30:00Z\ncard: 9\n---\n# T\n\nBody.\n"
	if string(raw) != want {
		t.Fatalf("got %q, want %q", raw, want)
	}
	d, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Synced != "2026-10-18T11:30:00Z" || d.Card != 9 {
		t.Fatalf("synced %q card %d", d.Synced, d.Card)
	}
}

func TestRecordFieldsNeedsFrontMatter(t *testing.T) {
	p := writeFile(t, t.TempDir(), "x.md", "# T\n")
	if err := RecordFields(p, KV{"card", "1"}); !errors.Is(err, ErrDraft) {
		t.Fatalf("got %v", err)
	}
}
