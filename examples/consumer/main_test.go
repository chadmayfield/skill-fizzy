// Package consumer_test builds against skill-fizzy from a separate module, the
// way outside code would, so a package that only compiles from inside the
// module fails here.
package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/chadmayfield/skill-fizzy/fizzyapi"
)

const (
	account   = "1"
	token     = "test-token"
	boardID   = "03abcboard00000000000000mn"
	columnID  = "03abccolumn0000000000000mn"
	stepID    = "03abc123def456ghi789jkl0mn"
	commentID = "03abccomment000000000000mn"
)

// countingLimiter counts Wait calls. It stands in for *rate.Limiter from
// golang.org/x/time/rate, which has the same Wait method.
type countingLimiter struct {
	mu    sync.Mutex
	waits int
}

func (l *countingLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.waits++
	return ctx.Err()
}

func (l *countingLimiter) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.waits
}

// server is a fake Fizzy that counts the requests it answers and records
// the body of the last triage call.
type server struct {
	mu       sync.Mutex
	requests int
	triage   string
}

func (s *server) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Errorf("encode: %v", err)
		}
	}
	card := func(n int, title string) map[string]any {
		return map[string]any{
			"number": n, "title": title,
			"board":  map[string]any{"id": boardID, "name": "Gate"},
			"column": nil,
			"steps":  []map[string]any{{"id": stepID, "content": "Oil the hinge", "completed": true}},
		}
	}
	prefix := "/" + account
	mux.HandleFunc("GET "+prefix+"/cards", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("board_ids[]") != boardID {
			t.Errorf("cards query = %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page") == "" {
			// The first page links to a second, so the list takes two requests.
			w.Header().Set("Link", fmt.Sprintf("<%s/cards?board_ids%%5B%%5D=%s&page=2>; rel=\"next\"", prefix, boardID))
			writeJSON(w, []any{card(87, "Fix the gate")})
			return
		}
		writeJSON(w, []any{card(88, "Paint the gate")})
	})
	mux.HandleFunc("GET "+prefix+"/cards/87", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, card(87, "Fix the gate"))
	})
	mux.HandleFunc("GET "+prefix+"/activities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{map[string]any{
			"id": "03abcactivity000000000000mn", "action": "card_triaged",
			"particulars": map[string]any{"column": "Working"},
			"eventable":   map[string]any{"number": 87},
			"creator":     map[string]any{"id": "03abcuser00000000000000mn", "name": "Gate keeper"},
		}})
	})
	mux.HandleFunc("GET "+prefix+"/boards/"+boardID+"/columns", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{map[string]any{"id": columnID, "name": "Working"}})
	})
	mux.HandleFunc("GET "+prefix+"/boards/"+boardID+"/columns/"+columnID+"/cards", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{card(87, "Fix the gate")})
	})
	mux.HandleFunc("POST "+prefix+"/boards/"+boardID+"/cards", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", prefix+"/cards/89")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST "+prefix+"/cards/87/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", prefix+"/cards/87/comments/"+commentID)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST "+prefix+"/cards/87/triage", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ColumnID string `json:"column_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("triage body: %v", err)
		}
		s.mu.Lock()
		s.triage = body.ColumnID
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests++
		s.mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("%s %s: Authorization = %q", r.Method, r.URL.Path, got)
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

