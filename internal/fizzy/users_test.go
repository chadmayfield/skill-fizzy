package fizzy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUsersAndUser(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/1/users" {
			io.WriteString(w, "["+shapeUser+"]")
			return
		}
		io.WriteString(w, shapeUser)
	})
	users, err := f.Users()
	if err != nil || len(users) != 1 {
		t.Fatalf("users=%v err=%v", users, err)
	}
	u := users[0]
	if u.ID != "03abc0000000000000000000a1" || u.Name != "Alex" || u.Role != "owner" || !u.Active ||
		u.EmailAddress != "alex@example.com" || u.AvatarURL != "https://fizzy.example.com/a.png" ||
		u.URL != "https://fizzy.example.com/users/1" || u.CreatedAt != "2026-01-01T00:00:00.000Z" {
		t.Errorf("user = %+v", u)
	}
	if string(u.Raw()) != shapeUser {
		t.Errorf("raw = %s", u.Raw())
	}
	one, err := f.User("03abc0000000000000000000a1")
	if err != nil || one.Name != "Alex" {
		t.Fatalf("user=%+v err=%v", one, err)
	}
	if s.calls[1].Path != "/1/users/03abc0000000000000000000a1" {
		t.Errorf("path = %s", s.calls[1].Path)
	}
}

func TestRenameUser(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, shapeUser)
	})
	u, err := f.RenameUser("U1", "Gate Keeper")
	if err != nil || u.ID != "03abc0000000000000000000a1" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
	c := s.calls[0]
	if c.Method != "PUT" || c.Path != "/1/users/U1" || string(c.Body) != `{"user":{"name":"Gate Keeper"}}` {
		t.Errorf("call = %+v body %s", c, c.Body)
	}
}

func TestRenameUserDryRun(t *testing.T) {
	f, s, out, srv := newFizzy(t, true, func(w http.ResponseWriter, _ *http.Request, _ []byte) {})
	u, err := f.RenameUser("U1", "Gate Keeper")
	if err != nil || u.ID != "" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
	if len(s.calls) != 0 {
		t.Errorf("calls = %+v", s.calls)
	}
	want := "[dry-run] PUT " + srv.URL + `/1/users/U1 {"user": {"name": "Gate Keeper"}}` + "\n"
	if out.String() != want {
		t.Errorf("out = %q, want %q", out.String(), want)
	}
}

func TestMatchUserOrder(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, cardServer("{}"))
	for _, c := range []struct{ in, id, by string }{
		{"u2", "u2", "id"},
		{"Gate Keeper", "u1", "name"},
		{"gate keeper", "u1", "name"},
		{"hinge@fizzy.example.com", "u2", "email"},
		{"HINGE@fizzy.example.com", "u2", "email"},
		{"twin2@fizzy.example.com", "u4", "email"},
	} {
		m, err := f.MatchUser(c.in, false)
		if err != nil || m.ID != c.id || m.By != c.by {
			t.Errorf("MatchUser(%q) = %+v, %v; want id %s by %s", c.in, m, err, c.id, c.by)
		}
	}
	m, _ := f.MatchUser("gate keeper", false)
	if m.Name != "Gate Keeper" {
		t.Errorf("name = %q", m.Name)
	}
}

// An exact name beats a name that differs only in case.
func TestMatchUserExactNameFirst(t *testing.T) {
	users := []User{{ID: "u1", Name: "Hinge"}, {ID: "u2", Name: "hinge"}}
	m, err := matchUser(users, "hinge", false)
	if err != nil || m.ID != "u2" || m.By != "name" {
		t.Errorf("exact = %+v, %v", m, err)
	}
	if _, err := matchUser(users, "HINGE", false); !IsRefusal(err) || !strings.Contains(err.Error(), "2 users are named 'HINGE'") {
		t.Errorf("two case-insensitive matches: %v", err)
	}
}

func TestMatchUserAmbiguousRefuses(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, cardServer("{}"))
	for _, in := range []string{"Twin", "twin"} {
		_, err := f.MatchUser(in, false)
		if !IsRefusal(err) || !strings.Contains(err.Error(), "2 users are named '"+in+"'") {
			t.Errorf("%s: %v", in, err)
		}
	}
}

func TestMatchUserUnknown(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, cardServer("{}"))
	_, err := f.MatchUser("Nobody", false)
	if !IsRefusal(err) || err.Error() != "no user 'Nobody' (have: Gate Keeper, Hinge Smith, Twin, Twin)" {
		t.Errorf("unknown: %v", err)
	}
	// An id-like value that matches nobody is an error unless passthrough is on.
	const id = "03abc123def456ghi789jkl0mn"
	if _, err := f.MatchUser(id, false); err == nil {
		t.Error("id-like value matched without passthrough")
	}
	m, err := f.MatchUser(id, true)
	if err != nil || m.ID != id || m.By != "id" || m.Name != "" {
		t.Errorf("passthrough = %+v, %v", m, err)
	}
	if _, err := f.MatchUser("Nobody", true); err == nil {
		t.Error("a name passed through")
	}
}

func TestLooksLikeID(t *testing.T) {
	for s, want := range map[string]bool{
		"03abc123def456ghi789jkl0mn": true,
		"abcdefghijklmnopqrstuvwxyz": false,
		"03abc":                      false,
		"03ABC123DEF456GHI789JKL0MN": false,
	} {
		if got := looksLikeID(s); got != want {
			t.Errorf("looksLikeID(%q) = %v", s, got)
		}
	}
}
