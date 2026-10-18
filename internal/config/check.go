package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"
	"unicode"
)

// checkURL refuses a server URL fz should not send a token to. It requires
// https, except plain http to localhost, 127.0.0.0/8 or ::1, and it refuses
// credentials, a query, a fragment and an empty host. Errors never repeat the
// URL, because it may hold a secret.
func checkURL(raw string) error {
	bad := func(problem string) error {
		return fmt.Errorf("%w: the server URL %s; set it to https://HOST, with an optional path", ErrConfig, problem)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return bad("does not parse")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return bad("must start with https://")
	}
	if u.User != nil {
		return bad("must not hold a user name or password")
	}
	if u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return bad("has no host")
	}
	if u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "?") {
		return bad("must not have a query")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return bad("must not have a fragment")
	}
	if scheme == "http" && !isLoopback(u.Hostname()) {
		return bad("must use https (plain http is allowed only for localhost)")
	}
	return nil
}

// isLoopback reports whether host is localhost, an address in 127.0.0.0/8,
// or ::1.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// checkAccount refuses an account slug that could change the request path.
func checkAccount(account string) error {
	bad := func(problem string) error {
		return fmt.Errorf("%w: the account %s; set it to the account part of your Fizzy URLs", ErrConfig, problem)
	}
	switch {
	case account == "":
		return bad("is empty")
	case strings.IndexFunc(account, unicode.IsSpace) >= 0:
		return bad("must not contain spaces")
	case strings.ContainsAny(account, "/?#%"):
		return bad("must not contain /, ?, # or %")
	case strings.Contains(account, ".."):
		return bad("must not contain ..")
	}
	return nil
}

// currentUID returns the user fz runs as. A test replaces it.
var currentUID = os.Getuid

// checkFileInfo refuses a config file that people other than the user can
// write: one with the group or other write bit, or, on Unix, one owned by
// another user. The file names a command fz runs, so a writable file would
// let them run commands as the user. fi comes from the open file, so the
// file checked is the file read.
func checkFileInfo(path string, fi os.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: %s is writable by group or others, and it names a command fz runs; fix it with: chmod go-w %s", ErrConfig, path, path)
	}
	if uid, ok := fileUID(fi); ok && uid != currentUID() {
		return fmt.Errorf("%w: %s is owned by another user, and it names a command fz runs; use a config file of your own", ErrConfig, path)
	}
	return nil
}
