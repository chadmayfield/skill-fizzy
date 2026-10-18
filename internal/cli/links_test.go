package cli

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// linkServer records each request's path and its Authorization and Cookie
// headers.
type linkServer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
	auth  []string
}

func newLinkServer(t *testing.T, h http.HandlerFunc) *linkServer {
	t.Helper()
	s := &linkServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.auth = append(s.auth, r.Header.Get("Authorization")+r.Header.Get("Cookie"))
		s.mu.Unlock()
		if h != nil {
			h(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *linkServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

// allowLinkPorts lets link checks reach loopback only on the given ports,
// as if those servers were on the internet.
func allowLinkPorts(t *testing.T, servers ...*linkServer) {
	t.Helper()
	ports := map[uint16]bool{}
	for _, s := range servers {
		ports[netip.MustParseAddrPort(strings.TrimPrefix(s.URL, "http://")).Port()] = true
	}
	old := linkDialBlocked
	linkDialBlocked = func(a netip.AddrPort) bool { return !ports[a.Port()] && old(a) }
	t.Cleanup(func() { linkDialBlocked = old })
}

func linkDraft(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gate.md")
	writeFile(t, path, "---\nboard: Home\n---\n# Fix the gate\n\n"+body+"\n")
	return path
}

func TestLinksSkipLocalAddresses(t *testing.T) {
	srv := newSyncServer(t)
	local := newLinkServer(t, nil)
	path := linkDraft(t, "See "+local.URL+"/private and http://169.254.169.254/latest and http://[::1]:9/x and http://0.0.0.0:9/y and http://10.0.0.1:9/z")
	got := syncRun(t, srv, "draft", "check", path, "--links")
	if got.code != 0 {
		t.Errorf("exit %d: %s", got.code, got.stdout)
	}
	for _, u := range []string{local.URL + "/private", "http://169.254.169.254/latest", "http://[::1]:9/x", "http://0.0.0.0:9/y", "http://10.0.0.1:9/z"} {
		if !strings.Contains(got.stdout, "    skipped (local address) "+u+"\n") {
			t.Errorf("no skip line for %s:\n%s", u, got.stdout)
		}
	}
	if p := local.seen(); len(p) != 0 {
		t.Errorf("the local server got requests: %q", p)
	}
}

func TestLinksCheckEveryRedirectHop(t *testing.T) {
	srv := newSyncServer(t)
	inner := newLinkServer(t, nil)
	outer := newLinkServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL+"/private", http.StatusFound)
	})
	allowLinkPorts(t, outer)
	got := syncRun(t, srv, "draft", "check", linkDraft(t, "See "+outer.URL+"/go"), "--links")
	if !strings.Contains(got.stdout, "    skipped (local address) "+outer.URL+"/go\n") {
		t.Errorf("stdout:\n%s", got.stdout)
	}
	if p := inner.seen(); len(p) != 0 {
		t.Errorf("the redirect target got requests: %q", p)
	}
}

func TestLinksSkipCodeAndSendNoCredentials(t *testing.T) {
	srv := newSyncServer(t)
	site := newLinkServer(t, nil)
	allowLinkPorts(t, site)
	t.Setenv("FIZZY_TOKEN", "T")
	body := "See " + site.URL + "/ok.\n\nInline `" + site.URL + "/incode` here.\n\n```\n" + site.URL + "/fenced\n```\n"
	got := syncRun(t, srv, "draft", "check", linkDraft(t, body), "--links")
	if got.code != 0 || !strings.Contains(got.stdout, "    ok  200 "+site.URL+"/ok\n") {
		t.Errorf("code %d stdout:\n%s", got.code, got.stdout)
	}
	if p := site.seen(); len(p) != 1 || p[0] != "/ok" {
		t.Errorf("requests = %q, want only /ok", p)
	}
	for _, a := range site.auth {
		if a != "" {
			t.Errorf("a link check sent credentials: %q", a)
		}
	}
}

// The link check refuses every special-purpose and internal range, and
// allows ordinary public addresses.
func TestLinkDialBlockedRanges(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "100.100.100.100", "100.127.255.254", // shared address space (CGNAT, Tailscale)
		"0.1.2.3", "0.0.0.0", // this network
		"192.0.0.1", "192.0.0.170", // IETF protocol assignments
		"198.18.0.1", "198.19.255.254", // benchmarking
		"240.0.0.1", "255.255.255.255", // reserved and broadcast
		"224.0.0.1",
		"::1", "::", "fd00:ec2::254", "fe80::1", "ff02::1",
		"64:ff9b::a00:1", "64:ff9b::7f00:1", // NAT64 of 10.0.0.1 and 127.0.0.1
		"::a00:1", "::7f00:1", // IPv4-compatible
		"2002:a00:1::", "2002:7f00:1::", // 6to4 of 10.0.0.1 and 127.0.0.1
		"fec0::1",          // site-local
		"::ffff:10.0.0.1",  // IPv4-mapped
		"64:ff9b:1::a00:1", // local-use NAT64
		"2001:db8::1",      // documentation
		"100::1",           // discard-only
		"2001::1",          // Teredo
		"2001:10::1",       // ORCHID
		"198.51.100.1",     // documentation
		"203.0.113.1",      // documentation
		"192.0.2.1",        // documentation
		"192.88.99.1",      // 6to4 relay anycast
	}
	for _, s := range blocked {
		if !linkDialBlocked(netip.AddrPortFrom(netip.MustParseAddr(s), 443)) {
			t.Errorf("%s is not blocked", s)
		}
	}
	for _, s := range []string{"93.184.215.14", "8.8.8.8", "1.1.1.1", "2606:4700::1111", "2a00:1450:4001::1", "64:ff9b::808:808", "2002:808:808::"} {
		if linkDialBlocked(netip.AddrPortFrom(netip.MustParseAddr(s), 443)) {
			t.Errorf("%s is blocked", s)
		}
	}
}
