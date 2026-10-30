package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// replyError is a reply that fz could not decode. Its text names the
// request and what was wrong in JSON terms, never a Go type.
type replyError struct {
	method, path, why string
}

func (e *replyError) Error() string {
	return fmt.Sprintf("%s %s: the reply was not what fz expected (%s)", e.method, e.path, e.why)
}

// ReplyError turns a JSON decode error for the reply to method and path
// into one that says which request it was and that the reply was not what
// fz expected. path is the request path as sent, such as /1/cards/87. A
// nil err stays nil, and an error already made by ReplyError is kept.
func ReplyError(method, path string, err error) error {
	if err == nil {
		return nil
	}
	var re *replyError
	if errors.As(err, &re) {
		return err
	}
	return &replyError{method, path, replyWhy(err)}
}

// RequestPath returns the path fz sends for path, with the configured
// URL's own path and, when accountScoped, the account in front.
func (c *Client) RequestPath(path string, accountScoped bool) string {
	u, err := url.Parse(c.URL(path, accountScoped))
	if err != nil {
		return path
	}
	return u.Path
}

// replyWhy says in JSON terms why a reply did not decode.
func replyWhy(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		got := jsonValueName(te.Value)
		want := jsonKindName(te.Type)
		if te.Field != "" {
			return fmt.Sprintf("%s where fz expected %s for %s", got, want, te.Field)
		}
		return fmt.Sprintf("%s where fz expected %s", got, want)
	}
	var se *json.SyntaxError
	if errors.As(err, &se) || strings.Contains(err.Error(), "unexpected end of JSON input") {
		return "it is not valid JSON"
	}
	return "it could not be read"
}

// jsonValueName names the JSON value an UnmarshalTypeError reports.
func jsonValueName(v string) string {
	switch {
	case v == "array":
		return "a list"
	case v == "object":
		return "an object"
	case v == "string":
		return "a string"
	case v == "bool":
		return "true or false"
	case v == "null":
		return "null"
	case strings.HasPrefix(v, "number"):
		return "a number"
	}
	return "a value"
}

// jsonKindName names the JSON value that decodes into t.
func jsonKindName(t reflect.Type) string {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "another value"
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	}
	return "another value"
}
