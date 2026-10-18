package fizzy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// User is a person on the account.
type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Active       bool   `json:"active"`
	EmailAddress string `json:"email_address"`
	CreatedAt    string `json:"created_at"`
	URL          string `json:"url"`
	AvatarURL    string `json:"avatar_url"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this user.
func (u User) Raw() json.RawMessage { return u.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (u *User) UnmarshalJSON(data []byte) error {
	type plain User
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*u = User(p)
	u.raw = append(json.RawMessage(nil), data...)
	return nil
}

// Users lists the account's active users.
func (f *Fizzy) Users() ([]User, error) { return list[User](f.C, "/users") }

// User fetches one user by id.
func (f *Fizzy) User(id string) (User, error) {
	if err := CheckIDs(id); err != nil {
		return User{}, err
	}
	var u User
	err := f.C.Get("/users/"+url.PathEscape(id), &u)
	return u, err
}

// RenameUser changes a user's display name and returns the updated user,
// which is empty in dry-run mode.
func (f *Fizzy) RenameUser(id, name string) (User, error) {
	if err := CheckIDs(id); err != nil {
		return User{}, err
	}
	path := "/users/" + url.PathEscape(id)
	raw, err := f.C.Write(http.MethodPut, path, wrap("user", struct {
		Name string `json:"name"`
	}{name}))
	var u User
	err = decodeWrite(f.C, http.MethodPut, path, raw, err, &u)
	return u, err
}

// UserMatch is the user a USER argument resolved to. By says what matched:
// "id", "name" or "email". Name is empty for an id passed through unmatched.
type UserMatch struct {
	ID, Name, By string
}

// Label is the user's name, or the id when the match has no name.
func (m UserMatch) Label() string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// MatchUser resolves s against the account's active users. It tries, in
// order: an exact id, an exact name, a name ignoring case when only one user
// has it, and an email ignoring case. More than one user with the name is a
// Refusal, and so is a value that matches nobody. With idPassthrough, a value that matches nobody but has the form
// of an id is returned as it is, so a deactivated user's id still works.
func (f *Fizzy) MatchUser(s string, idPassthrough bool) (UserMatch, error) {
	users, err := f.Users()
	if err != nil {
		return UserMatch{}, err
	}
	return matchUser(users, s, idPassthrough)
}

// MatchUsersIn resolves each spec with the MatchUser rules, over a user
// list the caller already holds. It drops a user named twice and keeps the
// first order. A spec that matches nobody, or more than one user, is a
// Refusal.
func MatchUsersIn(users []User, specs []string) ([]UserMatch, error) {
	var out []UserMatch
	seen := map[string]bool{}
	for _, s := range specs {
		u, err := matchUser(users, s, false)
		if err != nil {
			return nil, err
		}
		if !seen[u.ID] {
			seen[u.ID] = true
			out = append(out, u)
		}
	}
	return out, nil
}

func matchUser(users []User, s string, idPassthrough bool) (UserMatch, error) {
	for _, u := range users {
		if u.ID == s {
			return UserMatch{u.ID, u.Name, "id"}, nil
		}
	}
	for _, same := range []func(string) bool{
		func(name string) bool { return name == s },
		func(name string) bool { return strings.EqualFold(name, s) },
	} {
		var named []User
		for _, u := range users {
			if same(u.Name) {
				named = append(named, u)
			}
		}
		switch len(named) {
		case 0:
		case 1:
			return UserMatch{named[0].ID, named[0].Name, "name"}, nil
		default:
			return UserMatch{}, refusef("%d users are named %s; use the user id or email", len(named), PyRepr(s))
		}
	}
	for _, u := range users {
		if u.EmailAddress != "" && strings.EqualFold(u.EmailAddress, s) {
			return UserMatch{u.ID, u.Name, "email"}, nil
		}
	}
	if idPassthrough && looksLikeID(s) {
		return UserMatch{ID: s, By: "id"}, nil
	}
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = config.TerminalLine(u.Name)
	}
	return UserMatch{}, refusef("no user %s (have: %s)", PyRepr(s), strings.Join(names, ", "))
}

// looksLikeID reports whether s has the form of a Fizzy id: 20 to 32
// lowercase letters and digits, at least one of them a digit.
func looksLikeID(s string) bool {
	if len(s) < 20 || len(s) > 32 {
		return false
	}
	digit := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r < 'a' || r > 'z':
			return false
		}
	}
	return digit
}
