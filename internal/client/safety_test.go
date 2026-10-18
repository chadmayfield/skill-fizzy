package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// spy is a plain http server that counts requests and records whether any
// of them carried an Authorization header.
type spy struct {
	*httptest.Server
	hits    atomic.Int32
	sawAuth atomic.Bool
}

func newSpy(t *testing.T) *spy {
	t.Helper()
	s := &spy{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			s.sawAuth.Store(true)
		}
		fmt.Fprint(w, `[{"title":"stolen"}]`)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *spy) check(t *testing.T, wantHits int32) {
	t.Helper()
	if s.sawAuth.Load() {
		t.Error("the second server saw an Authorization header")
	}
	if got := s.hits.Load(); got != wantHits {
		t.Errorf("the second server got %d requests, want %d", got, wantHits)
	}
}

// tlsClient returns a client for a TLS test server. Its transport trusts the
// test certificate and still speaks plain http.
func tlsClient(t *testing.T, srv *httptest.Server) (*Client, string) {
	t.Helper()
	c, _, logPath := newTestClient(t, srv, false)
	c.http.Transport = srv.Client().Transport
	return c, logPath
}

func TestNextLinkParsing(t *testing.T) {
	cases := map[string]string{
		`<https://x/1/cards?page=2>; rel="next"`:                     "https://x/1/cards?page=2",
		`<u>; type="json"; rel="next"`:                               "u",
		`<u>; rel="prev next"`:                                       "u",
		`<u>; REL="NEXT"`:                                            "u",
		`<u>;rel=next`:                                               "u",
		`<u> ; rel = "next" ; title="gate"`:                          "u",
		`<p>; rel="prev", <u>; rel="next"`:                           "u",
		`<a>; rel="next-archive"`:                                    "",
		`<a>; rel="next-archive", <u>; rel=next`:                     "u",
		`<u,v>; rel="next"`:                                          "u,v",
		`<a>; title="x, rel=next"; rel="prev"`:                       "",
		`<a>; title="say \"rel=next\""; rel="prev", <u>; rel="next"`: "u",
		`<a>; rel="first", <b>; rel="last"`:                          "",
		`garbage`:                                                    "",
		`<unterminated; rel="next"`:                                  "",
		``:                                                           "",
		`<https://x/1/cards?page=1>; rel="prev"`:                     "",
		`<https://x/1/a?page=3>; rel="next", <https://x/1/a?page=9>; rel="last"`: "https://x/1/a?page=3",
	}
	for in, want := range cases {
		if got := NextLink(in); got != want {
			t.Errorf("NextLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPaginationRefusesAnotherOrigin(t *testing.T) {
	other := newSpy(t)
	main := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=2>; rel="next"`, other.URL))
		fmt.Fprint(w, `[{"title":"a"}]`)
	}))
	defer main.Close()
	c, _ := tlsClient(t, main)
	items, err := c.GetAll("/tags")
	if err == nil || !strings.Contains(err.Error(), "not the configured server") {
		t.Errorf("err = %v", err)
	}
	if items != nil {
		t.Errorf("items = %s, want nil", items)
	}
	other.check(t, 0)
}

func TestPaginationRefusesSchemeAndPortChanges(t *testing.T) {
	other := newSpy(t)
	var main *httptest.Server
	main = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "" {
			t.Errorf("a next page was fetched: %s", r.URL)
		}
		u, _ := url.Parse(main.URL)
		links := map[string]string{
			"http":   "http://" + u.Host + "/1/tags?page=2",
			"port":   "https://" + u.Hostname() + ":1/1/tags?page=2",
			"spy":    other.URL + "/1/tags?page=2",
			"scheme": "//" + strings.TrimPrefix(other.URL, "http://") + "/1/tags?page=2",
			"bare":   "tags?page=2",
			"query":  "?page=2",
		}
		w.Header().Set("Link", "<"+links[r.URL.Query().Get("case")]+`>; rel="next"`)
		fmt.Fprint(w, `[]`)
	}))
	defer main.Close()
	c, _ := tlsClient(t, main)
	for _, name := range []string{"http", "port", "spy", "scheme", "bare", "query"} {
		if _, err := c.GetAll("/tags?case=" + name); err == nil {
			t.Errorf("%s: link followed", name)
		}
	}
	other.check(t, 0)
}

func TestRelativeNextLinkUsesTheOrigin(t *testing.T) {
	for _, prefix := range []string{"", "/fz"} {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.RequestURI())
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("Link", `<`+prefix+`/1/tags?page=2>; rel="next"`)
			}
			fmt.Fprint(w, `[{"title":"a"}]`)
		}))
		cfg := &config.Config{URL: srv.URL + prefix, Account: "1", Token: "T", LogPath: filepath.Join(t.TempDir(), "log")}
		c := New(cfg, false, &bytes.Buffer{})
		items, err := c.GetAll("/tags")
		srv.Close()
		if err != nil || len(items) != 2 {
			t.Errorf("prefix %q: items=%d err=%v", prefix, len(items), err)
		}
		want := []string{prefix + "/1/tags", prefix + "/1/tags?page=2"}
		if fmt.Sprint(paths) != fmt.Sprint(want) {
			t.Errorf("prefix %q: paths = %v, want %v", prefix, paths, want)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	same := [][2]string{
		{"https://fizzy.example.com", "https://fizzy.example.com:443/1/x"},
		{"HTTPS://Fizzy.Example.com", "https://fizzy.example.com/1/x"},
		{"http://localhost:3000/fz", "http://localhost:3000/other"},
		{"http://127.0.0.1", "http://127.0.0.1:80/"},
	}
	diff := [][2]string{
		{"https://fizzy.example.com", "http://fizzy.example.com"},
		{"https://fizzy.example.com", "https://fizzy.example.com:8443"},
		{"https://fizzy.example.com", "https://gate.example.com"},
		{"https://fizzy.example.com", "https://fizzy.example.com.example.com"},
		{"https://fizzy.example.com", "/1/x"},
	}
	for _, p := range same {
		a, _ := url.Parse(p[0])
		b, _ := url.Parse(p[1])
		if !sameOrigin(a, b) {
			t.Errorf("%s and %s: not the same origin", p[0], p[1])
		}
	}
	for _, p := range diff {
		a, _ := url.Parse(p[0])
		b, _ := url.Parse(p[1])
		if sameOrigin(a, b) {
			t.Errorf("%s and %s: the same origin", p[0], p[1])
		}
	}
}

func TestPageCap(t *testing.T) {
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		limit := r.URL.Query().Get("pages")
		if limit == "" || fmt.Sprint(n) != limit {
			w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?pages=%s&page=%d>; rel="next"`, srv.URL, limit, n+1))
		}
		fmt.Fprint(w, `[1]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)

	items, err := c.GetAll("/tags?pages=1000")
	if err != nil || len(items) != MaxPages {
		t.Errorf("1000 pages: items=%d err=%v", len(items), err)
	}
	hits.Store(0)
	items, err = c.GetAll("/tags")
	if err == nil || !strings.Contains(err.Error(), "1000") || items != nil {
		t.Errorf("endless pages: items=%d err=%v", len(items), err)
	}
	if got := hits.Load(); got != MaxPages {
		t.Errorf("endless pages: %d requests, want %d", got, MaxPages)
	}
}

func TestEmptyPage(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=2>; rel="next"`, srv.URL))
			fmt.Fprint(w, `[{"title":"a"}]`)
		case "2":
			w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=3>; rel="next"`, srv.URL))
		case "3":
			fmt.Fprint(w, `[{"title":"c"}]`)
		}
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	items, err := c.GetAll("/tags")
	if err != nil || len(items) != 2 {
		t.Errorf("empty page: items=%s err=%v", items, err)
	}
}

func TestPaginationErrorReturnsNil(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			http.Error(w, "boom", 500)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=2>; rel="next"`, srv.URL))
		fmt.Fprint(w, `[{"title":"a"}]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	items, err := c.GetAll("/tags")
	if err == nil || items != nil {
		t.Errorf("items=%s err=%v", items, err)
	}
}

func TestEachPageStops(t *testing.T) {
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=%d>; rel="next"`, srv.URL, hits.Load()+1))
		fmt.Fprint(w, `[1]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	pages := 0
	err := c.EachPage("/tags", func(payload []byte) (bool, error) {
		pages++
		return pages == 2, nil
	})
	if err != nil || pages != 2 || hits.Load() != 2 {
		t.Errorf("err=%v pages=%d hits=%d", err, pages, hits.Load())
	}
}

func TestTokenGoesOnlyToTheConfiguredOrigin(t *testing.T) {
	other := newSpy(t)
	main := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer main.Close()
	c, _ := tlsClient(t, main)
	if _, _, _, err := c.Do(http.MethodGet, other.URL+"/1/tags", nil); err == nil {
		t.Error("an authenticated request went to another origin")
	}
	other.check(t, 0)
	if _, _, _, err := c.Do(http.MethodGet, other.URL+"/blob", nil, NoAuth(), NotAccountScoped()); err != nil {
		t.Errorf("NoAuth request to another origin: %v", err)
	}
	other.check(t, 1)
}

func TestRedirectHTTPSToHTTPIsRefused(t *testing.T) {
	other := newSpy(t)
	main := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/1/tags", http.StatusFound)
	}))
	defer main.Close()
	c, _ := tlsClient(t, main)
	err := c.Get("/tags", nil)
	if err == nil || !strings.Contains(err.Error(), "https to http") {
		t.Errorf("err = %v", err)
	}
	other.check(t, 0)
}

func TestRedirectToAnotherOriginDropsToken(t *testing.T) {
	other := newSpy(t)
	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T" {
			t.Errorf("first request auth = %q", r.Header.Get("Authorization"))
		}
		http.Redirect(w, r, other.URL+"/1/tags", http.StatusFound)
	}))
	defer main.Close()
	c, _, _ := newTestClient(t, main, false)
	if err := c.Get("/tags", nil); err != nil {
		t.Errorf("err = %v", err)
	}
	other.check(t, 1)
}

func TestRedirectBetweenTLSHostsDropsToken(t *testing.T) {
	var sawAuth atomic.Bool
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth.Store(r.Header.Get("Authorization") != "")
		fmt.Fprint(w, `{}`)
	}))
	defer other.Close()
	main := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/1/tags", http.StatusMovedPermanently)
	}))
	defer main.Close()
	c, _ := tlsClient(t, main)
	if err := c.Get("/tags", nil); err != nil {
		t.Errorf("err = %v", err)
	}
	if sawAuth.Load() {
		t.Error("the token followed a redirect to another port")
	}
}

func TestRedirectSameOriginKeepsToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1/old" {
			http.Redirect(w, r, "/1/new", http.StatusFound)
			return
		}
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	if err := c.Get("/old", nil); err != nil || auth != "Bearer T" {
		t.Errorf("err=%v auth=%q", err, auth)
	}
}

func TestRedirectedWriteIsAnError(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, code := range []int{301, 302, 303} {
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method)
				w.Header().Set("Location", "/1/elsewhere")
				w.WriteHeader(code)
			}))
			c, _, logPath := newTestClient(t, srv, false)
			status, _, _, err := c.Do(method, "/cards", map[string]string{"title": "gate"})
			srv.Close()
			if err == nil || !strings.Contains(err.Error(), "redirected a write") || !strings.Contains(err.Error(), "https") {
				t.Errorf("%s %d: err = %v", method, code, err)
			}
			var ae *APIError
			if !errors.As(err, &ae) || status != code {
				t.Errorf("%s %d: status=%d err=%T", method, code, status, err)
			}
			if len(requests) != 1 || requests[0] != method {
				t.Errorf("%s %d: requests = %v", method, code, requests)
			}
			if data, _ := os.ReadFile(logPath); !strings.Contains(string(data), fmt.Sprintf(`"status": %d`, code)) {
				t.Errorf("%s %d: log = %q", method, code, data)
			}
		}
	}
}

func TestNetworkErrorHidesTheQuery(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	c, _, logPath := newTestClient(t, dead, false)
	signed := deadURL + "/storage/gate-key/hinge.jpg?X-Amz-Signature=SECRETSIG&X-Amz-Credential=SECRETCRED"
	_, _, _, err := c.Do(http.MethodPut, signed, nil, Raw([]byte("img")), NoAuth(), NotAccountScoped())
	if err == nil {
		t.Fatal("want a network error")
	}
	msg := err.Error()
	if strings.Contains(msg, "SECRET") || strings.Contains(msg, "X-Amz") || strings.Contains(msg, "?") {
		t.Errorf("error shows the query: %q", msg)
	}
	if !strings.Contains(msg, deadURL+"/storage/gate-key/hinge.jpg") {
		t.Errorf("error lacks scheme, host and path: %q", msg)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("a write with no response was not logged: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["status"] != float64(0) || rec["path"] != "/storage/gate-key/hinge.jpg" || strings.Contains(string(data), "SECRET") {
		t.Errorf("log = %s", data)
	}
}

func TestActionLogMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c, _, logPath := newTestClient(t, srv, false)
	if _, err := c.Write("POST", "/x", nil); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(logPath); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("new log: mode=%v err=%v", fi.Mode().Perm(), err)
	}
	if err := os.Chmod(logPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write("POST", "/x", nil); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(logPath); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("existing log: mode=%v err=%v", fi.Mode().Perm(), err)
	}
}

func TestResponseBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := 32 << 20
		if r.URL.Path == "/1/big" {
			n++
		}
		w.Write(bytes.Repeat([]byte(" "), n))
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	if _, _, p, err := c.Do(http.MethodGet, "/fits", nil); err != nil || len(p) != 32<<20 {
		t.Errorf("32 MiB: len=%d err=%v", len(p), err)
	}
	if _, _, p, err := c.Do(http.MethodGet, "/big", nil); err == nil || !strings.Contains(err.Error(), "32 MiB") || p != nil {
		t.Errorf("over 32 MiB: len=%d err=%v", len(p), err)
	}
}

func TestAPIErrorStripsEscapes(t *testing.T) {
	e := &APIError{Status: 500, Method: "GET", Path: "/p", Body: "\x1b]0;pwned\x07\x1b[2Jgate\r\nhinge\x1b[0m"}
	if got, want := e.Error(), "GET /p -> HTTP 500: gate  hinge"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConcurrentAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
	}))
	defer srv.Close()
	cfg := &config.Config{URL: srv.URL, Account: "1", TokenCmd: "printf T", LogPath: filepath.Join(t.TempDir(), "log")}
	c := New(cfg, false, &bytes.Buffer{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Get("/x", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// One list may not pass maxListBytes in all its pages together. The cap is
// 256 MiB; the test lowers it so it can run small.
func TestListByteCap(t *testing.T) {
	if maxListBytes != 256<<20 {
		t.Errorf("maxListBytes = %d, want 256 MiB", maxListBytes)
	}
	old := maxListBytes
	maxListBytes = 1000
	t.Cleanup(func() { maxListBytes = old })
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=%d>; rel="next"`, srv.URL, n+1))
		fmt.Fprint(w, "["+strings.Repeat(`"hinge",`, 40)+`"gate"]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	items, err := c.GetAll("/tags")
	if err == nil || !strings.Contains(err.Error(), "fz stopped") || items != nil {
		t.Errorf("items=%d err=%v", len(items), err)
	}
	if got := hits.Load(); got != 4 {
		t.Errorf("%d requests, want 4: the page that passes the cap is the last", got)
	}
}

// A storage request names only the host and the status in its error. The
// storage service's reply can echo signing details, so its body is never
// printed.
func TestStorageErrorHasNoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<Error><Code>SignatureDoesNotMatch</Code><AWSAccessKeyId>GATEKEY</AWSAccessKeyId><StringToSign>HINGE</StringToSign></Error>`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	status, _, payload, err := c.Do(http.MethodPut, srv.URL+"/bucket/object", nil,
		Stream(strings.NewReader("jpg"), 3), NoAuth(), NotAccountScoped(), Upload(), LogHost())
	if status != http.StatusForbidden || err == nil {
		t.Fatalf("status %d, err %v", status, err)
	}
	if msg := err.Error(); strings.Contains(msg, "GATEKEY") || strings.Contains(msg, "HINGE") || strings.Contains(msg, "Signature") || !strings.Contains(msg, "HTTP 403") {
		t.Errorf("error %q", msg)
	}
	if len(payload) != 0 {
		t.Errorf("payload %q", payload)
	}
}

