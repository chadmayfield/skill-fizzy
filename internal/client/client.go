// Package client is a thin HTTP client for the Fizzy API.
//
// Every request is scoped to the one configured account. Lists are followed
// through every page (Link: <...>; rel="next"), up to MaxPages, and only to
// the configured server. Create calls return the new object's id parsed from
// the Location header. In dry-run mode GETs are sent and writes are printed
// instead. Every write that is sent is appended to an action log holding the
// time, method, path and status, never the token. A write that gets no
// response is logged with status 0.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// APIError is returned for any HTTP status of 300 or above.
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

// Error gives the method, path and status, plus up to 300 characters of the
// body with terminal escapes and control characters removed.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s %s -> HTTP %d", e.Method, e.Path, e.Status)
	if detail := config.SafeText(e.Body, 300); detail != "" {
		msg += ": " + detail
	}
	return msg
}

// NextLink returns the target of the first link in a Link header whose rel
// holds "next", or "". Parameters may come in any order, rel values are
// space-separated and compared without case, and "next-archive" is not
// "next". Parsing stops at the first malformed entry.
func NextLink(header string) string {
	s, i := header, 0
	skip := func(chars string) {
		for i < len(s) && strings.IndexByte(chars, s[i]) >= 0 {
			i++
		}
	}
	for {
		skip(" \t,")
		if i >= len(s) || s[i] != '<' {
			return ""
		}
		end := strings.IndexByte(s[i+1:], '>')
		if end < 0 {
			return ""
		}
		target := s[i+1 : i+1+end]
		i += end + 2
		var rel string
		haveRel := false
		for {
			skip(" \t")
			if i >= len(s) || s[i] != ';' {
				break
			}
			i++
			skip(" \t")
			start := i
			for i < len(s) && strings.IndexByte("=;, \t", s[i]) < 0 {
				i++
			}
			name := s[start:i]
			skip(" \t")
			var value string
			if i < len(s) && s[i] == '=' {
				i++
				skip(" \t")
				if i < len(s) && s[i] == '"' {
					var b strings.Builder
					i++
					for i < len(s) && s[i] != '"' {
						if s[i] == '\\' && i+1 < len(s) {
							i++
						}
						b.WriteByte(s[i])
						i++
					}
					if i >= len(s) {
						return ""
					}
					i++
					value = b.String()
				} else {
					start := i
					for i < len(s) && strings.IndexByte(";, \t", s[i]) < 0 {
						i++
					}
					value = s[start:i]
				}
			}
			if strings.EqualFold(name, "rel") && !haveRel {
				rel, haveRel = value, true
			}
		}
		for _, r := range strings.Fields(rel) {
			if strings.EqualFold(r, "next") {
				return target
			}
		}
		skip(" \t")
		if i < len(s) && s[i] != ',' {
			return ""
		}
	}
}

// sameOrigin reports whether a and b have the same scheme, host and port.
// A missing port counts as the scheme's default.
func sameOrigin(a, b *url.URL) bool {
	return a.Scheme != "" && strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// redactURL keeps the scheme, host and path of raw and drops the rest, so a
// signed query never reaches an error message or a log.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparsable URL)"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// redactErr removes the query from the URL in a *url.Error.
func redactErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = redactURL(ue.URL)
	}
	return err
}

// redactErrHost names only the storage origin and a generic error.
// Nested diagnostics can echo signing keys from redirect headers.
func redactErrHost(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		shown := "(unparsable URL)"
		if u, perr := url.Parse(ue.URL); perr == nil {
			shown = (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
		}
		return &url.Error{Op: ue.Op, URL: shown, Err: storageRequestError{ue.Err}}
	}
	return storageRequestError{err}
}

type storageRequestError struct{ err error }

// Error hides storage diagnostics that may carry signing details.
func (e storageRequestError) Error() string {
	if e.Timeout() {
		return "storage request timed out"
	}
	if errors.Is(e.err, context.Canceled) {
		return "storage request canceled"
	}
	return "storage request failed"
}

// Unwrap preserves the underlying cause for errors.Is and errors.As.
func (e storageRequestError) Unwrap() error { return e.err }

