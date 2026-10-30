package fizzy

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The board id from Location goes into the postpone PUT escaped, and an id
// that cannot be a path segment is refused.
func TestCreateBoardLocationIDIsChecked(t *testing.T) {
	loc := "/1/boards/abc%3Fall_access=false.json"
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.Method == http.MethodPost {
			w.Header().Set("Location", loc)
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(204)
	})
	if _, err := f.CreateBoard("Gate", 30, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.calls) != 2 || s.calls[1].Path != "/1/boards/abc?all_access=false" || s.calls[1].Query != "" {
		t.Fatalf("calls %+v", s.calls)
	}

	loc = "/1/boards/...json"
	s.calls = nil
	if _, err := f.CreateBoard("Gate", 30, nil); err == nil || !strings.Contains(err.Error(), "board 'Gate' was created") {
		t.Errorf("dot id: %v", err)
	}
	if len(s.calls) != 1 {
		t.Errorf("dot id sent %+v", s.calls)
	}
}

// With no Location there is no id for the postpone PUT. CreateBoard says so
// instead of reporting success.
func TestCreateBoardNoLocationWithPostpone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.WriteHeader(201) })
	_, err := f.CreateBoard("Gate", 30, nil)
	if err == nil || err.Error() != "board 'Gate' was created, but Fizzy sent no id for it, so the auto-postpone period was not set" {
		t.Errorf("err = %v", err)
	}
	if len(s.calls) != 1 {
		t.Errorf("calls %+v", s.calls)
	}
	// With no period and no columns it is still an error, as for a card.
	s.calls = nil
	if id, err := f.CreateBoard("Gate", 0, nil); !errors.Is(err, ErrNoLocation) || id != "" {
		t.Errorf("no postpone: %q %v", id, err)
	}
}

// The user id from /my/identity goes into the export paths only if it can be
// a path segment.
func TestExportChecksIdentityUserID(t *testing.T) {
	ident := `{"id":"I1","accounts":[{"id":"A1","name":"Gate Works","slug":"/1","user":{"id":".."}}]}`
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, ident)
	})
	if _, err := f.StartMyExport(); !IsRefusal(err) {
		t.Errorf("start: %v", err)
	}
	if _, err := f.MyExport("E1"); !IsRefusal(err) {
		t.Errorf("status: %v", err)
	}
	for _, c := range s.calls {
		if c.Path != "/my/identity" {
			t.Errorf("sent %s %s", c.Method, c.Path)
		}
	}
}

// A reply that is not a webhook with an id is an error, not an empty
// success. Dry run still returns an empty webhook and no error.
func TestUpdateWebhookUnreadableReply(t *testing.T) {
	name := "Gate"
	for _, reply := range []string{`{"id":5}`, `{}`, ``, `not json`} {
		f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, reply)
		})
		if _, err := f.UpdateWebhook("B1", "H1", &name, nil); err == nil || !strings.Contains(err.Error(), "webhook H1") {
			t.Errorf("reply %q: err = %v", reply, err)
		}
	}
	f, s, _, _ := newFizzy(t, true, nil)
	if w, err := f.UpdateWebhook("B1", "H1", &name, nil); err != nil || w.ID != "" || len(s.calls) != 0 {
		t.Errorf("dry run: %+v %v %+v", w, err, s.calls)
	}
}

// Without attachable_sgid, Upload falls back to signed_id, which renders as
// a broken attachment. It still returns it, and warns on the client's
// output.
func TestUploadSignedIDFallbackWarns(t *testing.T) {
	f, _, out, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, map[string]any{"signed_id": "BLOB", "direct_upload": map[string]any{"url": "/put"}})
			return
		}
		w.WriteHeader(204)
	})
	p := filepath.Join(t.TempDir(), "hinge.png")
	os.WriteFile(p, []byte("x"), 0o644)
	sgid, err := f.Upload(p)
	if err != nil || sgid != "BLOB" {
		t.Fatalf("sgid=%q err=%v", sgid, err)
	}
	want := "fz: warning: Fizzy sent no attachable_sgid for hinge.png, so fz used its signed_id; the attachment may not render\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}
