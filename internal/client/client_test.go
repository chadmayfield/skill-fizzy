package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chadmayfield/skill-fizzy/internal/config"
)

func newTestClient(t *testing.T, srv *httptest.Server, dryRun bool) (*Client, *bytes.Buffer, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "state", "log")
	cfg := &config.Config{URL: srv.URL, Account: "1", Token: "T", LogPath: logPath}
	out := &bytes.Buffer{}
	return New(cfg, dryRun, out), out, logPath
}

func TestNextLink(t *testing.T) {
	if got := NextLink(`<https://x/1/cards?page=2>; rel="next"`); got != "https://x/1/cards?page=2" {
		t.Errorf("got %q", got)
	}
	if got := NextLink(`<https://x/1/cards?page=1>; rel="prev"`); got != "" {
		t.Errorf("prev: got %q", got)
	}
	if got := NextLink(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

func TestIDFromLocation(t *testing.T) {
	cases := map[string]string{
		"/1/boards/03abc.json":        "03abc",
		"https://fz.test/1/cards/87":  "87",
		"https://fz.test/1/cards/87/": "87",
		"":                            "",
	}
	for in, want := range cases {
		if got := IDFromLocation(in); got != want {
			t.Errorf("IDFromLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetAllFollowsPages(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"title":"b"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=2>; rel="next"`, srv.URL))
		fmt.Fprint(w, `[{"title":"a"}]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	items, err := c.GetAll("/tags")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || string(items[0]) != `{"title":"a"}` || string(items[1]) != `{"title":"b"}` {
		t.Errorf("items = %s", items)
	}
}

func TestDryRunSendsNoWrites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
	}))
	defer srv.Close()
	c, out, logPath := newTestClient(t, srv, true)
	id, err := c.Create("/boards", map[string]any{"name": "x"})
	if err != nil || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	want := "[dry-run] POST " + srv.URL + "/1/boards {\"name\": \"x\"}\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Error("dry run wrote the action log")
	}
}

func TestDryRunFormats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request")
	}))
	defer srv.Close()
	c, out, _ := newTestClient(t, srv, true)

	type body struct {
		A int      `json:"a"`
		B []string `json:"b"`
		C string   `json:"c"`
	}
	if _, err := c.Write("PUT", "/x", body{1, []string{"p", "q"}, "café <&> \U0001F600"}); err != nil {
		t.Fatal(err)
	}
	want := `[dry-run] PUT ` + srv.URL + `/1/x {"a": 1, "b": ["p", "q"], "c": "caf\u00e9 <&> \ud83d\ude00"}` + "\n"
	if out.String() != want {
		t.Errorf("got  %q\nwant %q", out.String(), want)
	}

	out.Reset()
	if _, _, _, err := c.Do("PUT", "/up", nil, Raw([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	if want := "[dry-run] PUT " + srv.URL + "/1/up <5 bytes>\n"; out.String() != want {
		t.Errorf("raw: got %q want %q", out.String(), want)
	}

	out.Reset()
	if _, _, _, err := c.Do("DELETE", "/x", nil); err != nil {
		t.Fatal(err)
	}
	if want := "[dry-run] DELETE " + srv.URL + "/1/x \n"; out.String() != want {
		t.Errorf("empty: got %q want %q", out.String(), want)
	}

	out.Reset()
	if _, err := c.Write("POST", "/x", map[string]string{"k": strings.Repeat("z", 500)}); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSuffix(out.String(), "\n")
	prefix := "[dry-run] POST " + srv.URL + "/1/x "
	if got := len(strings.TrimPrefix(line, prefix)); got != 400 {
		t.Errorf("body length = %d, want 400", got)
	}
}

func TestDryRunStillSendsGets(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, true)
	var v map[string]bool
	if err := c.Get("/ping", &v); err != nil || !v["ok"] || hits != 1 {
		t.Errorf("err=%v v=%v hits=%d", err, v, hits)
	}
}

func TestBearerHeader(t *testing.T) {
	var gotAuth, gotAccept, gotUA, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAccept, gotUA, gotCT = r.Header.Get("Authorization"), r.Header.Get("Accept"), r.Header.Get("User-Agent"), r.Header.Get("Content-Type")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	if _, err := c.Write("POST", "/x", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer T" || gotAccept != "application/json" || gotUA != "fizzy-skill" || gotCT != "application/json" {
		t.Errorf("auth=%q accept=%q ua=%q ct=%q", gotAuth, gotAccept, gotUA, gotCT)
	}
}

func TestOptions(t *testing.T) {
	var gotAuth, gotPath, gotHdr, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotHdr = r.Header.Get("Authorization"), r.URL.Path, r.Header.Get("X-Extra")
		b := new(bytes.Buffer)
		b.ReadFrom(r.Body)
		gotBody = b.String()
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	_, _, _, err := c.Do("PUT", "/up", nil, Raw([]byte("rawdata")), NoAuth(), NotAccountScoped(), Headers(map[string]string{"X-Extra": "v"}))
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" || gotPath != "/up" || gotHdr != "v" || gotBody != "rawdata" {
		t.Errorf("auth=%q path=%q hdr=%q body=%q", gotAuth, gotPath, gotHdr, gotBody)
	}
}

func TestActionLogHasNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/1/boards/03abc.json")
		w.WriteHeader(201)
	}))
	defer srv.Close()
	c, _, logPath := newTestClient(t, srv, false)
	id, err := c.Create("/boards", map[string]string{"name": "x"})
	if err != nil || id != "03abc" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if err := c.Get("/boards", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 log line (writes only), got %d: %q", len(lines), data)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["method"] != "POST" || rec["path"] != "/1/boards" || rec["status"] != float64(201) || rec["t"] == nil {
		t.Errorf("rec = %v", rec)
	}
	if strings.Contains(string(data), "Bearer") {
		t.Error("token in log")
	}
	re := `{"t": "`
	if !strings.HasPrefix(lines[0], re) || !strings.HasSuffix(lines[0], `Z", "method": "POST", "path": "/1/boards", "status": 201}`) {
		t.Errorf("log format = %q", lines[0])
	}
}

func TestAPIErrorText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		fmt.Fprint(w, `{"error":"x"}`)
	}))
	defer srv.Close()
	c, _, logPath := newTestClient(t, srv, false)
	_, err := c.Write("POST", "/boards", map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("want error")
	}
	want := `POST /1/boards -> HTTP 422: {"error":"x"}`
	if err.Error() != want {
		t.Errorf("got %q want %q", err.Error(), want)
	}
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 422 {
		t.Errorf("not APIError: %v", err)
	}
	if data, _ := os.ReadFile(logPath); !strings.Contains(string(data), `"status": 422`) {
		t.Errorf("failed write not logged: %q", data)
	}
}

func TestAPIErrorTruncatesAndOmitsEmptyBody(t *testing.T) {
	e := &APIError{Status: 500, Method: "GET", Path: "/p", Body: strings.Repeat("a", 400)}
	if want := "GET /p -> HTTP 500: " + strings.Repeat("a", 300); e.Error() != want {
		t.Errorf("got %q", e.Error())
	}
	e = &APIError{Status: 404, Method: "GET", Path: "/p", Body: "  \n"}
	if e.Error() != "GET /p -> HTTP 404" {
		t.Errorf("got %q", e.Error())
	}
}

func TestAbsoluteURLPassesThrough(t *testing.T) {
	cfg := &config.Config{URL: "https://fz.example.com", Account: "1"}
	c := New(cfg, false, &bytes.Buffer{})
	if got := c.URL("https://other.example/put", true); got != "https://other.example/put" {
		t.Errorf("got %q", got)
	}
	if got := c.URL("/cards", true); got != "https://fz.example.com/1/cards" {
		t.Errorf("got %q", got)
	}
	if got := c.URL("/my/identity", false); got != "https://fz.example.com/my/identity" {
		t.Errorf("got %q", got)
	}
}

func TestTokenResolvedLazily(t *testing.T) {
	cfg := &config.Config{URL: "https://fz.example.com", Account: "1", TokenCmd: "false"}
	c := New(cfg, false, &bytes.Buffer{})
	_ = c.URL("/x", true)
	if _, _, _, err := c.Do("GET", "/x", nil); err == nil || !errors.Is(err, config.ErrConfig) {
		t.Errorf("want config error on first authenticated request, got %v", err)
	}
}
