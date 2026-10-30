package fizzy

import (
	"os"
	"strings"
	"testing"
)

// sizedInfo is a FileInfo with another size.
type sizedInfo struct {
	os.FileInfo
	size int64
}

func (s sizedInfo) Size() int64 { return s.size }

// A file whose size changes between the stat and the read is refused, and
// nothing is sent.
func TestUploadFileChangedSize(t *testing.T) {
	old := statUpload
	t.Cleanup(func() { statUpload = old })
	statUpload = func(p string) (os.FileInfo, error) {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		return sizedInfo{fi, fi.Size() + 1}, nil
	}
	f, s := uploadFizzy(t, false, func(u string) string { return u + "/put" })
	_, err := f.Upload(smallFile(t))
	if err == nil || !strings.Contains(err.Error(), "changed size while fz read it") || len(s.calls) != 0 {
		t.Errorf("err %v, %d requests", err, len(s.calls))
	}
}

// Plain http is allowed for localhost, 127.0.0.1 and ::1 only, not for a
// host near them.
func TestWebhookHTTPOnlyForLoopbackNames(t *testing.T) {
	for _, u := range []string{"http://localhost/h", "http://127.0.0.1:8080/h", "http://[::1]/h"} {
		if err := CheckWebhookURL(u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	for _, u := range []string{
		"http://localhost.example.com/h", "http://127.0.0.2/h", "http://[::2]/h", "http://0.0.0.0/h",
		"http://localhost6/h", "http://127.1/h", "http://ip6-localhost/h", "http://hooks.example.com/h",
	} {
		if err := CheckWebhookURL(u); !IsRefusal(err) {
			t.Errorf("%s: %v", u, err)
		}
	}
}
