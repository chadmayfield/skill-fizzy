package cli

import (
	"strings"
	"testing"
)

// The --users preview names, by name and id, who keeps access, who gains it
// and who loses it.
func TestBoardAccessUsersPreviewNamesEveryone(t *testing.T) {
	srv, calls := bxServer(t, false)
	editEnv(t, srv)
	r := runGo([]string{"board", "access", "Home", "--users", "u1,u3"})
	want := "about to change access to board Home (b1)\n" +
		"  now: 2 users\n" +
		"  new: only these users: Pat (u1), Kim (u3)\n" +
		"  keeps access: Pat (u1)\n" +
		"  gains access: Kim (u3)\n" +
		"  loses access: Sam (u2)\n" +
		"  --users replaces the whole list; anyone not named loses access\n" +
		"refusing without --yes\n"
	if r.code != 2 || r.stderr != want {
		t.Fatalf("got %+v\nwant stderr %q", r, want)
	}
	if len(writes(calls())) != 0 {
		t.Fatal("the preview sent a write")
	}
}

// On a board every user can reach, everyone not named loses access.
func TestBoardAccessUsersPreviewFromAllAccess(t *testing.T) {
	srv, _ := bxServer(t, true)
	editEnv(t, srv)
	r := runGo([]string{"board", "access", "Home", "--users", "u1"})
	for _, line := range []string{
		"  now: all users\n",
		"  keeps access: Pat (u1)\n",
		"  gains access: none\n",
		"  loses access: Sam (u2), Kim (u3), pat (u4)\n",
	} {
		if r.code != 2 || !strings.Contains(r.stderr, line) {
			t.Errorf("missing %q in %+v", line, r)
		}
	}
}

// A board reply without user_ids does not say who has access now. The
// preview says so and still refuses without --yes.
func TestBoardAccessUsersPreviewUnknownList(t *testing.T) {
	_, calls := acctServer(t, map[string]string{
		"GET /1/boards":    `[{"id":"B1","name":"Home"}]`,
		"GET /1/boards/B1": `{"id":"B1","name":"Home","all_access":false}`,
		"GET /1/users":     "[" + acctUser + "," + acctUser2 + "]",
	})
	r := runGo([]string{"board", "access", "Home", "--users", "U1"})
	want := "about to change access to board Home (B1)\n" +
		"  now: selected users\n" +
		"  new: only these users: Alex (U1)\n" +
		"  the current list is unknown, so fz cannot say who keeps or loses access\n" +
		"  --users replaces the whole list; anyone not named loses access\n" +
		"refusing without --yes\n"
	if r.code != 2 || r.stderr != want {
		t.Fatalf("got %+v\nwant stderr %q", r, want)
	}
	for _, c := range calls() {
		if c.method != "GET" {
			t.Errorf("sent %s %s", c.method, c.path)
		}
	}
}
