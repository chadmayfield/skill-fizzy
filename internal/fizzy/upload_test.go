package fizzy

import (
	"io"
	"net/http/httptest"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// uploadFizzy answers the direct upload with storage as the storage URL
// and accepts the PUT.
func uploadFizzy(t *testing.T, dryRun bool, storage func(srvURL string) string) (*Fizzy, *stub) {
	t.Helper()
	var srvURL string
	f, s, _, srv := newFizzy(t, dryRun, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, map[string]any{"signed_id": "B", "attachable_sgid": "A",
				"direct_upload": map[string]any{"url": storage(srvURL)}})
			return
		}
		if r.Method == http.MethodPut && r.Header.Get("Content-Length") == "" {
			t.Error("storage PUT has no Content-Length")
		}
		w.WriteHeader(204)
	})
	srvURL = srv.URL
	return f, s
}

func smallFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hinge.png")
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A folder, a device and a file over 100 MiB are refused before anything
// is read or sent, in a dry run too.
func TestUploadRefusesWhatItCannotSend(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.bin")
	fh, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := fh.Truncate(MaxUploadSize + 1); err != nil {
		t.Fatal(err)
	}
	fh.Close()
	exact := filepath.Join(t.TempDir(), "exact.bin")
	fh, _ = os.Create(exact)
	fh.Truncate(MaxUploadSize)
	fh.Close()
	for _, dry := range []bool{false, true} {
		for _, p := range []string{t.TempDir(), os.DevNull, big} {
			f, s := uploadFizzy(t, dry, func(u string) string { return u + "/put" })
			_, err := f.Upload(p)
			if err == nil || !IsRefusal(err) {
				t.Errorf("dry %v, %s: err %v, want a refusal", dry, p, err)
			}
			if len(s.calls) != 0 {
				t.Errorf("dry %v, %s: sent %d requests", dry, p, len(s.calls))
			}
		}
	}
	// Exactly 100 MiB is allowed. A dry run reads it for the checksum only.
	f, s := uploadFizzy(t, true, func(u string) string { return u + "/put" })
	if sgid, err := f.Upload(exact); err != nil || sgid != "DRY-RUN-SGID" || len(s.calls) != 0 {
		t.Errorf("exact size: sgid %q err %v calls %d", sgid, err, len(s.calls))
	}
}

// The storage URL must be absolute https, the configured server, or a
// path that starts with one slash. Anything else is refused before the PUT.
func TestUploadStorageURL(t *testing.T) {
	for _, tc := range []struct {
		url  func(srvURL string) string
		ok   bool
		path string
	}{
		{func(u string) string { return u + "/disk/put?sig=1" }, true, "/disk/put"},
		{func(string) string { return "/disk/put" }, true, "/disk/put"},
		{func(string) string { return "//storage.example.com/put" }, false, ""},
		{func(string) string { return "@storage.example.com/put" }, false, ""},
		{func(string) string { return "disk/put" }, false, ""},
		{func(string) string { return "http://storage.example.com/put" }, false, ""},
		{func(string) string { return "ftp://storage.example.com/put" }, false, ""},
		{func(string) string { return "javascript:alert(1)" }, false, ""},
		{func(string) string { return "https:/put" }, false, ""},
		{func(string) string { return "https://gate:hinge@storage.example.com/put" }, false, ""},
	} {
		f, s := uploadFizzy(t, false, tc.url)
		_, err := f.Upload(smallFile(t))
		if tc.ok {
			if err != nil || len(s.calls) != 2 || s.calls[1].Path != tc.path {
				t.Errorf("%s: err %v, %d requests", tc.url("SRV"), err, len(s.calls))
			}
			continue
		}
		// A refusal, not a failed dial: the network guard in testenv.Main
		// fails any dial off loopback, and that error is no Refusal.
		if !IsRefusal(err) || len(s.calls) != 1 {
			t.Errorf("%s: err %v, %d requests, want a refusal before the PUT", tc.url("SRV"), err, len(s.calls))
		}
	}
}

// The action log names the storage host for the PUT, never its path or
// query, which can carry a signed blob key.
func TestUploadLogsOnlyTheStorageHost(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/1/rails/active_storage/direct_uploads" {
			jsonOK(w, map[string]any{"signed_id": "B", "attachable_sgid": "A",
				"direct_upload": map[string]any{"url": srvURL + "/disk/SECRETKEY/put?sig=SECRETSIG"}})
			return
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	logPath := filepath.Join(t.TempDir(), "log")
	f := &Fizzy{C: client.New(&config.Config{URL: srv.URL, Account: "1", Token: "T", LogPath: logPath}, false, io.Discard)}
	if _, err := f.Upload(smallFile(t)); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 2 {
		t.Fatalf("log %q", log)
	}
	host := strings.TrimPrefix(srvURL, "http://")
	if strings.Contains(string(log), "SECRET") || !strings.Contains(lines[1], `"method": "PUT", "path": "`+host+`"`) {
		t.Errorf("log %q", log)
	}
}
