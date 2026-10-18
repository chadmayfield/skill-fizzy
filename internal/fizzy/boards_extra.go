package fizzy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/client"
)

// BoardInfo is one board as GET /boards/:id returns it. The list endpoint
// sends fewer fields, so Board holds only those.
type BoardInfo struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	AllAccess                bool     `json:"all_access"`
	AutoPostponePeriodInDays *int     `json:"auto_postpone_period_in_days"`
	URL                      string   `json:"url"`
	PublicURL                string   `json:"public_url"`
	PublicDescription        string   `json:"public_description"`
	UserIDs                  []string `json:"user_ids"`
	Creator                  struct {
		Name string `json:"name"`
	} `json:"creator"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this board.
func (b BoardInfo) Raw() json.RawMessage { return b.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (b *BoardInfo) UnmarshalJSON(data []byte) error {
	type plain BoardInfo
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = BoardInfo(p)
	b.raw = append(json.RawMessage(nil), data...)
	return nil
}

// BoardDetail fetches one board by id.
func (f *Fizzy) BoardDetail(boardID string) (BoardInfo, error) {
	var b BoardInfo
	err := f.C.Get("/boards/"+url.PathEscape(boardID), &b)
	return b, err
}

// BoardFields are the fields UpdateBoard can change. Nil fields are not sent.
type BoardFields struct {
	Name              *string `json:"name,omitempty"`
	PostponeDays      *int    `json:"auto_postpone_period_in_days,omitempty"`
	PublicDescription *string `json:"public_description,omitempty"`
}

// UpdateBoard changes the fields that are set. The public description is
// sent as given, so the caller converts Markdown first.
func (f *Fizzy) UpdateBoard(boardID string, fields BoardFields) error {
	if fields == (BoardFields{}) {
		return refusef("nothing to update")
	}
	if fields.PostponeDays != nil && !slices.Contains(AutoPostponeDays, *fields.PostponeDays) {
		return refusef("auto-postpone must be one of %s", pyIntTuple(AutoPostponeDays))
	}
	_, err := f.C.Write(http.MethodPut, "/boards/"+url.PathEscape(boardID), wrap("board", fields))
	return err
}

// SetBoardAccess opens a board to every user, or limits it to userIDs. The
// list replaces the whole set of users who have access.
func (f *Fizzy) SetBoardAccess(boardID string, all bool, userIDs []string) error {
	var body any
	if all {
		body = wrap("board", struct {
			All bool `json:"all_access"`
		}{true})
	} else {
		body = wrap("board", struct {
			All     bool     `json:"all_access"`
			UserIDs []string `json:"user_ids"`
		}{false, userIDs})
	}
	_, err := f.C.Write(http.MethodPut, "/boards/"+url.PathEscape(boardID), body)
	return err
}

// PublishBoard publishes a board and returns its public link, empty in
// dry-run mode.
func (f *Fizzy) PublishBoard(boardID string) (string, error) {
	raw, err := f.C.Write(http.MethodPost, "/boards/"+url.PathEscape(boardID)+"/publication", nil)
	if err != nil || raw == nil {
		return "", err
	}
	var b BoardInfo
	if err := json.Unmarshal(raw, &b); err != nil {
		return "", err
	}
	return b.PublicURL, nil
}

// UnpublishBoard removes a board's public link.
func (f *Fizzy) UnpublishBoard(boardID string) error {
	_, err := f.C.Write(http.MethodDelete, "/boards/"+url.PathEscape(boardID)+"/publication", nil)
	return err
}

// DeleteBoard deletes a board and its cards.
func (f *Fizzy) DeleteBoard(boardID string) error {
	_, err := f.C.Write(http.MethodDelete, "/boards/"+url.PathEscape(boardID), nil)
	return err
}

// AccessUser is one account user and whether the board is open to them.
// Involvement is empty when the server sends null.
type AccessUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	HasAccess   bool   `json:"has_access"`
	Involvement string `json:"involvement"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this user.
