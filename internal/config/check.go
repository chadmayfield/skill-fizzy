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
func checkURL(raw string) error { return checkURLWith(raw, false) }

// checkURLWith is checkURL. When clusterHTTP is true it also allows plain
// http to a host that ends in .svc or .svc.cluster.local, the names of
// in-cluster services.
func checkURLWith(raw string, clusterHTTP bool) error {
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
	if scheme == "http" && !isLoopback(u.Hostname()) && !(clusterHTTP && isClusterService(u.Hostname())) {
		return bad("must use https (plain http is allowed only for localhost)")
	}
	return nil
}

// isClusterService reports whether host is an in-cluster service name: one
// or more DNS labels before a final .svc or .svc.cluster.local. A trailing
// dot is ignored.
func isClusterService(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	for _, suffix := range []string{".svc", ".svc.cluster.local"} {
		if name, ok := strings.CutSuffix(h, suffix); ok && name != "" {
			for _, label := range strings.Split(name, ".") {
				if !isDNSLabel(label) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// isDNSLabel reports whether s is 1 to 63 lowercase letters, digits or
// dashes, not starting or ending with a dash.
func isDNSLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
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
	case account == ".":
		return bad("must not be .")
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
