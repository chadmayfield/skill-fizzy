// Package config loads settings from a JSON file, with environment variables
// taking precedence.
//
// The default file is ~/.config/fizzy-skill/config.json, or $FIZZY_CONFIG.
// Environment variables: FIZZY_URL, FIZZY_ACCOUNT, FIZZY_TOKEN,
// FIZZY_TOKEN_CMD, FIZZY_INBOX, FIZZY_RULES_CARD.
//
// The tool never stores the token. Give it in FIZZY_TOKEN, or give a command
// that prints it (FIZZY_TOKEN_CMD or "token_cmd" in the file).
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// tokenCmdTimeout caps how long the token command may run.
var tokenCmdTimeout = 30 * time.Second

// ErrConfig is wrapped by every error this package returns.
var ErrConfig = errors.New("config error")

const (
	DefaultConfigPath = "~/.config/fizzy-skill/config.json"
	DefaultInbox      = "~/.fizzy-inbox"
	DefaultLog        = "~/.local/state/fizzy-skill/actions.log"
)

// Config holds the resolved settings. RulesCard is 0 when unset.
type Config struct {
	URL       string
	Account   string
	TokenCmd  string
	Token     string
	Inbox     string
	RulesCard int
	LogPath   string
}

// ExpandTilde replaces a leading ~ or ~/ with the home directory.
func ExpandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// InboxPath returns the inbox directory with ~ expanded.
func (c *Config) InboxPath() string { return ExpandTilde(c.Inbox) }

// LogFile returns the action log path with ~ expanded.
func (c *Config) LogFile() string { return ExpandTilde(c.LogPath) }

// ResolveToken returns the token from Token, or by running TokenCmd. The
// command gets tokenCmdTimeout to finish. A failure names the exit status, or
// "not found", and says to run the command by hand. Neither stdout nor stderr
// appears in an error, because a broken command may print the secret on
// either one.
func (c *Config) ResolveToken() (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	if c.TokenCmd == "" {
		return "", fmt.Errorf("%w: no token: set FIZZY_TOKEN or FIZZY_TOKEN_CMD (or token_cmd in the config file)", ErrConfig)
	}
	words, err := SplitWords(c.TokenCmd)
	if err != nil || len(words) == 0 {
		return "", fmt.Errorf("%w: token command failed (invalid command)", ErrConfig)
	}
	ctx, cancel := context.WithTimeout(context.Background(), tokenCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	// A child that keeps stdout open would hold Wait past the kill.
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var reason string
		var exitErr *exec.ExitError
		switch {
		case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
			reason = "not found"
		case ctx.Err() == context.DeadlineExceeded:
			reason = fmt.Sprintf("timed out after %s", tokenCmdTimeout)
		case errors.As(err, &exitErr):
			reason = exitErr.ProcessState.String()
		default:
			reason = "could not run"
		}
		return "", fmt.Errorf("%w: token command failed (%s); run the command by hand to see its error", ErrConfig, reason)
	}
	token := strings.TrimSpace(out.String())
	if token == "" {
		return "", fmt.Errorf("%w: token command printed nothing", ErrConfig)
	}
	return token, nil
}

// scalar renders a JSON string or number as text. Other types give "".
func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	}
	return ""
}

// Load reads the config file at path (or $FIZZY_CONFIG, or the default) and
// applies environment overrides. A missing file is not an error.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("FIZZY_CONFIG")
	}
	if path == "" {
		path = DefaultConfigPath
	}
	path = ExpandTilde(path)

	data := map[string]any{}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := checkFileMode(path); err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&data); err != nil {
			return nil, fmt.Errorf("%w: %s: invalid JSON (%v)", ErrConfig, path, err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, fmt.Errorf("%w: %s: invalid JSON (text after the JSON object)", ErrConfig, path)
		}
		if _, ok := data["token"]; ok {
			return nil, fmt.Errorf("%w: %s: the config file cannot hold the token; give a command that prints it in token_cmd, or set FIZZY_TOKEN", ErrConfig, path)
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%w: %s: %v", ErrConfig, path, err)
	}

	pick := func(env, key string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		return scalar(data[key])
	}
	url := pick("FIZZY_URL", "url")
	account := pick("FIZZY_ACCOUNT", "account")
	if url == "" || account == "" {
		return nil, fmt.Errorf("%w: set FIZZY_URL and FIZZY_ACCOUNT, or url/account in %s", ErrConfig, path)
	}
	if err := checkURL(url); err != nil {
		return nil, err
	}
	account = strings.Trim(account, "/")
	if err := checkAccount(account); err != nil {
		return nil, err
	}
	cfg := &Config{
		URL:      strings.TrimRight(url, "/"),
		Account:  account,
		TokenCmd: pick("FIZZY_TOKEN_CMD", "token_cmd"),
		Token:    os.Getenv("FIZZY_TOKEN"),
		Inbox:    pick("FIZZY_INBOX", "inbox"),
		LogPath:  DefaultLog,
	}
	if cfg.Inbox == "" {
		cfg.Inbox = DefaultInbox
	}
	if rules := pick("FIZZY_RULES_CARD", "rules_card"); rules != "" {
		n, err := strconv.Atoi(rules)
		if err != nil {
			return nil, fmt.Errorf("%w: rules_card %q is not a number", ErrConfig, rules)
		}
		cfg.RulesCard = n
	}
	return cfg, nil
}
