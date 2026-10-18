package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	for _, k := range []string{"FIZZY_URL", "FIZZY_ACCOUNT", "FIZZY_TOKEN", "FIZZY_TOKEN_CMD", "FIZZY_INBOX", "FIZZY_RULES_CARD", "FIZZY_CONFIG"} {
		t.Setenv(k, "")
	}
}

func writeFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFileOnly(t *testing.T) {
	isolate(t)
	p := writeFile(t, `{"url":"https://fizzy.example.com/","account":"/1/"}`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://fizzy.example.com" || c.Account != "1" {
		t.Errorf("got URL=%q Account=%q", c.URL, c.Account)
	}
}

func TestLoadNumericAccountAndRulesCard(t *testing.T) {
	isolate(t)
	for _, body := range []string{
		`{"url":"https://fizzy.example.com","account":1,"rules_card":87}`,
		`{"url":"https://fizzy.example.com","account":"1","rules_card":"87"}`,
	} {
		c, err := Load(writeFile(t, body))
		if err != nil {
			t.Fatal(err)
		}
		if c.Account != "1" || c.RulesCard != 87 {
			t.Errorf("%s: got Account=%q RulesCard=%d", body, c.Account, c.RulesCard)
		}
	}
}

func TestEnvOverridesFile(t *testing.T) {
	isolate(t)
	p := writeFile(t, `{"url":"https://file.example.com","account":"1","token_cmd":"file-cmd","inbox":"/file/inbox","rules_card":1}`)
	t.Setenv("FIZZY_URL", "https://env.example.com/")
	t.Setenv("FIZZY_ACCOUNT", "2")
	t.Setenv("FIZZY_TOKEN_CMD", "env-cmd")
	t.Setenv("FIZZY_INBOX", "/env/inbox")
	t.Setenv("FIZZY_RULES_CARD", "87")
	t.Setenv("FIZZY_TOKEN", "tok")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://env.example.com" || c.Account != "2" || c.TokenCmd != "env-cmd" ||
		c.Inbox != "/env/inbox" || c.RulesCard != 87 || c.Token != "tok" {
		t.Errorf("env did not override: %+v", c)
	}
}

func TestFizzyConfigEnvPicksFile(t *testing.T) {
	isolate(t)
	t.Setenv("FIZZY_CONFIG", writeFile(t, `{"url":"https://fizzy.example.com","account":"7"}`))
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Account != "7" {
		t.Errorf("Account = %q", c.Account)
	}
}

func TestMissingURLOrAccount(t *testing.T) {
	isolate(t)
	_, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("want ErrConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "FIZZY_URL") || !strings.Contains(err.Error(), "FIZZY_ACCOUNT") {
		t.Errorf("error = %v", err)
	}
}

func TestInvalidJSON(t *testing.T) {
	isolate(t)
	p := writeFile(t, `{not json`)
	_, err := Load(p)
	if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), p) {
		t.Errorf("error = %v", err)
	}
}

func TestDefaultsInboxAndLog(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, err := Load(writeFile(t, `{"url":"https://fizzy.example.com","account":"1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Inbox != "~/.fizzy-inbox" || c.LogPath != "~/.local/state/fizzy-skill/actions.log" {
		t.Errorf("defaults: %q %q", c.Inbox, c.LogPath)
	}
	if got, want := c.InboxPath(), filepath.Join(home, ".fizzy-inbox"); got != want {
		t.Errorf("InboxPath = %q, want %q", got, want)
	}
	if got, want := c.LogFile(), filepath.Join(home, ".local/state/fizzy-skill/actions.log"); got != want {
		t.Errorf("LogFile = %q, want %q", got, want)
	}
}

func TestResolveTokenDirect(t *testing.T) {
	c := &Config{Token: "direct", TokenCmd: "false-no-such-command"}
	got, err := c.ResolveToken()
	if err != nil || got != "direct" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestResolveTokenCmd(t *testing.T) {
	c := &Config{TokenCmd: `printf ' abc \n'`}
	got, err := c.ResolveToken()
	if err != nil || got != "abc" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestNoTokenSource(t *testing.T) {
	if _, err := (&Config{}).ResolveToken(); !errors.Is(err, ErrConfig) {
		t.Errorf("err = %v", err)
	}
}

func TestTokenCmdFailureHidesOutput(t *testing.T) {
	c := &Config{TokenCmd: `sh -c 'echo SECRET; echo denied >&2; exit 1'`}
	_, err := c.ResolveToken()
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("err = %v", err)
	}
}

func TestTokenCmdEmpty(t *testing.T) {
	c := &Config{TokenCmd: `printf ''`}
	_, err := c.ResolveToken()
	if err == nil || !strings.Contains(err.Error(), "token command printed nothing") {
		t.Errorf("err = %v", err)
	}
}
