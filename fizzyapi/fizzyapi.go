// Package fizzyapi builds a Fizzy API client for use from other Go programs.
//
// The client it returns is the one fz uses, with the same safety checks: the
// token goes only to the configured server, list pages and redirects must
// stay on that server, and writes are never redirected. It reads no config
// file and no FIZZY_* variable, writes no action log, and is never in dry-run
// mode.
package fizzyapi

import (
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/chadmayfield/skill-fizzy/client"
	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// Options configures New.
type Options struct {
	// BaseURL is the server URL, such as https://fizzy.example.com. It must
	// use https, except plain http to localhost, 127.0.0.0/8 or ::1, or to an
	// in-cluster service when AllowPlainHTTP is set. It must hold no user
	// name, password, query or fragment.
	BaseURL string
	// AllowPlainHTTP lets BaseURL use plain http to an in-cluster service,
	// a host that ends in .svc or .svc.cluster.local, on any port. It is for
	// a program that runs in the same Kubernetes cluster as Fizzy and talks
	// to its service directly. Any other plain http host is still refused.
	// The token then travels unencrypted inside the cluster network.
	AllowPlainHTTP bool
	// Account is the account part of the server's URLs. Slashes around it
	// are dropped.
	Account string
	// Token is the API access token, sent as a bearer token.
	Token string
	// HTTPClient, when set, carries the requests. Its transport, jar and
	// timeout are kept; its redirect policy is replaced with the client's
	// own. When nil, a client on http.DefaultTransport is used.
	HTTPClient *http.Client
	// Limiter is waited on before every HTTP request, including each page
	// of a list and each redirect followed. It is required: New refuses a
	// nil Limiter, including a nil pointer of a type that satisfies it.
	// *rate.Limiter from golang.org/x/time/rate satisfies it. A redirect's
	// wait counts against that request's time limit.
	Limiter client.Limiter
}

// New checks opts and returns a client for one Fizzy account. It sends no
// request.
func New(opts Options) (*fizzy.Fizzy, error) {
	cfg, err := config.FromValues(opts.BaseURL, opts.Account, opts.Token, opts.AllowPlainHTTP)
	if err != nil {
		return nil, err
	}
	if isNil(opts.Limiter) {
		return nil, errors.New("fizzyapi: Options.Limiter is nil")
	}
	c := client.New(cfg, false, io.Discard)
	if opts.HTTPClient != nil {
		c.UseHTTPClient(opts.HTTPClient)
	}
	c.UseLimiter(opts.Limiter)
	return &fizzy.Fizzy{C: c}, nil
}

// isNil reports whether l is a nil interface, or holds a nil pointer, map,
// slice, func, channel or interface, on which Wait would panic.
func isNil(l client.Limiter) bool {
	if l == nil {
		return true
	}
	v := reflect.ValueOf(l)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
}
