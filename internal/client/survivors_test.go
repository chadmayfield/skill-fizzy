package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// The token goes in Authorization and in no other header.
func TestTokenOnlyInAuthorization(t *testing.T) {
	var mu sync.Mutex
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Clone()
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		if _, _, _, err := c.Do(m, "/cards/87", map[string]string{"a": "b"}); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		for k, vs := range got {
			for _, v := range vs {
				if strings.Contains(v, "T") && k != "Authorization" && k != "Content-Type" && k != "Accept" {
					t.Errorf("%s: header %s = %q", m, k, v)
				}
			}
		}
		if a := got.Values("Authorization"); len(a) != 1 || a[0] != "Bearer T" {
			t.Errorf("%s: Authorization %q", m, a)
		}
		for k := range got {
			switch k {
			case "Accept", "User-Agent", "Authorization", "Content-Type", "Accept-Encoding", "Content-Length":
			default:
				t.Errorf("%s: unexpected header %s", m, k)
			}
		}
		mu.Unlock()
	}
}

// A GET that redirects without end stops after 10 hops and returns the
// last 3xx as an error.
func TestRedirectCap(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/1/hop/%d", n), http.StatusFound)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	_, _, _, err := c.Do(http.MethodGet, "/hop/0", nil)
	var api *APIError
	if !errors.As(err, &api) || api.Status != http.StatusFound {
		t.Errorf("err %v", err)
	}
	if n := hits.Load(); n != 10 {
		t.Errorf("%d requests, want 10", n)
	}
}

// A next link to another origin stops the list with the pagination
// message, before any request to it.
func TestNextLinkOtherOrigin(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Link", `<https://fizzy.example.com/1/tags?page=2>; rel="next"`)
		w.Write([]byte(`["hinge"]`))
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	_, err := c.GetAll("/tags")
	if err == nil || !strings.Contains(err.Error(), "the next link points to https://fizzy.example.com, not the configured server") {
		t.Errorf("err %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d requests", n)
	}
}
