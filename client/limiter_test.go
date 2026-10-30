package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// countLimiter counts waits and fails them with err when err is set.
type countLimiter struct {
	waits atomic.Int32
	err   error
}

func (l *countLimiter) Wait(ctx context.Context) error {
	l.waits.Add(1)
	if l.err != nil {
		return l.err
	}
	return ctx.Err()
}

func TestLimiterCountsEveryRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/1/old":
			w.Header().Set("Location", "/1/new")
			w.WriteHeader(302)
		case "/1/list":
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("Link", `</1/list?page=2>; rel="next"`)
			}
			fmt.Fprint(w, `[{}]`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	lim := &countLimiter{}
	c.UseLimiter(lim)

	if err := c.Get("/old", nil); err != nil {
		t.Fatal(err)
	}
	if items, err := c.GetAll("/list"); err != nil || len(items) != 2 {
		t.Fatalf("GetAll: items=%d err=%v", len(items), err)
	}
	if got := lim.waits.Load(); got != 4 || hits.Load() != 4 {
		t.Errorf("waits = %d, requests = %d; want 4 and 4", got, hits.Load())
	}
}

func TestLimiterErrorSendsNothing(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	stop := errors.New("limit reached")
	c.UseLimiter(&countLimiter{err: stop})
	if _, _, _, err := c.DoCtx(context.Background(), http.MethodGet, "/cards", nil); !errors.Is(err, stop) {
		t.Fatalf("err = %v, want the limiter's error", err)
	}
	if hits.Load() != 0 {
		t.Errorf("server got %d requests", hits.Load())
	}
}

func TestDryRunWriteDoesNotWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, true)
	lim := &countLimiter{}
	c.UseLimiter(lim)
	if _, err := c.Write(http.MethodPost, "/cards", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if lim.waits.Load() != 0 {
		t.Errorf("waits = %d for a write that was not sent", lim.waits.Load())
	}
}

// A caller's http.Client keeps its transport but gets the client's redirect
// policy, so a redirected write is still not followed.
func TestUseHTTPClientKeepsRedirectPolicy(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Location", "/1/elsewhere")
		w.WriteHeader(307)
	}))
	defer srv.Close()
	c, _, _ := newTestClient(t, srv, false)
	follow := &http.Client{
		Transport:     srv.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
	}
	c.UseHTTPClient(follow)
	if _, err := c.Write(http.MethodPost, "/cards", map[string]int{"a": 1}); err == nil {
		t.Error("POST 307 must be an error")
	}
	if len(methods) != 1 {
		t.Errorf("requests = %v, want one POST", methods)
	}
	if follow.CheckRedirect == nil {
		t.Error("the caller's http.Client was changed")
	}
}
