package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeoutDefaults(t *testing.T) {
	if RequestTimeout != 60*time.Second || UploadTimeout != 10*time.Minute {
		t.Errorf("RequestTimeout = %v, UploadTimeout = %v", RequestTimeout, UploadTimeout)
	}
}

// A request with the Upload option gets UploadTimeout instead of
// RequestTimeout, so a large file on a slow link is not cut off at 60 s.
func TestUploadGetsTheLongerTimeout(t *testing.T) {
	oldReq, oldUp := RequestTimeout, UploadTimeout
	RequestTimeout, UploadTimeout = 100*time.Millisecond, 5*time.Second
	t.Cleanup(func() { RequestTimeout, UploadTimeout = oldReq, oldUp })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, _, _ := newTestClient(t, srv, false)
	if _, _, _, err := c.Do(http.MethodGet, "/boards", nil); err == nil {
		t.Error("a plain request outlived RequestTimeout")
	}
	if _, _, _, err := c.Do(http.MethodPut, srv.URL+"/storage", nil, Raw([]byte("hinge")), NoAuth(), NotAccountScoped(), Upload()); err != nil {
		t.Errorf("upload: %v", err)
	}
}