// Timeout reports whether the underlying storage failure timed out.
func (e storageRequestError) Timeout() bool {
	var timeout interface{ Timeout() bool }
	return errors.As(e.err, &timeout) && timeout.Timeout()
}

// ErrUnsafeID is the error IDFromLocation returns for an id that holds
// whitespace, a control character or a format character. Such an id could
// split a printed line or a URL path, so fz never uses it.
var ErrUnsafeID = errors.New("the id in its Location header holds whitespace or a control character, so fz does not use it")

// IDFromLocation returns the last path segment of a Location header without a
// .json suffix, or "" when the header is empty. An id that holds whitespace,
// a control character or a format character is ErrUnsafeID.
func IDFromLocation(loc string) (string, error) {
	if loc == "" {
		return "", nil
	}
	u, err := url.Parse(loc)
	if err != nil {
		return "", nil
	}
	p := strings.TrimRight(u.Path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	id := strings.TrimSuffix(p, ".json")
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return "", ErrUnsafeID
		}
	}
	return id, nil
}

type reqOpts struct {
	raw           []byte
	stream        io.Reader
	streamSize    int64
	logHost       bool
	headers       map[string]string
	noAuth        bool
	notAccountScp bool
	upload        bool
	deadline      time.Time
}

// Option adjusts one request.
type Option func(*reqOpts)

// Raw sends these bytes as the body instead of JSON.
func Raw(b []byte) Option { return func(o *reqOpts) { o.raw = b } }

// Stream sends size bytes read from r as the body instead of JSON, without
// holding them in memory.
func Stream(r io.Reader, size int64) Option {
	return func(o *reqOpts) { o.stream, o.streamSize = r, size }
}

// LogHost writes the request's host to the action log instead of its path,
// for a URL whose path or query carries a secret, such as a signed storage
// key.
func LogHost() Option { return func(o *reqOpts) { o.logHost = true } }

// Headers adds request headers, overriding the defaults.
func Headers(h map[string]string) Option {
	return func(o *reqOpts) {
		if o.headers == nil {
			o.headers = map[string]string{}
		}
		for k, v := range h {
			o.headers[k] = v
		}
	}
}

// NoAuth omits the Authorization header.
func NoAuth() Option { return func(o *reqOpts) { o.noAuth = true } }

// deadline ends the request at t, if that comes before its own timeout.
func deadline(t time.Time) Option { return func(o *reqOpts) { o.deadline = t } }

// NotAccountScoped leaves the account prefix off the path.
func NotAccountScoped() Option { return func(o *reqOpts) { o.notAccountScp = true } }

// Upload gives the request UploadTimeout instead of RequestTimeout, for
// sending a file's bytes.
func Upload() Option { return func(o *reqOpts) { o.upload = true } }

// RequestTimeout caps one request, reply body included. Tests may lower it.
var RequestTimeout = 60 * time.Second

// UploadTimeout caps a request sent with the Upload option, so a large file
// on a slow link has time to arrive. Tests may lower it.
var UploadTimeout = 10 * time.Minute

// Client talks to one Fizzy account.
type Client struct {
	cfg    *config.Config
	dryRun bool
	out    io.Writer
	http   *http.Client
	origin *url.URL // the configured URL; nil when it does not parse

	mu       sync.Mutex // guards token, hasTok and tokenCtx
	token    string
	hasTok   bool
	tokenCtx context.Context // nil unless SetTokenContext was called
}

// SetTokenContext ties the token command to ctx: when ctx ends, the command
// and every process it started are killed. See config.ResolveTokenContext.
// Without it the command gets its usual time limit only.
func (c *Client) SetTokenContext(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenCtx = ctx
}

// New returns a client. Dry-run lines go to out.
func New(cfg *config.Config, dryRun bool, out io.Writer) *Client {
	origin, err := url.Parse(cfg.URL)
	if err != nil {
		origin = nil
	}
	return &Client{cfg: cfg, dryRun: dryRun, out: out, origin: origin,
		http: &http.Client{CheckRedirect: checkRedirect}}
}

// MaxBody caps the size of one response body.
const MaxBody = 32 << 20