// A storage request that fails to connect names only the host. The signed
// path can hold the signing key, so neither it nor the query is printed.
func TestStorageNetworkErrorHasNoPath(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, _, _ := newTestClient(t, srv, false)
	srv.Close()
	_, _, _, err := c.Do(http.MethodPut, srv.URL+"/disk/HINGEKEY/gate.jpg?signature=GATESIG", nil,
		Stream(strings.NewReader("jpg"), 3), NoAuth(), NotAccountScoped(), Upload(), LogHost())
	if err == nil {
		t.Fatal("want a connection error")
	}
	if msg := err.Error(); strings.Contains(msg, "HINGEKEY") || strings.Contains(msg, "GATESIG") || !strings.Contains(msg, strings.TrimPrefix(srv.URL, "http://")) {
		t.Errorf("error %q", msg)
	}
}

// One list read, all its pages together, has a time limit. A server that
// answers each page slowly cannot hold fz for hours: past the limit the
// read stops with an error, even in the middle of a page.
func TestListTimeLimit(t *testing.T) {
	if ListTimeout != 10*time.Minute {
		t.Errorf("ListTimeout = %v, want 10 minutes", ListTimeout)
	}
	old := ListTimeout
	ListTimeout = 400 * time.Millisecond
	t.Cleanup(func() { ListTimeout = old })
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Link", fmt.Sprintf(`<%s/1/tags?page=%d>; rel="next"`, srv.URL, n+1))
		fmt.Fprint(w, `["hinge"]`)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	start := time.Now()
	items, err := c.GetAll("/tags")
	if err == nil || !strings.Contains(err.Error(), "took more than") || items != nil {
		t.Errorf("items=%d err=%v", len(items), err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
	if got := hits.Load(); got > 4 {
		t.Errorf("%d requests, want at most 4", got)
	}
}
