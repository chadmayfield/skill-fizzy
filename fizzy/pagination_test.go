package fizzy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// otherHost is a second server that records whether it was reached and
// whether it saw the token.
func otherHost(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	var hits atomic.Int32
	var sawAuth atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			sawAuth.Store(true)
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &sawAuth
}

func TestListUpToRefusesAnotherOrigin(t *testing.T) {
	other, hits, sawAuth := otherHost(t)
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Link", "<"+other.URL+"/1/activities?page=2>; rel=\"next\"")
		fmt.Fprint(w, `[{"id":"a1"}]`)
	})
	acts, err := f.Activities("", "", 0)
	if err == nil || acts != nil {
		t.Errorf("acts=%v err=%v", acts, err)
	}
	if hits.Load() != 0 || sawAuth.Load() {
		t.Errorf("other host: hits=%d sawAuth=%v", hits.Load(), sawAuth.Load())
	}
}

func TestBoardAccessesRefusesAnotherOrigin(t *testing.T) {
	other, hits, sawAuth := otherHost(t)
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Link", "<"+other.URL+"/1/boards/b1/accesses?page=2>; rel=\"next\"")
		jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []any{map[string]any{"id": "u1"}}})
	})
	acc, err := f.BoardAccesses("b1")
	if err == nil || acc.BoardID != "" || acc.Users != nil {
		t.Errorf("acc=%+v err=%v", acc, err)
	}
	if hits.Load() != 0 || sawAuth.Load() {
		t.Errorf("other host: hits=%d sawAuth=%v", hits.Load(), sawAuth.Load())
	}
}

// An empty 200 body is an empty page, as it is for every other list.
func TestBoardAccessesEmptyPage(t *testing.T) {
	var srvURL string
	f, _, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", "<"+srvURL+"/1/boards/b1/accesses?page=2>; rel=\"next\"")
			jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []any{map[string]any{"id": "u1"}}})
		case "2":
			w.Header().Set("Link", "<"+srvURL+"/1/boards/b1/accesses?page=3>; rel=\"next\"")
		case "3":
			jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []any{map[string]any{"id": "u2"}}})
		}
	})
	srvURL = srv.URL
	acc, err := f.BoardAccesses("b1")
	if err != nil || acc.BoardID != "b1" || len(acc.Users) != 2 {
		t.Errorf("acc=%+v err=%v", acc, err)
	}
}

// An error on a later page returns no partial result.
func TestBoardAccessesErrorReturnsNothing(t *testing.T) {
	var srvURL string
	f, _, _, srv := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Query().Get("page") == "2" {
			http.Error(w, "boom", 500)
			return
		}
		w.Header().Set("Link", "<"+srvURL+"/1/boards/b1/accesses?page=2>; rel=\"next\"")
		jsonOK(w, map[string]any{"board_id": "b1", "all_access": false, "users": []any{map[string]any{"id": "u1"}}})
	})
	srvURL = srv.URL
	acc, err := f.BoardAccesses("b1")
	if err == nil || acc.BoardID != "" || acc.Users != nil {
		t.Errorf("acc=%+v err=%v", acc, err)
	}
}

// listUpTo follows a relative link without a second account prefix, and
// stops reading pages once it holds limit items.
func TestListUpToRelativeLinkAndLimit(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		n := 1
		fmt.Sscan(r.URL.Query().Get("page"), &n)
		w.Header().Set("Link", fmt.Sprintf("</1/activities?page=%d>; rel=\"next\"", n+1))
		fmt.Fprintf(w, `[{"id":"a%d"},{"id":"b%d"}]`, n, n)
	})
	acts, err := f.Activities("", "", 3)
	if err != nil || len(acts) != 3 {
		t.Fatalf("acts=%v err=%v", acts, err)
	}
	if len(s.calls) != 2 {
		t.Fatalf("calls = %+v", s.calls)
	}
	for _, c := range s.calls {
		if c.Path != "/1/activities" || strings.Contains(c.Path, "/1/1/") {
			t.Errorf("path = %q", c.Path)
		}
	}
}
