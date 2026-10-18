package fizzy

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetBoardDecodesDetail(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		jsonOK(w, map[string]any{
			"id": "b1", "name": "Home", "all_access": false, "auto_postpone_period_in_days": 30,
			"public_url": "https://fizzy.example.com/1/public/boards/xyz", "user_ids": []string{"u1", "u2"},
			"creator": map[string]any{"name": "Pat"},
		})
	})
	b, err := f.BoardDetail("b1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Home" || b.AllAccess || b.PublicURL == "" || len(b.UserIDs) != 2 || b.Creator.Name != "Pat" || *b.AutoPostponePeriodInDays != 30 {
		t.Errorf("got %+v", b)
	}
	if !strings.Contains(string(b.Raw()), `"public_url"`) {
		t.Errorf("raw %s", b.Raw())
	}
}

func TestUpdateBoardSendsOnlySetFields(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) { jsonOK(w, map[string]any{}) })
	name, days := "Garden", 90
	if err := f.UpdateBoard("b1", BoardFields{Name: &name, PostponeDays: &days}); err != nil {
		t.Fatal(err)
	}
	c := s.calls[0]
	if c.Method != "PUT" || c.Path != "/1/boards/b1" || strings.TrimSpace(string(c.Body)) != `{"board":{"name":"Garden","auto_postpone_period_in_days":90}}` {
		t.Errorf("got %s %s %s", c.Method, c.Path, c.Body)
	}
	if err := f.UpdateBoard("b1", BoardFields{}); err == nil {
		t.Error("empty update should fail")
	}
	bad := 5
	if err := f.UpdateBoard("b1", BoardFields{PostponeDays: &bad}); err == nil {
		t.Error("5 days should fail")
	}
}

func TestSetBoardAccess(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) { jsonOK(w, map[string]any{}) })
	if err := f.SetBoardAccess("b1", true, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.SetBoardAccess("b1", false, []string{"u1", "u2"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(s.calls[0].Body)); got != `{"board":{"all_access":true}}` {
		t.Errorf("all: %s", got)
	}
	if got := strings.TrimSpace(string(s.calls[1].Body)); got != `{"board":{"all_access":false,"user_ids":["u1","u2"]}}` {
		t.Errorf("users: %s", got)
	}
}

func TestPublishAndUnpublishBoard(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.Method == "POST" {
			w.WriteHeader(201)
			jsonOK(w, map[string]any{"public_url": "https://fizzy.example.com/1/public/boards/xyz"})
			return
		}
		w.WriteHeader(204)
	})
	url, err := f.PublishBoard("b1")
	if err != nil || url != "https://fizzy.example.com/1/public/boards/xyz" {
		t.Fatalf("got %q, %v", url, err)
	}
	if err := f.UnpublishBoard("b1"); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Path != "/1/boards/b1/publication" || s.calls[1].Method != "DELETE" || s.calls[1].Path != "/1/boards/b1/publication" {
		t.Errorf("calls %+v", s.calls)
	}
}

func TestDeleteBoard(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) { w.WriteHeader(204) })
	if err := f.DeleteBoard("b1"); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Method != "DELETE" || s.calls[0].Path != "/1/boards/b1" {
		t.Errorf("calls %+v", s.calls)
	}
}

func TestBoardAccessesFollowsPages(t *testing.T) {
	f, s, _, srv := newFizzy(t, false, nil)
	s.h = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<`+srv.URL+`/1/boards/b1/accesses?page=2>; rel="next"`)
			jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []map[string]any{{"id": "u1", "name": "Pat", "has_access": true, "involvement": "watching"}}})
			return
		}
		jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []map[string]any{{"id": "u2", "name": "Sam", "has_access": false, "involvement": nil}}})
	}
	a, err := f.BoardAccesses("b1")
	if err != nil {
		t.Fatal(err)
	}
	if a.AllAccess || len(a.Users) != 2 || a.Users[0].Involvement != "watching" || a.Users[1].HasAccess || a.Users[1].Involvement != "" {
		t.Errorf("got %+v", a)
	}
	if !strings.Contains(string(a.Users[1].Raw()), `"Sam"`) {
		t.Errorf("raw %s", a.Users[1].Raw())
	}
}

func TestColumnOps(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		jsonOK(w, map[string]any{"id": "c1", "name": "Next", "color": map[string]any{"name": "Lime", "value": "var(--color-card-4)"}})
	})
	col, err := f.GetColumn("b1", "c1")
	if err != nil || col.Color.Name != "Lime" || col.Raw() == nil {
		t.Fatalf("got %+v, %v", col, err)
	}
	name, color := "Soon", "4"
	if err := f.UpdateColumn("b1", "c1", ColumnFields{Name: &name, Color: &color}); err != nil {
		t.Fatal(err)
	}
	if err := f.UpdateColumn("b1", "c1", ColumnFields{}); err == nil {
		t.Error("empty column update should fail")
	}
	if err := f.DeleteColumn("b1", "c1"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(s.calls[1].Body)); s.calls[1].Method != "PUT" || s.calls[1].Path != "/1/boards/b1/columns/c1" || got != `{"column":{"name":"Soon","color":"var(--color-card-4)"}}` {
		t.Errorf("update %s %s %s", s.calls[1].Method, s.calls[1].Path, got)
	}
	if s.calls[2].Method != "DELETE" || s.calls[2].Path != "/1/boards/b1/columns/c1" {
		t.Errorf("delete %+v", s.calls[2])
	}
}

func TestColumnColorValue(t *testing.T) {
	for in, want := range map[string]string{"": "", "4": "var(--color-card-4)", "var(--color-card-1)": "var(--color-card-1)"} {
		if got := ColumnColorValue(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
