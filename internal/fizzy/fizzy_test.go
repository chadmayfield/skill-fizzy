package fizzy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/config"
)

type call struct {
	Method, Path, Query string
	Body                []byte
}

type stub struct {
	mu    sync.Mutex
	calls []call
	h     func(w http.ResponseWriter, r *http.Request, body []byte)
}

func newFizzy(t *testing.T, dryRun bool, h func(w http.ResponseWriter, r *http.Request, body []byte)) (*Fizzy, *stub, *bytes.Buffer, *httptest.Server) {
	t.Helper()
	s := &stub{h: h}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.calls = append(s.calls, call{r.Method, r.URL.Path, r.URL.RawQuery, b})
		s.mu.Unlock()
		s.h(w, r, b)
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T", LogPath: filepath.Join(t.TempDir(), "log")}
	out := &bytes.Buffer{}
	return &Fizzy{C: client.New(cfg, dryRun, out)}, s, out, srv
}

// unHTML undoes encoding/json's HTML escaping so bodies compare readably.
func unHTML(b []byte) string {
	return strings.NewReplacer("\\u003c", "<", "\\u003e", ">", "\\u0026", "&").Replace(string(b))
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestCreateBoardThenPostpone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method == "POST" {
			w.Header().Set("Location", "/1/boards/B1.json")
			w.WriteHeader(201)
			return
		}
		jsonOK(w, map[string]any{})
	})
	id, err := f.CreateBoard("Home", 365)
	if err != nil || id != "B1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(s.calls) != 2 {
		t.Fatalf("calls=%d", len(s.calls))
	}
	if string(s.calls[0].Body) != `{"board":{"name":"Home"}}` {
		t.Errorf("first body %s", s.calls[0].Body)
	}
	if s.calls[1].Method != "PUT" || s.calls[1].Path != "/1/boards/B1" ||
		string(s.calls[1].Body) != `{"board":{"auto_postpone_period_in_days":365}}` {
		t.Errorf("second call %+v", s.calls[1])
	}
}

func TestCreateBoardBadPostpone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, nil)
	if _, err := f.CreateBoard("Home", 5); err == nil {
		t.Error("expected error")
	}
	if len(s.calls) != 0 {
		t.Error("request sent")
	}
}

func TestCreateBoardNoPostpone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/boards/B1.json")
		w.WriteHeader(201)
	})
	if _, err := f.CreateBoard("Home", 0); err != nil {
		t.Fatal(err)
	}
	if len(s.calls) != 1 {
		t.Errorf("calls=%d", len(s.calls))
	}
}

func TestAddTagIdempotent(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		jsonOK(w, map[string]any{"tags": []string{"buy"}})
	})
	added, err := f.AddTag(5, "#Buy")
	if err != nil || added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if len(s.calls) != 1 || s.calls[0].Method != "GET" {
		t.Errorf("calls %+v", s.calls)
	}
}

func TestAddAndRemoveTagToggle(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method == "GET" {
			jsonOK(w, map[string]any{"tags": []string{"buy"}})
			return
		}
		w.WriteHeader(204)
	})
	added, err := f.AddTag(5, "#Sell")
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if string(s.calls[1].Body) != `{"tag_title":"sell"}` || s.calls[1].Path != "/1/cards/5/taggings" {
		t.Errorf("toggle %+v", s.calls[1])
	}
	if rm, _ := f.RemoveTag(5, "gone"); rm {
		t.Error("removed a tag that is absent")
	}
	if rm, err := f.RemoveTag(5, "#BUY"); err != nil || !rm {
		t.Errorf("rm=%v err=%v", rm, err)
	}
}

func TestMoveToNamedColumn(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		switch r.URL.Path {
		case "/1/cards/5":
			jsonOK(w, map[string]any{"board": map[string]any{"id": "B1"}})
		case "/1/boards/B1/columns":
			jsonOK(w, []map[string]any{{"id": "C1", "name": "Now"}})
		default:
			w.WriteHeader(204)
		}
	})
	if err := f.MoveCard(5, "now", ""); err != nil {
		t.Fatal(err)
	}
	last := s.calls[len(s.calls)-1]
	if last.Method != "POST" || last.Path != "/1/cards/5/triage" || string(last.Body) != `{"column_id":"C1"}` {
		t.Errorf("last %+v", last)
	}
}