func (u AccessUser) Raw() json.RawMessage { return u.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (u *AccessUser) UnmarshalJSON(data []byte) error {
	type plain AccessUser
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*u = AccessUser(p)
	u.raw = append(json.RawMessage(nil), data...)
	return nil
}

// BoardAccess is the board's access report with every page of users.
type BoardAccess struct {
	BoardID   string
	AllAccess bool
	Users     []AccessUser
}

// BoardAccesses reads who can reach a board, following every page.
func (f *Fizzy) BoardAccesses(boardID string) (BoardAccess, error) {
	var out BoardAccess
	next := f.C.URL("/boards/"+url.PathEscape(boardID)+"/accesses", true)
	seen := map[string]bool{}
	for next != "" && !seen[next] {
		seen[next] = true
		_, hdr, payload, err := f.C.Do(http.MethodGet, next, nil)
		if err != nil {
			return out, err
		}
		var page struct {
			BoardID   string       `json:"board_id"`
			AllAccess bool         `json:"all_access"`
			Users     []AccessUser `json:"users"`
		}
		if err := json.Unmarshal(payload, &page); err != nil {
			return out, err
		}
		out.BoardID, out.AllAccess = page.BoardID, page.AllAccess
		out.Users = append(out.Users, page.Users...)
		next = client.NextLink(hdr.Get("Link"))
	}
	return out, nil
}

// columnColors are the column colors Fizzy's API docs list, in their order.
// Each name maps to the only CSS value the server stores for it.
var columnColors = []struct{ Name, Value string }{
	{"Blue", "var(--color-card-default)"},
	{"Gray", "var(--color-card-1)"},
	{"Tan", "var(--color-card-2)"},
	{"Yellow", "var(--color-card-3)"},
	{"Lime", "var(--color-card-4)"},
	{"Aqua", "var(--color-card-5)"},
	{"Violet", "var(--color-card-6)"},
	{"Purple", "var(--color-card-7)"},
	{"Pink", "var(--color-card-8)"},
}

// ColumnColorNames lists the color names ColumnColorValue accepts.
func ColumnColorNames() []string {
	names := make([]string, len(columnColors))
	for i, c := range columnColors {
		names[i] = c.Name
	}
	return names
}

// ColumnColorValue turns a color name such as "Lime", in any case, into the
// CSS value Fizzy stores. One of those exact var() values passes through.
// Anything else is a Refusal, because the server accepts an unknown value
// without using it and still reports success.
func ColumnColorValue(color string) (string, error) {
	for _, c := range columnColors {
		if strings.EqualFold(color, c.Name) || color == c.Value {
			return c.Value, nil
		}
	}
	return "", refusef("unknown column color %s; use one of %s, or one of their var(--color-card-...) values",
		pyRepr(color), strings.Join(ColumnColorNames(), ", "))
}

// GetColumn fetches one column by id.
func (f *Fizzy) GetColumn(boardID, columnID string) (Column, error) {
	var c Column
	err := f.C.Get("/boards/"+url.PathEscape(boardID)+"/columns/"+url.PathEscape(columnID), &c)
	return c, err
}

// ColumnFields are the fields UpdateColumn can change. Nil fields are not
// sent. Color is a name such as "Lime" or a var() value.
type ColumnFields struct {
	Name  *string
	Color *string
}

// UpdateColumn changes a column's name or color.
func (f *Fizzy) UpdateColumn(boardID, columnID string, fields ColumnFields) error {
	if fields == (ColumnFields{}) {
		return refusef("nothing to update")
	}
	body := struct {
		Name  *string `json:"name,omitempty"`
		Color *string `json:"color,omitempty"`
	}{Name: fields.Name}
	if fields.Color != nil {
		c, err := ColumnColorValue(*fields.Color)
		if err != nil {
			return err
		}
		body.Color = &c
	}
	_, err := f.C.Write(http.MethodPut, "/boards/"+url.PathEscape(boardID)+"/columns/"+url.PathEscape(columnID), wrap("column", body))
	return err
}

// DeleteColumn deletes a column.
func (f *Fizzy) DeleteColumn(boardID, columnID string) error {
	_, err := f.C.Write(http.MethodDelete, "/boards/"+url.PathEscape(boardID)+"/columns/"+url.PathEscape(columnID), nil)
	return err
}
