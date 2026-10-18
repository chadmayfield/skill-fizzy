package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// loadEnv loads with only FIZZY_URL and FIZZY_ACCOUNT set and no file.
func loadEnv(t *testing.T, url, account string) (*Config, error) {
	t.Helper()
	isolate(t)
	t.Setenv("FIZZY_URL", url)
	t.Setenv("FIZZY_ACCOUNT", account)
	return Load(filepath.Join(t.TempDir(), "absent.json"))
}

func TestURLAccepted(t *testing.T) {
	cases := map[string]string{
		"https://fizzy.example.com":        "https://fizzy.example.com",
		"https://fizzy.example.com/":       "https://fizzy.example.com",
		"https://fizzy.example.com/fizzy/": "https://fizzy.example.com/fizzy",
		"HTTPS://fizzy.example.com":        "HTTPS://fizzy.example.com",
		"https://fizzy.example.com:8443":   "https://fizzy.example.com:8443",
		"http://localhost:3000":            "http://localhost:3000",
		"http://127.0.0.1:8080/":           "http://127.0.0.1:8080",
		"http://127.5.6.7":                 "http://127.5.6.7",
		"http://[::1]:3000":                "http://[::1]:3000",
	}
	for in, want := range cases {
		c, err := loadEnv(t, in, "1")
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if c.URL != want {
			t.Errorf("%q: URL = %q, want %q", in, c.URL, want)
		}
	}
}

func TestURLRejected(t *testing.T) {
	cases := map[string]string{
		"http://fizzy.example.com":                    "https",
		"fizzy.example.com":                           "https",
		"ftp://fizzy.example.com":                     "https",
		"https://":                                    "host",
		"https:///1":                                  "host",
		"https:fizzy.example.com":                     "host",
		"https://gate:SECRETTOKEN@fizzy.example.com":  "user",
		"https://SECRETTOKEN@fizzy.example.com":       "user",
		"https://fizzy.example.com/?a=b":              "query",
		"https://fizzy.example.com/?":                 "query",
		"https://fizzy.example.com/#hinge":            "fragment",
		"https://fizzy.example.com#":                  "fragment",
		"http://localhost.example.com":                "https",
		"http://127.0.0.1.example.com":                "https",
		"http://[::2]:3000":                           "https",
		"http://0.0.0.0:3000":                         "https",
		"https://fizzy.example.com/x?token=SECRETTOK": "query",
	}
	for in, word := range cases {
		_, err := loadEnv(t, in, "1")
		if err == nil {
			t.Errorf("%q: accepted", in)
			continue
		}
		if !errors.Is(err, ErrConfig) {
			t.Errorf("%q: not a config error: %v", in, err)
		}
		if !strings.Contains(err.Error(), word) {
			t.Errorf("%q: error %q does not mention %q", in, err, word)
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%q: error echoes the secret: %q", in, err)
		}
	}
}

func TestAccountSlug(t *testing.T) {
	for _, ok := range []string{"1", "/1/", "897362094", "gate-house"} {
		if _, err := loadEnv(t, "https://fizzy.example.com", ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"/", " 1 ", "1 2", "1\t", "1/2", "1?x=", "1#x", "1%2F", "1/../../admin", "..", "a..b"} {
		_, err := loadEnv(t, "https://fizzy.example.com", bad)
		if err == nil {
			t.Errorf("%q accepted", bad)
			continue
		}
		if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "account") {
			t.Errorf("%q: error = %v", bad, err)
		}
	}
}

func TestConfigFileWritableByOthers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are Unix only")
	}
	isolate(t)
	for _, mode := range []os.FileMode{0o620, 0o602, 0o666} {
		p := writeFile(t, `{"url":"https://fizzy.example.com","account":"1"}`)
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "writable") || !strings.Contains(err.Error(), "chmod") {
			t.Errorf("mode %o: error = %v", mode, err)
		}
	}
	p := writeFile(t, `{"url":"https://fizzy.example.com","account":"1"}`)
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Errorf("mode 644 refused: %v", err)
	}
}

func TestConfigTrailingData(t *testing.T) {
	isolate(t)
	for _, body := range []string{
		`{"url":"https://fizzy.example.com","account":"1"} {"account":"2"}`,
		`{"url":"https://fizzy.example.com","account":"1"} hinge`,
		`{"url":"https://fizzy.example.com","account":"1"}}`,
	} {
		_, err := Load(writeFile(t, body))
		if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "after the JSON object") {
			t.Errorf("%s: error = %v", body, err)
		}
	}
	if _, err := Load(writeFile(t, "{\"url\":\"https://fizzy.example.com\",\"account\":\"1\"}\n\n")); err != nil {
		t.Errorf("trailing newline refused: %v", err)
	}
}

func TestConfigTokenKey(t *testing.T) {
	isolate(t)
	_, err := Load(writeFile(t, `{"url":"https://fizzy.example.com","account":"1","token":"SECRETTOKEN"}`))
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("want a config error, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "token_cmd") || !strings.Contains(msg, "FIZZY_TOKEN") || strings.Contains(msg, "SECRET") {
		t.Errorf("error = %q", msg)
	}
}

func TestTokenCmdFailureReason(t *testing.T) {
	c := &Config{TokenCmd: `sh -c 'echo SECRET; echo keychain locked >&2; exit 3'`}
	_, err := c.ResolveToken()
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "SECRET") {
		t.Errorf("stdout leaked: %q", msg)
	}
	if !strings.Contains(msg, "exit status 3") || !strings.Contains(msg, "run the command by hand") {
		t.Errorf("error = %q", msg)
	}
}

func TestTokenCmdNotFound(t *testing.T) {
	c := &Config{TokenCmd: `fz-no-such-command-hinge --flag`}
	_, err := c.ResolveToken()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v", err)
	}
}

// A broken token command may print the token on stderr, so fz never shows it.
func TestTokenCmdHidesStderr(t *testing.T) {
	c := &Config{TokenCmd: `sh -c 'echo hinge-token-on-stderr >&2; exit 1'`}
	_, err := c.ResolveToken()
	if err == nil {
		t.Fatal("want an error")
	}
	if msg := err.Error(); strings.Contains(msg, "hinge-token-on-stderr") {
		t.Errorf("stderr shown: %q", msg)
	}
}

func TestTokenCmdTimeout(t *testing.T) {
	old := tokenCmdTimeout
	tokenCmdTimeout = 200 * time.Millisecond
	t.Cleanup(func() { tokenCmdTimeout = old })
	c := &Config{TokenCmd: `sh -c 'sleep 10; echo late'`}
	start := time.Now()
	_, err := c.ResolveToken()
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestTokenCmdTimeoutIs30s(t *testing.T) {
	if tokenCmdTimeout != 30*time.Second {
		t.Errorf("tokenCmdTimeout = %v", tokenCmdTimeout)
	}
}