func TestMoveSpecialTargets(t *testing.T) {
	cases := []struct{ target, method, path string }{
		{"maybe", "DELETE", "/1/cards/5/triage"},
		{"Maybe?", "DELETE", "/1/cards/5/triage"},
		{"not-now", "POST", "/1/cards/5/not_now"},
		{"Not now", "POST", "/1/cards/5/not_now"},
		{"done", "POST", "/1/cards/5/closure"},
	}
	for _, c := range cases {
		f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(204) })
		if err := f.MoveCard(5, c.target, ""); err != nil {
			t.Fatal(err)
		}
		if len(s.calls) != 1 || s.calls[0].Method != c.method || s.calls[0].Path != c.path {
			t.Errorf("%s: %+v", c.target, s.calls)
		}
	}
}

func TestMoveUnknownColumn(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		jsonOK(w, []map[string]any{{"id": "C1", "name": "Now"}})
	})
	err := f.MoveCard(5, "later", "B1")
	if err == nil || !strings.Contains(err.Error(), "no column") || !strings.Contains(err.Error(), "Now") {
		t.Errorf("err=%v", err)
	}
}

func TestCardsQueryEncoding(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { jsonOK(w, []any{}) })
	_, err := f.Cards("B1", []Param{{"terms[]", "first aid"}, {"terms[]", "a&b"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "board_ids%5B%5D=B1&terms%5B%5D=first+aid&terms%5B%5D=a%26b"
	if s.calls[0].Path != "/1/cards" || s.calls[0].Query != want {
		t.Errorf("got %s?%s", s.calls[0].Path, s.calls[0].Query)
	}
}

func TestCardsOrderFollowsCaller(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { jsonOK(w, []any{}) })
	f.Cards("", []Param{{"indexed_by", "closed"}, {"terms[]", "x"}, {"a", "b"}})
	if s.calls[0].Query != "indexed_by=closed&terms%5B%5D=x&a=b" {
		t.Errorf("query %s", s.calls[0].Query)
	}
	f.Cards("", nil)
	if s.calls[1].Query != "" {
		t.Errorf("query %q", s.calls[1].Query)
	}
}

func TestCardsFollowsPages(t *testing.T) {
	var srvURL string
	f, s, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Query().Get("page") == "2" {
			jsonOK(w, []map[string]any{{"number": 2, "title": "two"}})
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/cards?page=2>; rel="next"`, srvURL))
		jsonOK(w, []map[string]any{{"number": 1, "title": "one"}})
	})
	srvURL = srv.URL
	cards, err := f.Cards("", nil)
	if err != nil || len(cards) != 2 || cards[1].Title != "two" || len(s.calls) != 2 {
		t.Fatalf("cards=%+v err=%v calls=%d", cards, err, len(s.calls))
	}
}

func TestColumnCardsFollowsPages(t *testing.T) {
	var srvURL string
	f, s, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Query().Get("page") == "2" {
			jsonOK(w, []map[string]any{{"number": 2}})
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/boards/B1/columns/C1/cards?page=2>; rel="next"`, srvURL))
		jsonOK(w, []map[string]any{{"number": 1}})
	})
	srvURL = srv.URL
	cards, err := f.ColumnCards("B1", "C1")
	if err != nil || len(cards) != 2 || cards[0].Number != 1 || cards[1].Number != 2 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	if s.calls[0].Path != "/1/boards/B1/columns/C1/cards" {
		t.Errorf("path %s", s.calls[0].Path)
	}
}

func TestBoardLookup(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		jsonOK(w, []map[string]any{{"id": "B1", "name": "Home"}, {"id": "B2", "name": "Shop"}, {"id": "B3", "name": "shop"}})
	})
	if b, err := f.Board("B2"); err != nil || b.Name != "Shop" {
		t.Errorf("by id: %+v %v", b, err)
	}
	if b, err := f.Board("HOME"); err != nil || b.ID != "B1" {
		t.Errorf("by name: %+v %v", b, err)
	}
	if _, err := f.Board("shop"); err == nil || !strings.Contains(err.Error(), "2 boards are named") {
		t.Errorf("ambiguous: %v", err)
	}
	_, err := f.Board("zzz")
	if err == nil || !strings.Contains(err.Error(), "no board named") || !strings.Contains(err.Error(), "Home, Shop, shop") {
		t.Errorf("missing: %v", err)
	}
}

