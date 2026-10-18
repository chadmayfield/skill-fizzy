package fizzy

import (
	"io"
	"net/http"
	"testing"
)

// CloseCard posts the closure without reading the board's columns, so a
// custom column named Done does not stand in its way. A card that is
// already closed gets no POST.
func TestCloseCard(t *testing.T) {
	for _, tc := range []struct {
		card      string
		changed   bool
		wantWrite bool
	}{
		{`{"number":87,"closed":false,"board":{"id":"b1","name":"Home"}}`, true, true},
		{`{"number":87,"closed":true,"board":{"id":"b1","name":"Home"}}`, false, false},
	} {
		f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
			if r.Method != http.MethodGet {
				w.WriteHeader(204)
				return
			}
			io.WriteString(w, tc.card)
		})
		changed, err := f.CloseCard(87)
		if err != nil || changed != tc.changed {
			t.Fatalf("%s: changed=%v err=%v", tc.card, changed, err)
		}
		var got []string
		for _, c := range s.calls {
			got = append(got, c.Method+" "+c.Path)
		}
		want := []string{"GET /1/cards/87"}
		if tc.wantWrite {
			want = append(want, "POST /1/cards/87/closure")
		}
		if len(got) != len(want) || got[0] != want[0] || len(got) == 2 && got[1] != want[1] {
			t.Errorf("%s: sent %q, want %q", tc.card, got, want)
		}
	}
}