// errDowngrade is returned for a redirect from https to plain http.
var errDowngrade = errors.New("the server redirected from https to http, which fz never follows")

// checkRedirect follows a redirect of a GET or HEAD for 301, 302, 303 and
// 307. Every other redirect, including every redirect of a write and any 308,
// returns the 3xx response, which Do reports as an *APIError. A redirect from
// https to http is an error and is never sent. A redirect to another scheme,
// host or port than the first request's goes without the Authorization
// header.
func checkRedirect(req *http.Request, via []*http.Request) error {
	for _, v := range via {
		if strings.EqualFold(v.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
			return errDowngrade
		}
	}
	if !sameOrigin(req.URL, via[0].URL) {
		req.Header.Del("Authorization")
	}
	if len(via) >= 10 {
		return http.ErrUseLastResponse
	}
	code := req.Response.StatusCode
	switch via[len(via)-1].Method {
	case http.MethodGet, http.MethodHead:
		if code == 301 || code == 302 || code == 303 || code == 307 {
			return nil
		}
	}
	return http.ErrUseLastResponse
}

func (c *Client) auth() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasTok {
		resolve := c.cfg.ResolveToken
		if c.tokenCtx != nil {
			ctx := c.tokenCtx
			resolve = func() (string, error) { return c.cfg.ResolveTokenContext(ctx) }
		}
		t, err := resolve()
		if err != nil {
			return "", err
		}
		c.token, c.hasTok = t, true
	}
	return c.token, nil
}

// URL returns the full URL for path. Absolute URLs pass through unchanged.
func (c *Client) URL(path string, accountScoped bool) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	prefix := ""
	if accountScoped {
		prefix = "/" + c.cfg.Account
	}
	return c.cfg.URL + prefix + path
}

// Account returns the configured account, without slashes.
func (c *Client) Account() string { return c.cfg.Account }

// Do sends one request. In dry-run mode a non-GET request is printed, not
// sent, and Do returns status 0 with no payload. A status of 300 or above
// returns an *APIError along with the status, header and payload.
func (c *Client) Do(method, path string, body any, opts ...Option) (int, http.Header, []byte, error) {
	var o reqOpts
	for _, opt := range opts {
		opt(&o)
	}
	u := c.URL(path, !o.notAccountScp)
	isWrite := method != http.MethodGet
	if isWrite && c.dryRun {
		shown := ""
		switch {
		case body != nil:
			s, err := pyDumps(body)
			if err != nil {
				return 0, nil, nil, err
			}
			// Long bodies are cut so the line stays readable; the marker says
			// how much was left out, so a cut body never looks complete.
			if r := []rune(s); len(r) > 400 {
				s = fmt.Sprintf("%s ... (+%d more characters not shown)", string(r[:400]), len(r)-400)
			}
			shown = s
		case len(o.raw) > 0:
			shown = fmt.Sprintf("<%d bytes>", len(o.raw))
		case o.stream != nil:
			shown = fmt.Sprintf("<%d bytes>", o.streamSize)
		}
		fmt.Fprintf(c.out, "[dry-run] %s %s %s\n", method, u, shown)
		return 0, nil, nil, nil
	}

	data := o.raw
	hdrs := map[string]string{"Accept": "application/json", "User-Agent": "fizzy-skill"}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
		data = b
		hdrs["Content-Type"] = "application/json"
	}
	for k, v := range o.headers {
		hdrs[k] = v
	}
	if !o.noAuth && strings.Contains(path, "://") {
		if pu, err := url.Parse(u); err != nil || c.origin == nil || !sameOrigin(pu, c.origin) {
			return 0, nil, nil, fmt.Errorf("refusing to send the token to %s, which is not the configured server", redactURL(u))
		}
	}
	if !o.noAuth {
		t, err := c.auth()
		if err != nil {
			return 0, nil, nil, err
		}
		hdrs["Authorization"] = "Bearer " + t
	}
	var rd io.Reader
	if data != nil {
		rd = bytes.NewReader(data)
	}
	if o.stream != nil && body == nil {
		rd = io.LimitReader(o.stream, o.streamSize)
	}
	timeout := RequestTimeout
	if o.upload {
		timeout = UploadTimeout
	}
	// The deadline covers the reply body too, which is read before Do
	// returns.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if !o.deadline.IsZero() {
		var cancelAt context.CancelFunc
		ctx, cancelAt = context.WithDeadline(ctx, o.deadline)
		defer cancelAt()
	}
	redact := redactErr
	if o.logHost {
		redact = redactErrHost
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, nil, redact(err)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	if o.stream != nil && body == nil {
		req.ContentLength = o.streamSize
	}
	// logPath names the request in the action log and in errors.
	logPath := req.URL.Path
	if o.logHost {
		logPath = req.URL.Host
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if isWrite {
			c.logAction(method, logPath, 0)
		}
		return 0, nil, nil, redact(err)
	}
	defer resp.Body.Close()
	if isWrite {
		c.logAction(method, logPath, resp.StatusCode)
	}
	if isWrite && (resp.StatusCode == 301 || resp.StatusCode == 302 || resp.StatusCode == 303) {
		return resp.StatusCode, resp.Header, nil, &APIError{resp.StatusCode, method, logPath,
			"the server redirected a write, and fz does not follow that; the configured URL may need https"}
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, redact(err)
	}
	if len(payload) > MaxBody {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("%s %s: the response is larger than 32 MiB", method, logPath)
	}
	if resp.StatusCode >= 300 {
		// A storage service's error reply can echo signing details, so for
		// a request logged by host only the status is kept.
		if o.logHost {
			return resp.StatusCode, resp.Header, nil, &APIError{resp.StatusCode, method, logPath, ""}
		}
		return resp.StatusCode, resp.Header, payload, &APIError{resp.StatusCode, method, logPath, string(payload)}
	}
	return resp.StatusCode, resp.Header, payload, nil
}