func newClient(t *testing.T) (*fizzy.Fizzy, *server, *countingLimiter) {
	t.Helper()
	srv := &server{}
	ts := httptest.NewServer(srv.handler(t))
	t.Cleanup(ts.Close)
	lim := &countingLimiter{}
	f, err := fizzyapi.New(fizzyapi.Options{
		BaseURL:    ts.URL,
		Account:    account,
		Token:      token,
		HTTPClient: ts.Client(),
		Limiter:    lim,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, srv, lim
}

func TestConsumer(t *testing.T) {
	f, srv, lim := newClient(t)
	ctx := context.Background()

	cards, err := f.CardsCtx(ctx, boardID, nil)
	if err != nil {
		t.Fatalf("Cards: %v", err)
	}
	if len(cards) != 2 || cards[0].Number != 87 || cards[1].Number != 88 {
		t.Fatalf("cards = %+v", cards)
	}

	var card fizzy.Card
	card, err = f.CardCtx(ctx, 87)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	var step fizzy.CardStep = card.Steps[0]
	if card.Title != "Fix the gate" || card.Board.ID != boardID || step.ID != stepID || !step.Completed {
		t.Fatalf("card = %+v", card)
	}

	acts, err := f.ActivitiesCtx(ctx, boardID, "", 10)
	if err != nil {
		t.Fatalf("Activities: %v", err)
	}
	if len(acts) != 1 || acts[0].Particulars["column"] != "Working" || acts[0].Creator == nil {
		t.Fatalf("activities = %+v", acts)
	}

	target, err := f.ResolveMoveTarget(boardID, columnID)
	if err != nil {
		t.Fatalf("ResolveMoveTarget: %v", err)
	}
	if err := f.MoveCardToCtx(ctx, 87, target); err != nil {
		t.Fatalf("MoveCardTo: %v", err)
	}
	if srv.triage != columnID {
		t.Fatalf("triage column = %q, want %q", srv.triage, columnID)
	}

	// Each page of the card list is one request, and the limiter saw every
	// request the server answered.
	if got, want := srv.count(), 6; got != want {
		t.Errorf("server requests = %d, want %d", got, want)
	}
	if got := lim.count(); got != srv.count() {
		t.Errorf("limiter waits = %d, server requests = %d", got, srv.count())
	}

	// The calls without a context still work, and are throttled too.
	if _, err := f.Card(87); err != nil {
		t.Fatalf("Card: %v", err)
	}
	if got := lim.count(); got != srv.count() {
		t.Errorf("after Card: limiter waits = %d, server requests = %d", got, srv.count())
	}
}

func TestConsumerContextCanceled(t *testing.T) {
	f, srv, _ := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.CardCtx(ctx, 87); !errors.Is(err, context.Canceled) {
		t.Fatalf("CardCtx with a canceled context: err = %v, want context.Canceled", err)
	}
	if srv.count() != 0 {
		t.Fatalf("server got %d requests after cancel", srv.count())
	}
}

func TestConsumerMoreCtxCalls(t *testing.T) {
	f, srv, lim := newClient(t)
	ctx := context.Background()
	cols, err := f.ColumnsCtx(ctx, boardID)
	if err != nil || len(cols) != 1 || cols[0].ID != columnID {
		t.Fatalf("ColumnsCtx: %+v, %v", cols, err)
	}
	cards, err := f.ColumnCardsCtx(ctx, boardID, columnID)
	if err != nil || len(cards) != 1 || cards[0].Number != 87 {
		t.Fatalf("ColumnCardsCtx: %+v, %v", cards, err)
	}
	target, err := f.ResolveMoveTargetCtx(ctx, boardID, "Working")
	if err != nil {
		t.Fatalf("ResolveMoveTargetCtx: %v", err)
	}
	if err := f.MoveCardToCtx(ctx, 87, target); err != nil || srv.triage != columnID {
		t.Fatalf("MoveCardToCtx: column %q, %v", srv.triage, err)
	}
	n, err := f.CreateCardCtx(ctx, boardID, "Oil the hinge", "", "")
	if err != nil || n != 89 {
		t.Fatalf("CreateCardCtx: %d, %v", n, err)
	}
	id, err := f.AddCommentCtx(ctx, 87, "<p>Done</p>")
	if err != nil || id != commentID {
		t.Fatalf("AddCommentCtx: %q, %v", id, err)
	}
	if got := lim.count(); got != srv.count() {
		t.Errorf("limiter waits = %d, server requests = %d", got, srv.count())
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	before := srv.count()
	if _, err := f.AddCommentCtx(canceled, 87, "<p>x</p>"); !errors.Is(err, context.Canceled) {
		t.Errorf("AddCommentCtx with a canceled context: %v", err)
	}
	if _, err := f.CreateCardCtx(canceled, boardID, "x", "", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateCardCtx with a canceled context: %v", err)
	}
	if srv.count() != before {
		t.Errorf("server got %d requests after cancel", srv.count()-before)
	}
}

// nilPointerLimiter satisfies the limiter interface through a pointer, so a
// nil one is a typed nil.
type nilPointerLimiter struct{}

func (*nilPointerLimiter) Wait(context.Context) error { return nil }

func TestNewRefusesBadOptions(t *testing.T) {
	lim := &countingLimiter{}
	var typedNil *nilPointerLimiter
	for name, opts := range map[string]fizzyapi.Options{
		"no URL":                {Account: account, Token: token, Limiter: lim},
		"plain http":            {BaseURL: "http://fizzy.example.com", Account: account, Token: token, Limiter: lim},
		"no account":            {BaseURL: "https://fizzy.example.com", Token: token, Limiter: lim},
		"bad account":           {BaseURL: "https://fizzy.example.com", Account: "a/b", Token: token, Limiter: lim},
		"no token":              {BaseURL: "https://fizzy.example.com", Account: account, Limiter: lim},
		"nil limiter":           {BaseURL: "https://fizzy.example.com", Account: account, Token: token},
		"typed nil limiter":     {BaseURL: "https://fizzy.example.com", Account: account, Token: token, Limiter: typedNil},
		"service without flag":  {BaseURL: "http://fizzy.fizzy.svc", Account: account, Token: token, Limiter: lim},
		"long name, no flag":    {BaseURL: "http://fizzy.fizzy.svc.cluster.local:80", Account: account, Token: token, Limiter: lim},
		"IP with flag":          {BaseURL: "http://10.0.0.5", Account: account, Token: token, Limiter: lim, AllowPlainHTTP: true},
		"public host with flag": {BaseURL: "http://example.com", Account: account, Token: token, Limiter: lim, AllowPlainHTTP: true},
		"svc mid-name, flag":    {BaseURL: "http://evil.svc.example.com", Account: account, Token: token, Limiter: lim, AllowPlainHTTP: true},
		"bare .svc with flag":   {BaseURL: "http://.svc", Account: account, Token: token, Limiter: lim, AllowPlainHTTP: true},
	} {
		_, err := fizzyapi.New(opts)
		if err == nil {
			t.Errorf("%s: New returned no error", name)
			continue
		}
		t.Logf("%s: %v", name, err)
	}
}

func TestNewAllowsClusterServiceHTTP(t *testing.T) {
	for _, u := range []string{
		"http://fizzy.fizzy.svc",
		"http://fizzy.fizzy.svc.cluster.local:80",
		"https://fizzy.example.com",
	} {
		_, err := fizzyapi.New(fizzyapi.Options{BaseURL: u, Account: account, Token: token,
			Limiter: &countingLimiter{}, AllowPlainHTTP: true})
		if err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
}
