package cli

import (
	"net/http"
	"strings"
	"testing"
)

// An id of "." or ".." would change which endpoint a request names once a
// proxy or server removes dot segments. Every command that puts an id from
// the user into a path refuses one with exit 2 and sends no write.
func TestDotIDsRefuseEverywhere(t *testing.T) {
	cmds := [][]string{
		{"comment", "show", "87", "ID"},
		{"comment", "edit", "87", "ID", "new text"},
		{"comment", "delete", "87", "ID", "--yes"},
		{"comment", "react", "87", "ID", "ok"},
		{"comment", "reactions", "87", "ID"},
		{"comment", "unreact", "87", "ID", "r1"},
		{"comment", "unreact", "87", "c1", "ID"},
		{"card", "unboost", "87", "ID"},
		{"webhook", "show", "Home", "ID"},
		{"webhook", "edit", "Home", "ID", "--name", "Gate", "--yes"},
		{"webhook", "delete", "Home", "ID", "--yes"},
		{"webhook", "activate", "Home", "ID"},
		{"webhook", "deliveries", "Home", "ID"},
		{"notification", "read", "ID"},
		{"notification", "unread", "ID"},
		{"export", "status", "ID"},
		{"export", "status", "ID", "--mine"},
		{"step", "edit", "87", "ID", "new text"},
		{"step", "check", "87", "ID"},
		{"step", "uncheck", "87", "ID"},
		{"step", "delete", "87", "ID", "--yes"},
		{"user", "show", "ID"},
		{"user", "edit", "ID", "--name", "Alex", "--yes"},
	}
	_, calls := acctServer(t, map[string]string{
		"GET /1/boards":            `[{"id":"B1","name":"Home"}]`,
		"GET /1/users":             "[" + acctUser + "]",
		"GET /my/identity":         acctIdentity,
		"GET /1/cards/87":          `{"number":87,"title":"Gate"}`,
		"GET /1/cards/87/comments": "[]",
	})
	for _, bad := range []string{".", ".."} {
		for _, args := range cmds {
			args = append([]string(nil), args...)
			for i, a := range args {
				if a == "ID" {
					args[i] = bad
				}
			}
			before := len(calls())
			r := runGo(args)
			sent := calls()[before:]
			if r.code != 2 {
				t.Errorf("%v: exit %d, want 2 (stderr %q)", args, r.code, r.stderr)
			}
			for _, c := range sent {
				if c.method != http.MethodGet || strings.Contains(c.path, "/./") || strings.Contains(c.path, "/../") ||
					strings.HasSuffix(c.path, "/.") || strings.HasSuffix(c.path, "/..") {
					t.Errorf("%v: sent %s %s", args, c.method, c.path)
				}
			}
		}
	}
}