func (c *Client) logAction(method, path string, status int) {
	p := c.cfg.LogFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	// A log made by an older fz may be readable by others.
	_ = f.Chmod(0o600)
	line, _ := pyDumps(struct {
		T      string `json:"t"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}{time.Now().UTC().Format("2006-01-02T15:04:05Z"), method, path, status})
	fmt.Fprintln(f, line)
}

// Get sends a GET and decodes the JSON payload into v. An empty payload
// leaves v untouched; a nil v discards the payload.
func (c *Client) Get(path string, v any) error {
	_, _, payload, err := c.Do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if v == nil || len(payload) == 0 {
		return nil
	}
	return ReplyError(http.MethodGet, c.RequestPath(path, true), json.Unmarshal(payload, v))
}

// GetAll GETs a list endpoint and follows every page. On any error it
// returns nil, never a partial list.
func (c *Client) GetAll(path string) ([]json.RawMessage, error) {
	var items []json.RawMessage
	err := c.EachPage(path, func(payload []byte) (bool, error) {
		var page []json.RawMessage
		if err := json.Unmarshal(payload, &page); err != nil {
			return false, ReplyError(http.MethodGet, c.RequestPath(path, true), err)
		}
		items = append(items, page...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// ListTimeout caps one list read, all its pages together. It is a variable
// so a test can lower it.
var ListTimeout = 10 * time.Minute

// MaxPages caps how many pages one list may follow.
const MaxPages = 1000

// maxListBytes caps the bytes of all the pages of one list together, so a
// server cannot fill memory with 1,000 pages of MaxBody each. It is a
// variable so a test can lower it.
var maxListBytes = 256 << 20

// EachPage GETs path and each rel="next" page after it, passing every
// non-empty payload to fn; fn returns true to stop. Next links must stay on
// the configured origin, so the token never leaves it. A repeated link, more
// than MaxPages pages, 256 MiB in total or ListTimeout is an error.
func (c *Client) EachPage(path string, fn func(payload []byte) (stop bool, err error)) error {
	next := c.URL(path, true)
	seen := map[string]bool{}
	total := 0
	end := time.Now().Add(ListTimeout)
	tooSlow := fmt.Errorf("pagination: the list took more than %v; fz stopped", ListTimeout)
	for pages := 0; next != ""; pages++ {
		if !time.Now().Before(end) {
			return tooSlow
		}
		if pages == MaxPages {
			return fmt.Errorf("pagination: more than %d pages; fz stopped", MaxPages)
		}
		if seen[next] {
			return fmt.Errorf("pagination loop: %s linked again", redactURL(next))
		}
		seen[next] = true
		_, hdr, payload, err := c.Do(http.MethodGet, next, nil, deadline(end))
		if err != nil {
			if !time.Now().Before(end) {
				return tooSlow
			}
			return err
		}
		if total += len(payload); total > maxListBytes {
			return fmt.Errorf("pagination: the list is over %d MiB; fz stopped", maxListBytes>>20)
		}
		if len(bytes.TrimSpace(payload)) > 0 {
			stop, err := fn(payload)
			if err != nil || stop {
				return err
			}
		}
		link := NextLink(strings.Join(hdr.Values("Link"), ", "))
		if link == "" {
			return nil
		}
		if next, err = c.resolveNext(link); err != nil {
			return err
		}
	}
	return nil
}

// resolveNext turns a next link into the absolute URL to fetch, or refuses it.
func (c *Client) resolveNext(link string) (string, error) {
	if c.origin == nil {
		return "", errors.New("pagination: the configured URL does not parse")
	}
	if strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//") {
		return c.origin.Scheme + "://" + c.origin.Host + link, nil
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("pagination: the server sent a next link fz cannot follow")
	}
	if !sameOrigin(u, c.origin) {
		return "", fmt.Errorf("pagination: the next link points to %s://%s, not the configured server; fz did not follow it", u.Scheme, u.Host)
	}
	return link, nil
}

// Create POSTs body and returns the new object's id from the Location header.
// The id is empty in dry-run mode or when the server sends no Location. An
// id IDFromLocation refuses is ErrUnsafeID.
func (c *Client) Create(path string, body any) (string, error) {
	status, hdr, _, err := c.Do(http.MethodPost, path, body)
	if err != nil || status == 0 {
		return "", err
	}
	return IDFromLocation(hdr.Get("Location"))
}

// Write sends a non-GET request and returns the JSON payload, or nil when
// the payload is empty or not valid JSON.
func (c *Client) Write(method, path string, body any) (json.RawMessage, error) {
	_, _, payload, err := c.Do(method, path, body)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || !json.Valid(payload) {
		return nil, nil
	}
	return json.RawMessage(payload), nil
}

// pyDumps marshals v the way Python's json.dumps does by default: ", " and
// ": " separators, non-ASCII escaped as \uXXXX. Key order follows Go's
// marshaling (struct field order, sorted map keys).
func pyDumps(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	dec := json.NewDecoder(&buf)
	dec.UseNumber()
	var out strings.Builder
	type frame struct {
		obj bool
		n   int // tokens seen in this container; keys and values both count
	}
	var stack []*frame
	// before writes the separator an element needs and reports whether the
	// token is an object key.
	before := func() bool {
		if len(stack) == 0 {
			return false
		}
		f := stack[len(stack)-1]
		isKey := f.obj && f.n%2 == 0
		if f.n > 0 && (!f.obj || isKey) {
			out.WriteString(", ")
		}
		return isKey
	}
	after := func(isKey bool) {
		if len(stack) == 0 {
			return
		}
		if isKey {
			out.WriteString(": ")
		}
		stack[len(stack)-1].n++
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			out.WriteRune(rune(d))
			stack = stack[:len(stack)-1]
			after(false)
			continue
		}
		isKey := before()
		switch t := tok.(type) {
		case json.Delim:
			out.WriteRune(rune(t))
			stack = append(stack, &frame{obj: t == '{'})
			continue
		case string:
			out.WriteString(pyString(t))
		case json.Number:
			out.WriteString(t.String())
		case bool:
			fmt.Fprint(&out, t)
		case nil:
			out.WriteString("null")
		}
		after(isKey)
	}
	return out.String(), nil
}

func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (r >= 0x7f && r < 0x10000):
			fmt.Fprintf(&b, `\u%04x`, r)
		case r >= 0x10000:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, r1, r2)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
