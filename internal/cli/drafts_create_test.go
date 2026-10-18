package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/client"
)

// createServer is a Fizzy with board Home and card 90 whose card create
// runs create. It counts the creates.
func createServer(t *testing.T, create func(w http.ResponseWriter, r *http.Request, srvURL string)) (*httptest.Server, *int64) {
	t.Helper()
	var creates int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/1/cards/90":
			io.WriteString(w, card90)
		case r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/cards":
			atomic.AddInt64(&creates, 1)
			create(w, r, srv.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &creates
}

// runCreate runs fz draft apply on a new draft against srv, twice, and
// returns both results and the draft text after each.
func runCreate(t *testing.T, srv *httptest.Server) (first, second result, after1, after2 string) {
	t.Helper()
	parityEnv(t, srv)
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n\nGate latches.\n")
	first = runGo([]string{"draft", "apply", path})
	after1 = syncReadDraft(t, path)
	second = runGo([]string{"draft", "apply", path})
	after2 = syncReadDraft(t, path)
	return
}

// A 201 with a Location names the card even when the reply body is cut
// short, so card: is written and the next apply skips the draft.
func TestApplyCreateBodyCutShort(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.Header().Set("Location", u+"/1/cards/90.json")
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"number":90`)
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	})
	first, second, after1, _ := runCreate(t, srv)
	if first.code != 0 || !strings.Contains(after1, "\ncard: 90\n") || strings.Contains(after1, "creating:") {
		t.Errorf("first run: %+v\n%s", first, after1)
	}
	if second.code != 0 || *creates != 1 {
		t.Errorf("second run: %+v, %d creates", second, *creates)
	}
}

// A create whose outcome is unknown leaves creating: in the draft and
// exits 1, saying a card may exist on the board. The next apply refuses
// the draft with exit 2 and sends nothing.
func TestApplyCreateUnknownOutcome(t *testing.T) {
	old := client.RequestTimeout
	client.RequestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { client.RequestTimeout = old })
	for name, create := range map[string]func(w http.ResponseWriter, r *http.Request, u string){
		"timeout": func(w http.ResponseWriter, r *http.Request, u string) { time.Sleep(600 * time.Millisecond) },
		"reset before the reply": func(w http.ResponseWriter, r *http.Request, u string) {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
		},
		"server error": func(w http.ResponseWriter, r *http.Request, u string) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"location with no number": func(w http.ResponseWriter, r *http.Request, u string) {
			w.Header().Set("Location", u+"/1/cards/90abc")
			w.WriteHeader(http.StatusCreated)
		},
		"no location": func(w http.ResponseWriter, r *http.Request, u string) { w.WriteHeader(http.StatusCreated) },
		// A 3xx may follow a create that ran, as Rails answers one outside
		// the JSON format, so it is an unknown outcome too.
		"302 to the new card": func(w http.ResponseWriter, r *http.Request, u string) {
			w.Header().Set("Location", u+"/1/cards/91")
			w.WriteHeader(http.StatusFound)
		},
		"303 with no location": func(w http.ResponseWriter, r *http.Request, u string) { w.WriteHeader(http.StatusSeeOther) },
		"location naming card 0": func(w http.ResponseWriter, r *http.Request, u string) {
			w.Header().Set("Location", u+"/1/cards/0")
			w.WriteHeader(http.StatusCreated)
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, creates := createServer(t, create)
			first, second, after1, after2 := runCreate(t, srv)
			if first.code != 1 || !strings.Contains(first.stderr, "may exist") || !strings.Contains(first.stderr, "board Home") ||
				!strings.Contains(first.stderr, "creating:") || strings.Contains(first.stderr, "no card created") {
				t.Errorf("first run: %+v", first)
			}
			if !strings.Contains(after1, "\ncreating: 20") || strings.Contains(after1, "card:") {
				t.Errorf("draft after the first run:\n%s", after1)
			}
			if second.code != 2 || !strings.Contains(second.stderr, "creating:") || after2 != after1 {
				t.Errorf("second run: %+v\n%s", second, after2)
			}
			if n := atomic.LoadInt64(creates); n != 1 {
				t.Errorf("%d creates, want 1", n)
			}
		})
	}
}

// A create the server refused with a 4xx made no card: the creating: line
// is removed and the draft can be applied again.
func TestApplyCreateRefusedClearsMarker(t *testing.T) {
	srv, creates := createServer(t, func(w http.ResponseWriter, r *http.Request, u string) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"title":["can't be blank"]}`)
	})
	first, second, after1, _ := runCreate(t, srv)
	if first.code != 1 || strings.Contains(after1, "creating:") || strings.Contains(first.stderr, "may exist") {
		t.Errorf("first run: %+v\n%s", first, after1)
	}
	if second.code != 1 || atomic.LoadInt64(creates) != 2 {
		t.Errorf("second run: %+v, %d creates", second, *creates)
	}
}
