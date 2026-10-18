package fizzy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/client"
)

// EmailFrequencies are the bundle_email_frequency values Fizzy accepts.
var EmailFrequencies = []string{"never", "every_few_hours", "daily", "weekly"}

// Account is the account from GET /account/settings.
type Account struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	CardsCount               int    `json:"cards_count"`
	CreatedAt                string `json:"created_at"`
	AutoPostponePeriodInDays *int   `json:"auto_postpone_period_in_days"`

	raw json.RawMessage
}

// Identity is the person behind the token, with one entry per account.
type Identity struct {
	ID       string            `json:"id"`
	Accounts []IdentityAccount `json:"accounts"`

	raw json.RawMessage
}

// IdentityAccount is one account of an identity. Slug has a leading slash.
type IdentityAccount struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"created_at"`
	User      User   `json:"user"`
}

// Notification is one of the current user's notifications.
type Notification struct {
	ID        string  `json:"id"`
	Read      bool    `json:"read"`
	ReadAt    *string `json:"read_at"`
	CreatedAt string  `json:"created_at"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Creator   *User   `json:"creator"`
	Card      *struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		URL    string `json:"url"`
	} `json:"card"`
	URL string `json:"url"`

	raw json.RawMessage
}

// NotificationSettings are the current user's notification settings.
type NotificationSettings struct {
	BundleEmailFrequency string `json:"bundle_email_frequency"`

	raw json.RawMessage
}

// Activity is one entry of the account's activity feed. Eventable.Number is
// 0 when the eventable is not a card.
type Activity struct {
	ID            string         `json:"id"`
	Action        string         `json:"action"`
	CreatedAt     string         `json:"created_at"`
	Description   string         `json:"description"`
	Particulars   map[string]any `json:"particulars"`
	URL           string         `json:"url"`
	EventableType string         `json:"eventable_type"`
	Eventable     struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
	} `json:"eventable"`
	Board   *Board `json:"board"`
	Creator *User  `json:"creator"`

	raw json.RawMessage
}

