package review

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

func card(t *testing.T, n int, title string, closed bool) fizzy.Card {
	t.Helper()
	var c fizzy.Card
	raw := fmt.Sprintf(`{"number":%d,"title":%q,"closed":%t}`, n, title, closed)
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func days(n int) *int { return &n }

func TestFormatBoard(t *testing.T) {
	r := Result{
		Board: "Garage", PostponeDays: days(90),
		Lanes: []Lane{
			{Name: "Later", Cards: []fizzy.Card{card(t, 11, "a", false)}},
			{Name: "Next"},
			{Name: "Now", Cards: []fizzy.Card{card(t, 87, "Order the hinge", false), card(t, 88, "Measure the gap", false)}},
		},
		Maybe:  []fizzy.Card{card(t, 12, "b", false), card(t, 13, "c", false)},
		Golden: []fizzy.Card{card(t, 88, "Measure the gap", false)},
	}
	want := strings.Join([]string{
		"== Garage (postpone 90d)",
		"Now (2):",
		"  #87 Order the hinge",
		"  #88 Measure the gap",
		"Next 0 · Later 1 · Maybe? 2",
		"Golden (1): #88 Measure the gap",
		"Postponing soon (0): none",
		"Stalled (0): none",
	}, "\n")
	if got := FormatBoard(r); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatBoardNoCustomColumns(t *testing.T) {
	got := FormatBoard(Result{Board: "X", PostponeDays: days(30)})
	if !strings.Contains(got, "Maybe? 0") {
		t.Errorf("missing Maybe? count:\n%s", got)
	}
	if strings.Contains(got, "(0):\n") {
		t.Errorf("no lanes, so no lane block:\n%s", got)
	}
}

func TestFormatBoardEmptyLastLane(t *testing.T) {
	got := FormatBoard(Result{Board: "X", PostponeDays: days(30), Lanes: []Lane{{Name: "Now"}}})
	if !strings.Contains(got, "Now (0):\n  none\n") {
		t.Errorf("empty last lane should print none:\n%s", got)
	}
}

func TestFormatBoardNoPostponeDays(t *testing.T) {
	if got := FormatBoard(Result{Board: "X"}); !strings.HasPrefix(got, "== X (postpone Noned)") {
		t.Errorf("got %q", got)
	}
}

func TestOpenOnly(t *testing.T) {
	got := OpenOnly([]fizzy.Card{card(t, 1, "a", false), card(t, 2, "b", true)})
	if len(got) != 1 || got[0].Number != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestGatherRequestOrder(t *testing.T) {
	var mu sync.Mutex
	var reqs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/1/boards/B1/columns":
			fmt.Fprint(w, `[{"id":"C1","name":"Next"},{"id":"C2","name":"Now"}]`)
		case "/1/boards/B1/columns/C1/cards":
			fmt.Fprint(w, `[{"number":11,"title":"a","closed":false}]`)
		case "/1/boards/B1/columns/C2/cards":
			fmt.Fprint(w, `[{"number":87,"title":"Order the hinge","closed":false},{"number":9,"title":"old","closed":true}]`)
		case "/1/cards":
			switch r.URL.Query().Get("indexed_by") {
			case "maybe":
				fmt.Fprint(w, `[{"number":12,"title":"b","closed":false}]`)
			case "golden":
				fmt.Fprint(w, `[{"number":87,"title":"Order the hinge","closed":false}]`)
			default:
				fmt.Fprint(w, `[]`)
			}
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T"}
	fz := &fizzy.Fizzy{C: client.New(cfg, false, &strings.Builder{})}

	r, err := Gather(fz, fizzy.Board{ID: "B1", Name: "Garage", AutoPostponePeriodInDays: days(90)})
	if err != nil {
		t.Fatal(err)
	}
	wantReqs := []string{
		"GET /1/boards/B1/columns",
		"GET /1/boards/B1/columns/C1/cards",
		"GET /1/boards/B1/columns/C2/cards",
		"GET /1/cards?board_ids%5B%5D=B1&indexed_by=maybe",
		"GET /1/cards?board_ids%5B%5D=B1&indexed_by=golden",
		"GET /1/cards?board_ids%5B%5D=B1&indexed_by=postponing_soon",
		"GET /1/cards?board_ids%5B%5D=B1&indexed_by=stalled",
	}
	if strings.Join(reqs, "\n") != strings.Join(wantReqs, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(reqs, "\n"), strings.Join(wantReqs, "\n"))
	}
	if len(r.Lanes) != 2 || len(r.Lanes[1].Cards) != 1 || r.Lanes[1].Cards[0].Number != 87 {
		t.Errorf("closed card should be dropped from lanes: %+v", r.Lanes)
	}
	if len(r.Maybe) != 1 || len(r.Golden) != 1 || len(r.PostponingSoon) != 0 || len(r.Stalled) != 0 {
		t.Errorf("index lists wrong: %+v", r)
	}
}
