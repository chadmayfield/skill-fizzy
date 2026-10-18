package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A Location header whose id holds a newline cannot forge a line of fz's
// output. fz refuses the id, says the object was created and exits 1.
func TestLocationIDCannotForgeLines(t *testing.T) {
	const evil = "x9%0Aboard%20'Evil'%20deleted%0Afz:%20FORGED%20all%20done"
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1/boards":
			io.WriteString(w, `[{"id":"b1","name":"Home"},{"id":"b2","name":"Gate"}]`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/columns"):
			io.WriteString(w, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/1/boards/b1/columns":
			w.Header().Set("Location", "/1/boards/b1/columns/"+evil+".json")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/1/boards/") && strings.HasSuffix(r.URL.Path, "/columns"):
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPost:
			w.Header().Set("Location", r.URL.Path+"/"+evil+".json")
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"Not found"}`)
		}
	})
	parityEnv(t, srv)
	for _, argv := range [][]string{
		{"board", "create", "Garden"},
		{"board", "create", "Garden", "--columns", "Now"},
		{"column", "create", "Home", "Later"},
		{"step", "add", "87", "Oil the hinge"},
		{"comment", "add", "87", "Gate latches."},
		{"webhook", "create", "Gate", "--name", "Hinge alerts", "--url", "https://fizzy.example.com/hook", "--actions", "card_closed", "--yes"},
	} {
		r := runGo(argv)
		for _, out := range []string{r.stdout, r.stderr} {
			for _, l := range strings.Split(out, "\n") {
				if strings.HasPrefix(l, "board 'Evil'") || strings.HasPrefix(l, "fz: FORGED") {
					t.Errorf("fz %q printed a forged line %q in %q", argv, l, out)
				}
			}
		}
		if r.code != 1 {
			t.Errorf("fz %q: exit %d, want 1: %+v", argv, r.code, r)
		}
	}
}