// Export is an account or personal data export. DownloadURL is set only
// when the export is completed and its file has not expired.
type Export struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	DownloadURL string `json:"download_url"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this account.
func (a Account) Raw() json.RawMessage { return a.raw }

// Raw returns the server's JSON for this identity.
func (i Identity) Raw() json.RawMessage { return i.raw }

// Raw returns the server's JSON for this notification.
func (n Notification) Raw() json.RawMessage { return n.raw }

// Raw returns the server's JSON for these settings.
func (s NotificationSettings) Raw() json.RawMessage { return s.raw }

// Raw returns the server's JSON for this activity.
func (a Activity) Raw() json.RawMessage { return a.raw }

// Raw returns the server's JSON for this export.
func (e Export) Raw() json.RawMessage { return e.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (a *Account) UnmarshalJSON(data []byte) error {
	type plain Account
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*a = Account(p)
	a.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (i *Identity) UnmarshalJSON(data []byte) error {
	type plain Identity
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*i = Identity(p)
	i.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (n *Notification) UnmarshalJSON(data []byte) error {
	type plain Notification
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*n = Notification(p)
	n.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (s *NotificationSettings) UnmarshalJSON(data []byte) error {
	type plain NotificationSettings
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*s = NotificationSettings(p)
	s.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (a *Activity) UnmarshalJSON(data []byte) error {
	type plain Activity
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*a = Activity(p)
	a.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (e *Export) UnmarshalJSON(data []byte) error {
	type plain Export
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*e = Export(p)
	e.raw = append(json.RawMessage(nil), data...)
	return nil
}

// decodeWrite decodes a write's reply into v. A nil reply (dry run or an
// empty body) leaves v as it is.
func decodeWrite(raw json.RawMessage, err error, v any) error {
	if err != nil || raw == nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// -- account --------------------------------------------------------------

// AccountSettings fetches the account.
func (f *Fizzy) AccountSettings() (Account, error) {
	var a Account
	err := f.C.Get("/account/settings", &a)
	return a, err
}

// PostponeAccount sets the account's default auto-postpone period and
// returns the account, which is empty in dry-run mode. The server checks
// which periods it accepts.
func (f *Fizzy) PostponeAccount(days int) (Account, error) {
	var a Account
	if days <= 0 {
		return a, errors.New("days must be a positive number")
	}
	raw, err := f.C.Write(http.MethodPut, "/account/entropy", wrap("entropy", struct {
		Days int `json:"auto_postpone_period_in_days"`
	}{days}))
	return a, decodeWrite(raw, err, &a)
}

// -- identity -------------------------------------------------------------

// Identity fetches GET /my/identity, which is not under the account path.
func (f *Fizzy) Identity() (Identity, error) {
	var id Identity
	_, _, payload, err := f.C.Do(http.MethodGet, "/my/identity", nil, client.NotAccountScoped())
	if err != nil {
		return id, err
	}
	err = json.Unmarshal(payload, &id)
	return id, err
}

// Me returns the current user on the configured account, found in the
// identity's account list by slug.
func (f *Fizzy) Me() (User, error) {
	id, err := f.Identity()
	if err != nil {
		return User{}, err
	}
	acct := f.C.Account()
	for _, a := range id.Accounts {
		if strings.Trim(a.Slug, "/") == acct {
			return a.User, nil
		}
	}
	return User{}, fmt.Errorf("account %s is not in /my/identity", acct)
}

// SetTimezone sets the current user's timezone, an IANA name. The caller
// validates the name.
func (f *Fizzy) SetTimezone(name string) error {
	_, err := f.C.Write(http.MethodPatch, "/my/timezone", struct {
		TimezoneName string `json:"timezone_name"`
	}{name})
	return err
}

// -- notifications --------------------------------------------------------

// Notifications lists the current user's notifications, unread first, in
// the order the server sends them.
func (f *Fizzy) Notifications() ([]Notification, error) {
	return list[Notification](f.C, "/notifications")
}

// ReadNotification marks a notification read.
func (f *Fizzy) ReadNotification(id string) error {
	_, err := f.C.Write(http.MethodPost, "/notifications/"+url.PathEscape(id)+"/reading", nil)
	return err
}

// UnreadNotification marks a notification unread.
func (f *Fizzy) UnreadNotification(id string) error {
	_, err := f.C.Write(http.MethodDelete, "/notifications/"+url.PathEscape(id)+"/reading", nil)
	return err
}

// ReadAllNotifications marks every unread notification read.
func (f *Fizzy) ReadAllNotifications() error {
	_, err := f.C.Write(http.MethodPost, "/notifications/bulk_reading", nil)
	return err
}

// NotificationSettings fetches the current user's notification settings.
func (f *Fizzy) NotificationSettings() (NotificationSettings, error) {
	var s NotificationSettings
	err := f.C.Get("/notifications/settings", &s)
	return s, err
}

// SetEmailFrequency sets how often notification emails are bundled. It
// must be one of EmailFrequencies.
func (f *Fizzy) SetEmailFrequency(freq string) error {
	ok := false
	for _, v := range EmailFrequencies {
		ok = ok || v == freq
	}
	if !ok {
		return fmt.Errorf("email frequency must be one of %s", strings.Join(EmailFrequencies, ", "))
	}
	_, err := f.C.Write(http.MethodPut, "/notifications/settings", wrap("user_settings", struct {
		Frequency string `json:"bundle_email_frequency"`
	}{freq}))
	return err
}

// -- activity -------------------------------------------------------------

// Activities lists the activity feed, newest first, following every page.
// An empty boardID or creatorID leaves that filter out.
func (f *Fizzy) Activities(boardID, creatorID string) ([]Activity, error) {
	var parts []string
	if boardID != "" {
		parts = append(parts, url.QueryEscape("board_ids[]")+"="+url.QueryEscape(boardID))
	}
	if creatorID != "" {
		parts = append(parts, url.QueryEscape("creator_ids[]")+"="+url.QueryEscape(creatorID))
	}
	path := "/activities"
	if len(parts) > 0 {
		path += "?" + strings.Join(parts, "&")
	}
	return list[Activity](f.C, path)
}

// -- exports --------------------------------------------------------------

// StartAccountExport starts an account export. The export is empty in
// dry-run mode.
func (f *Fizzy) StartAccountExport() (Export, error) {
	var e Export
	raw, err := f.C.Write(http.MethodPost, "/account/exports", nil)
	return e, decodeWrite(raw, err, &e)
}

// StartMyExport starts a personal data export for the current user. The
// export is empty in dry-run mode.
func (f *Fizzy) StartMyExport() (Export, error) {
	var e Export
	me, err := f.Me()
	if err != nil {
		return e, err
	}
	raw, err := f.C.Write(http.MethodPost, "/users/"+url.PathEscape(me.ID)+"/data_exports", nil)
	return e, decodeWrite(raw, err, &e)
}

// AccountExport fetches an account export's status.
func (f *Fizzy) AccountExport(id string) (Export, error) {
	var e Export
	err := f.C.Get("/account/exports/"+url.PathEscape(id), &e)
	return e, err
}

// MyExport fetches the status of one of the current user's data exports.
func (f *Fizzy) MyExport(id string) (Export, error) {
	var e Export
	me, err := f.Me()
	if err != nil {
		return e, err
	}
	err = f.C.Get("/users/"+url.PathEscape(me.ID)+"/data_exports/"+url.PathEscape(id), &e)
	return e, err
}
