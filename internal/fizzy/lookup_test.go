package fizzy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A column name that two columns share ignoring case, with no exact match,
// is a Refusal, as an ambiguous user is. An exact name or an id wins.
func TestColumnLookupAmbiguous(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method != http.MethodGet {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `[{"id":"c1","name":"Doing"},{"id":"c2","name":"doing"},{"id":"Next","name":"Later"},{"id":"c4","name":"Next"}]`)
	})
	if _, err := f.Column("b1", "DOING"); !IsRefusal(err) || err.Error() != "2 columns are named 'DOING'; use the column id" {
		t.Errorf("ambiguous: %v", err)
	}
	for in, want := range map[string]string{"doing": "c2", "Doing": "c1", "c2": "c2", "Next": "Next", "later": "Next"} {
		if c, err := f.Column("b1", in); err != nil || c.ID != want {
			t.Errorf("Column(%q) = %+v %v, want %s", in, c, err, want)
		}
	}
	s.calls = nil
	if err := f.MoveCard(87, "DOING", "b1"); !IsRefusal(err) {
		t.Errorf("move: %v", err)
	}
	for _, c := range s.calls {
		if c.Method != http.MethodGet {
			t.Errorf("sent %s %s", c.Method, c.Path)
		}
	}
}

func TestFindTagAmbiguous(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		io.WriteString(w, `[{"id":"t1","title":"Gate"},{"id":"t2","title":"gate"},{"id":"t3","title":"hinge"}]`)
	})
	if _, err := f.FindTag("GATE"); !IsRefusal(err) || err.Error() != "2 tags are named 'GATE'; use the exact title" {
		t.Errorf("ambiguous: %v", err)
	}
	for in, want := range map[string]string{"gate": "t2", "#Gate": "t1", "HINGE": "t3"} {
		if tg, err := f.FindTag(in); err != nil || tg.ID != want {
			t.Errorf("FindTag(%q) = %+v %v, want %s", in, tg, err, want)
		}
	}
}

// A card reply without a tags key says nothing about the card's tags, so the
// toggle could remove a tag the card has. fz refuses, as card assign does.
// An empty tag title is refused before anything is read.
func TestTagToggleRefusals(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method != http.MethodGet {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `{"number":87}`)
	})
	for _, op := range []func(int, string) (bool, error){f.AddTag, f.RemoveTag} {
		changed, err := op(87, "hinge")
		if !IsRefusal(err) || changed || err.Error() != "card #87 has no tags list in Fizzy's reply, so fz cannot tell whether it has the tag 'hinge'" {
			t.Errorf("no tags key: %v %v", changed, err)
		}
	}
	n := len(s.calls)
	for _, blank := range []string{"", "#", "  ", " # "} {
		if _, err := f.AddTag(87, blank); !IsRefusal(err) || err.Error() != "the tag cannot be empty" {
			t.Errorf("tag %q: %v", blank, err)
		}
	}
	if len(s.calls) != n {
		t.Errorf("an empty tag sent %d requests", len(s.calls)-n)
	}
	for _, c := range s.calls {
		if c.Method != http.MethodGet {
			t.Errorf("sent %s %s", c.Method, c.Path)
		}
	}
	if strings.Contains(TagTitle(" #Hinge "), "#") || TagTitle(" #Hinge ") != "hinge" {
		t.Errorf("TagTitle = %q", TagTitle(" #Hinge "))
	}
}
