package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// colorObject matches a column color in the {name, value} shape.
var colorObject = regexp.MustCompile(`"color":\{"name":"[^"]*","value":("[^"]*")\}`)

// stringColors serves what upstream serves, with every column color
// rewritten to the plain string shape Fizzy's API docs show.
func stringColors(t *testing.T, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(r *http.Response) error {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body.Close()
		b = colorObject.ReplaceAll(b, []byte(`"color":$1`))
		r.Body = io.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
		r.Header.Set("Content-Length", strconv.Itoa(len(b)))
		return nil
	}
	srv := httptest.NewServer(proxy)
	t.Cleanup(srv.Close)
	return srv
}

// card list, column list and review print the same thing whichever shape
// the server sends a column color in.
func TestStringColorsReadCommands(t *testing.T) {
	parity, review := fakeFizzy(t), reviewServer(t)
	for _, tc := range []struct {
		obj  *httptest.Server
		args []string
	}{
		{parity, []string{"card", "list", "--board", "Home"}},
		{parity, []string{"column", "list", "Home"}},
		{parity, []string{"show", "87"}},
		{review, []string{"review", "Garage"}},
	} {
		obj, args := tc.obj, tc.args
		str := stringColors(t, obj)
		parityEnv(t, obj)
		want := runGo(args)
		parityEnv(t, str)
		got := runGo(args)
		if want.code != 0 {
			t.Fatalf("fz %q with object colors: %+v", args, want)
		}
		got.stdout = strings.ReplaceAll(got.stdout, str.URL, obj.URL)
		if got != want {
			t.Errorf("fz %q with string colors\n got  %+v\n want %+v", args, got, want)
		}
	}
}

// draft apply --update finds the draft in sync when the card's column color
// is a string, so the fingerprint does not depend on the color shape.
func TestStringColorsDraftUpdate(t *testing.T) {
	srv := newSyncServer(t)
	srv.edit(func(c *cardState) {
		c.column = `{"id":"c1","name":"Next","color":{"name":"Blue","value":"var(--color-card-4)"}}`
	})
	str := stringColors(t, srv.Server)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, srv.draft(t, strings.Replace(syncDraftText, "column: Maybe?", "column: Next", 1)))
	got := syncRun(t, &syncServer{Server: str}, "draft", "apply", "--update", path)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
}
