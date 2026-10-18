package fizzy

import (
	"io"
	"net/http"
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
