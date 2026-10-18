package fizzy

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	var u User
	err := f.C.Get("/users/"+url.PathEscape(id), &u)
	return u, err
}

// RenameUser changes a user's display name and returns the updated user,
// which is empty in dry-run mode.
func (f *Fizzy) RenameUser(id, name string) (User, error) {
	raw, err := f.C.Write(http.MethodPut, "/users/"+url.PathEscape(id), wrap("user", struct {
		Name string `json:"name"`
	}{name}))
	var u User
	if err != nil || raw == nil {
		return u, err
	}
	err = json.Unmarshal(raw, &u)
	return u, err
}
