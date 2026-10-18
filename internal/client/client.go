// Package client is a thin HTTP client for the Fizzy API.
//
// Every request is scoped to the one configured account. Lists are followed
// through every page (Link: <...>; rel="next"). Create calls return the new
// object's id parsed from the Location header. In dry-run mode GETs are sent
// and writes are printed instead. Every write is appended to an action log
// holding the time, method, path and status, never the token.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// APIError is returned for any HTTP status of 300 or above.
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s %s -> HTTP %d", e.Method, e.Path, e.Status)
	detail := strings.TrimSpace(e.Body)
	if r := []rune(detail); len(r) > 300 {
		detail = string(r[:300])
	}
	if detail != "" {
		msg += ": " + detail
	}
	return msg
}

var linkNext = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="?next"?`)

// NextLink returns the rel="next" URL from a Link header, or "".
func NextLink(header string) string {
	if m := linkNext.FindStringSubmatch(header); m != nil {
		return m[1]
	}
	return ""
}

// IDFromLocation returns the last path segment of a Location header without a
// .json suffix, or "" when the header is empty.
func IDFromLocation(loc string) string {
	if loc == "" {
		return ""
	}
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	p := strings.TrimRight(u.Path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".json")
}

type reqOpts struct {
	raw           []byte
	headers       map[string]string
	noAuth        bool
	notAccountScp bool
}

// Option adjusts one request.
type Option func(*reqOpts)

// Raw sends these bytes as the body instead of JSON.
func Raw(b []byte) Option { return func(o *reqOpts) { o.raw = b } }

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

// NotAccountScoped leaves the account prefix off the path.
func NotAccountScoped() Option { return func(o *reqOpts) { o.notAccountScp = true } }

// Client talks to one Fizzy account.
type Client struct {
	cfg    *config.Config
	dryRun bool
	out    io.Writer
	http   *http.Client
	token  string
	hasTok bool
}

// New returns a client. Dry-run lines go to out.
func New(cfg *config.Config, dryRun bool, out io.Writer) *Client {
	return &Client{cfg: cfg, dryRun: dryRun, out: out, http: &http.Client{Timeout: 60 * time.Second, CheckRedirect: checkRedirect}}
}

// checkRedirect follows redirects the way Python's urllib does: GET follows
// 301, 302, 303 and 307; POST follows 301, 302 and 303 (as a GET). Anything
// else, including every PUT, PATCH and DELETE and any 308, returns the 3xx
// response, which Do reports as an *APIError.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return http.ErrUseLastResponse
	}
	code := req.Response.StatusCode
	switch via[len(via)-1].Method {
	case http.MethodGet, http.MethodHead:
		if code == 301 || code == 302 || code == 303 || code == 307 {
			return nil
		}
	case http.MethodPost:
		if code == 301 || code == 302 || code == 303 {
			return nil
		}
	}
	return http.ErrUseLastResponse
}

func (c *Client) auth() (string, error) {
	if !c.hasTok {
		t, err := c.cfg.ResolveToken()
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
			if r := []rune(s); len(r) > 400 {
				s = string(r[:400])
			}
			shown = s
		case len(o.raw) > 0:
			shown = fmt.Sprintf("<%d bytes>", len(o.raw))
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
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	reqPath := u
	if pu, perr := url.Parse(u); perr == nil {
		reqPath = pu.Path
	}
	if isWrite {
		c.logAction(method, reqPath, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, payload, &APIError{resp.StatusCode, method, reqPath, string(payload)}
	}
	return resp.StatusCode, resp.Header, payload, nil
}

func (c *Client) logAction(method, path string, status int) {
	p := c.cfg.LogFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
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
	return json.Unmarshal(payload, v)
}

// GetAll GETs a list endpoint and follows every page.
func (c *Client) GetAll(path string) ([]json.RawMessage, error) {
	var items []json.RawMessage
	next := c.URL(path, true)
	seen := map[string]bool{}
	for next != "" {
		if seen[next] {
			return nil, fmt.Errorf("pagination loop: %s linked again", next)
		}
		seen[next] = true
		_, hdr, payload, err := c.Do(http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			var page []json.RawMessage
			if err := json.Unmarshal(payload, &page); err != nil {
				return nil, err
			}
			items = append(items, page...)
		}
		next = NextLink(hdr.Get("Link"))
	}
	return items, nil
}

// Create POSTs body and returns the new object's id from the Location header.
// The id is empty in dry-run mode or when the server sends no Location.
func (c *Client) Create(path string, body any) (string, error) {
	status, hdr, _, err := c.Do(http.MethodPost, path, body)
	if err != nil || status == 0 {
		return "", err
	}
	return IDFromLocation(hdr.Get("Location")), nil
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