func TestColumnLookup(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		jsonOK(w, []map[string]any{{"id": "C1", "name": "Now"}})
	})
	if c, err := f.Column("B1", "now"); err != nil || c.ID != "C1" {
		t.Errorf("%+v %v", c, err)
	}
	if c, err := f.Column("B1", "C1"); err != nil || c.Name != "Now" {
		t.Errorf("%+v %v", c, err)
	}
}

func TestCardDecodes(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Write([]byte(`{"number":5,"title":"T","closed":true,"golden":true,"tags":["a"],"url":"u",
"description":"d","description_html":"<p>d</p>","last_active_at":"2026-10-01T00:00:00Z",
"board":{"id":"B1","name":"Home"},"column":{"id":"C1","name":"Now","color":{"name":"Red","value":"var(--color-card-1)"}},
"steps":[{"id":"S1","content":"c","completed":true}],"surprise":1}`))
	})
	c, err := f.Card(5)
	if err != nil {
		t.Fatal(err)
	}
	if c.Number != 5 || !c.Closed || !c.Golden || c.Board.ID != "B1" || c.Column == nil || c.Column.Name != "Now" ||
		len(c.Steps) != 1 || c.Steps[0].ID != "S1" || !c.Steps[0].Completed || c.LastActiveAt == "" ||
		c.DescriptionHTML != "<p>d</p>" || c.Description != "d" || c.URL != "u" || len(c.Tags) != 1 {
		t.Errorf("%+v", c)
	}
	f2, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.Write([]byte(`{"number":6}`)) })
	if c, _ := f2.Card(6); c.Column != nil {
		t.Error("column should be nil when absent")
	}
}

func TestCreateCardBodyAndDryRun(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/cards/42.json")
		w.WriteHeader(201)
	})
	n, err := f.CreateCard("B1", "T", "<p>d</p>", "published")
	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if unHTML(s.calls[0].Body) != `{"card":{"title":"T","description":"<p>d</p>","status":"published"}}` {
		t.Errorf("body %s", s.calls[0].Body)
	}
	f2, s2, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/cards/43.json")
		w.WriteHeader(201)
	})
	f2.CreateCard("B1", "T", "", "")
	if string(s2.calls[0].Body) != `{"card":{"title":"T"}}` {
		t.Errorf("minimal body %s", s2.calls[0].Body)
	}

	// Dry run lines must match Python's json.dumps byte for byte.
	fd, sd, out, srv := newFizzy(t, true, nil)
	n, err = fd.CreateCard("B1", "T", "<p>d</p>", "published")
	if err != nil || n != 0 || len(sd.calls) != 0 {
		t.Fatalf("n=%d err=%v calls=%d", n, err, len(sd.calls))
	}
	want := "[dry-run] POST " + srv.URL + `/1/boards/B1/cards {"card": {"title": "T", "description": "<p>d</p>", "status": "published"}}` + "\n"
	if out.String() != want {
		t.Errorf("got  %q\nwant %q", out.String(), want)
	}
}

func TestUpdateCard(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(204) })
	if err := f.UpdateCard(5, CardFields{}); !IsRefusal(err) || err.Error() != "nothing to update" {
		t.Errorf("err=%v", err)
	}
	if len(s.calls) != 0 {
		t.Error("request sent")
	}
	title, empty := "New", ""
	if err := f.UpdateCard(5, CardFields{Title: &title, Description: &empty}); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Method != "PUT" || s.calls[0].Path != "/1/cards/5" ||
		string(s.calls[0].Body) != `{"card":{"title":"New","description":""}}` {
		t.Errorf("%+v", s.calls[0])
	}
}

func TestCreateColumnDryRun(t *testing.T) {
	fd, _, out, srv := newFizzy(t, true, nil)
	if _, err := fd.CreateColumn("B1", "N", "Yellow"); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.CreateColumn("B1", "N", "var(--color-card-6)"); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.CreateColumn("B1", "N", ""); err != nil {
		t.Fatal(err)
	}
	p := "[dry-run] POST " + srv.URL + "/1/boards/B1/columns "
	want := p + `{"column": {"name": "N", "color": "var(--color-card-3)"}}` + "\n" +
		p + `{"column": {"name": "N", "color": "var(--color-card-6)"}}` + "\n" +
		p + `{"column": {"name": "N"}}` + "\n"
	if out.String() != want {
		t.Errorf("got\n%swant\n%s", out.String(), want)
	}
}

func TestStepDryRunLines(t *testing.T) {
	fd, _, out, srv := newFizzy(t, true, nil)
	c, d := "c", true
	if _, err := fd.AddStep(5, "c", true); err != nil {
		t.Fatal(err)
	}
	if err := fd.UpdateStep(5, "S1", &c, &d); err != nil {
		t.Fatal(err)
	}
	f := false
	if err := fd.UpdateStep(5, "S1", nil, &f); err != nil {
		t.Fatal(err)
	}
	p := "[dry-run] "
	want := p + "POST " + srv.URL + `/1/cards/5/steps {"step": {"content": "c", "completed": true}}` + "\n" +
		p + "PUT " + srv.URL + `/1/cards/5/steps/S1 {"step": {"content": "c", "completed": true}}` + "\n" +
		p + "PUT " + srv.URL + `/1/cards/5/steps/S1 {"step": {"completed": false}}` + "\n"
	if out.String() != want {
		t.Errorf("got\n%swant\n%s", out.String(), want)
	}
}

func TestAddStep(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Location", "/1/cards/5/steps/S9.json")
		w.WriteHeader(201)
	})
	id, err := f.AddStep(5, "do it", false)
	if err != nil || id != "S9" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if string(s.calls[0].Body) != `{"step":{"content":"do it","completed":false}}` {
		t.Errorf("body %s", s.calls[0].Body)
	}
}

func TestUpdateStepSendsOnlyGivenFields(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(204) })
	done := true
	text := "new"
	if err := f.UpdateStep(5, "S1", nil, &done); err != nil {
		t.Fatal(err)
	}
	if err := f.UpdateStep(5, "S1", &text, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.UpdateStep(5, "S1", &text, &done); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{`{"step":{"completed":true}}`, `{"step":{"content":"new"}}`, `{"step":{"content":"new","completed":true}}`} {
		if s.calls[i].Method != "PUT" || s.calls[i].Path != "/1/cards/5/steps/S1" || string(s.calls[i].Body) != want {
			t.Errorf("%d: %+v", i, s.calls[i])
		}
	}
	if err := f.UpdateStep(5, "S1", nil, nil); !IsRefusal(err) || err.Error() != "nothing to update" {
		t.Errorf("expected the nothing to update refusal, got %v", err)
	}
}

func TestDeleteStep(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(204) })
	if err := f.DeleteStep(5, "S1"); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Method != "DELETE" || s.calls[0].Path != "/1/cards/5/steps/S1" {
		t.Errorf("%+v", s.calls[0])
	}
}

func TestSimpleCardWrites(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(204) })
	f.ReopenCard(5)
	f.MoveCardToBoard(5, "B2")
	f.SetGolden(5, true)
	f.SetGolden(5, false)
	f.DeleteCard(5)
	want := []struct{ m, p, b string }{
		{"DELETE", "/1/cards/5/closure", ""},
		{"PUT", "/1/cards/5/board", `{"board_id":"B2"}`},
		{"POST", "/1/cards/5/goldness", ""},
		{"DELETE", "/1/cards/5/goldness", ""},
		{"DELETE", "/1/cards/5", ""},
	}
	for i, w := range want {
		c := s.calls[i]
		if c.Method != w.m || c.Path != w.p || string(c.Body) != w.b {
			t.Errorf("%d: %+v", i, c)
		}
	}
}

func TestAddCommentAndTags(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method == "POST" {
			w.Header().Set("Location", "/1/cards/5/comments/K1.json")
			w.WriteHeader(201)
			return
		}
		jsonOK(w, []map[string]any{{"id": "T1", "title": "buy"}})
	})
	id, err := f.AddComment(5, "<p>hi</p>")
	if err != nil || id != "K1" || unHTML(s.calls[0].Body) != `{"comment":{"body":"<p>hi</p>"}}` {
		t.Errorf("id=%q err=%v body=%s", id, err, s.calls[0].Body)
	}
	tags, err := f.Tags()
	if err != nil || len(tags) != 1 || tags[0].Title != "buy" {
		t.Errorf("%+v %v", tags, err)
	}
}

func TestUploadPrefersAttachableSgid(t *testing.T) {
	var f *Fizzy
	var s *stub
	var srvURL string
	info := func() map[string]any {
		return map[string]any{"signed_id": "BLOB", "attachable_sgid": "ATTACH",
			"direct_upload": map[string]any{"url": srvURL + "/put", "headers": map[string]string{"Content-Type": "image/png"}}}
	}
	f, s, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, info())
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("storage PUT carried the token")
		}
		if r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("content-type %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(204)
	})
	srvURL = srv.URL
	p := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(p, []byte("png"), 0o644)
	sgid, err := f.Upload(p)
	if err != nil || sgid != "ATTACH" {
		t.Fatalf("sgid=%q err=%v", sgid, err)
	}
	if len(s.calls) != 2 || s.calls[1].Method != "PUT" || s.calls[1].Path != "/put" || string(s.calls[1].Body) != "png" {
		t.Errorf("calls %+v", s.calls)
	}
	var blob struct {
		Blob map[string]any `json:"blob"`
	}
	json.Unmarshal(s.calls[0].Body, &blob)
	// md5("png") base64
	if blob.Blob["filename"] != "a.png" || blob.Blob["byte_size"] != float64(3) || blob.Blob["content_type"] != "image/png" ||
		blob.Blob["checksum"] != "v/E5+gWsWD9oWlI6s9EQoA==" {
		t.Errorf("blob %+v", blob.Blob)
	}
	if !strings.HasPrefix(string(s.calls[0].Body), `{"blob":{"filename":"a.png","byte_size":3,"checksum":`) {
		t.Errorf("blob key order: %s", s.calls[0].Body)
	}
}

func TestUploadFallsBackToSignedID(t *testing.T) {
	var srvURL string
	f, _, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, map[string]any{"signed_id": "BLOB", "direct_upload": map[string]any{"url": srvURL + "/put"}})
			return
		}
		w.WriteHeader(204)
	})
	srvURL = srv.URL
	p := filepath.Join(t.TempDir(), "blob.unknownext")
	os.WriteFile(p, []byte("x"), 0o644)
	sgid, err := f.Upload(p)
	if err != nil || sgid != "BLOB" {
		t.Errorf("sgid=%q err=%v", sgid, err)
	}
}

func TestUploadRelativeURLKeepsPathUnscoped(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, map[string]any{"signed_id": "B", "direct_upload": map[string]any{"url": "/disk/put"}})
			return
		}
		w.WriteHeader(204)
	})
	p := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	if _, err := f.Upload(p); err != nil {
		t.Fatal(err)
	}
	if s.calls[1].Path != "/disk/put" {
		t.Errorf("path %s", s.calls[1].Path)
	}
}

func TestUploadDryRun(t *testing.T) {
	f, s, out, srv := newFizzy(t, true, nil)
	p := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(p, []byte("png"), 0o644)
	sgid, err := f.Upload(p)
	if err != nil || sgid != "DRY-RUN-SGID" || len(s.calls) != 0 {
		t.Fatalf("sgid=%q err=%v calls=%d", sgid, err, len(s.calls))
	}
	want := "[dry-run] POST " + srv.URL + `/1/rails/active_storage/direct_uploads {"blob": {"filename": "a.png", "byte_size": 3, "checksum": "v/E5+gWsWD9oWlI6s9EQoA==", "content_type": "image/png"}}` + "\n"
	if out.String() != want {
		t.Errorf("got %q", out.String())
	}
}

func TestUploadMissingFile(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, nil)
	if _, err := f.Upload(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("expected error")
	}
}

// A direct-upload reply without a storage URL or an id is an error, and
// nothing more is sent: no PUT to the server root, no sgid="" to embed.
func TestUploadIncompleteReply(t *testing.T) {
	for name, reply := range map[string]map[string]any{
		"empty":     {},
		"no url":    {"signed_id": "B", "attachable_sgid": "A", "direct_upload": map[string]any{"headers": map[string]string{}}},
		"no ids":    {"direct_upload": map[string]any{"url": "/disk/put"}},
		"empty ids": {"signed_id": "", "attachable_sgid": "", "direct_upload": map[string]any{"url": "/disk/put"}},
		"empty url": {"signed_id": "B", "direct_upload": map[string]any{"url": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
				if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
					jsonOK(w, reply)
					return
				}
				w.WriteHeader(204)
			})
			p := filepath.Join(t.TempDir(), "a.png")
			os.WriteFile(p, []byte("png"), 0o644)
			sgid, err := f.Upload(p)
			if err == nil || sgid != "" {
				t.Fatalf("sgid=%q err=%v", sgid, err)
			}
			if !strings.Contains(err.Error(), "direct upload reply") {
				t.Errorf("err = %v", err)
			}
			if len(s.calls) != 1 {
				t.Errorf("calls %+v", s.calls)
			}
		})
	}
}
