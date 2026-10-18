// Package fizzy holds the high-level Fizzy operations built on client.
//
// Notes on Fizzy behaviour this package works around (verified against Fizzy
// source and docs, 2026-10):
//   - Creating a board with auto_postpone_period_in_days in the same request
//     can fail 422 "Account must exist"; boards are created by name, then
//     updated.
//   - POST /cards/:n/taggings toggles a tag. AddTag and RemoveTag read the
//     card's current tags first so they are idempotent.
//   - Fizzy fixes the lanes "Not now" (left), "Maybe?" (triage), and "Done"
//     (right); only custom columns can be created or reordered.
//
// Request bodies are structs or single-key maps, never multi-key maps: the
// client's dry-run output follows Go's field order for structs and sorted
// keys for maps, and the Python client printed insertion order.
package fizzy

import (
	"bytes"
	"crypto/md5" //nolint:gosec // ActiveStorage requires MD5 checksums.
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/client"
)

// AutoPostponeDays are the auto-postpone periods Fizzy accepts.
var AutoPostponeDays = []int{3, 7, 11, 30, 90, 365}

// Special column targets accepted by MoveCard.
const (
	maybe  = "maybe"
	notNow = "not-now"
	done   = "done"
)

// Fizzy runs Fizzy operations over one client.
type Fizzy struct{ C *client.Client }

func list[T any](c *client.Client, path string) ([]T, error) {
	raw, err := c.GetAll(path)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func wrap(key string, v any) map[string]any { return map[string]any{key: v} }

// -- boards and columns ---------------------------------------------------

// Boards lists every board.
func (f *Fizzy) Boards() ([]Board, error) { return list[Board](f.C, "/boards") }

// Board finds a board by id, or by name ignoring case.
func (f *Fizzy) Board(nameOrID string) (Board, error) {
	boards, err := f.Boards()
	if err != nil {
		return Board{}, err
	}
	for _, b := range boards {
		if b.ID == nameOrID {
			return b, nil
		}
	}
	var matches []Board
	for _, b := range boards {
		if strings.EqualFold(b.Name, nameOrID) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		names := make([]string, len(boards))
		for i, b := range boards {
			names[i] = b.Name
		}
		return Board{}, fmt.Errorf("no board named %s (have: %s)", PyRepr(nameOrID), strings.Join(names, ", "))
	}
	return Board{}, refusef("%d boards are named %s; use the board id", len(matches), PyRepr(nameOrID))
}

// CreateBoard creates a board by name, then sets the auto-postpone period in
// a second request when postponeDays is not 0, then creates each of columns
// in order. It returns the new id, empty in dry-run mode, where only the
// first request is printed. The id comes from the Location header. When it
// is missing, or cannot be a path segment, and a period or columns were
// asked for, the error says the board was created without them.
func (f *Fizzy) CreateBoard(name string, postponeDays int, columns []string) (string, error) {
	if postponeDays != 0 {
		if err := ValidPostponeDays(postponeDays); err != nil {
			return "", err
		}
	}
	// Do, not Create, so a dry run (status 0) is told apart from a reply
	// with no Location.
	status, hdr, _, err := f.C.Do(http.MethodPost, "/boards", wrap("board", struct {
		Name string `json:"name"`
	}{name}))
	if err != nil || status == 0 {
		return "", err
	}
	id := client.IDFromLocation(hdr.Get("Location"))
	var lost []string
	if postponeDays != 0 {
		lost = append(lost, "the auto-postpone period was not set")
	}
	if len(columns) > 0 {
		lost = append(lost, "its columns were not created")
	}
	if len(lost) == 0 {
		return id, nil
	}
	if id == "" {
		return "", fmt.Errorf("board %s was created, but Fizzy sent no id for it, so %s", PyRepr(name), strings.Join(lost, " and "))
	}
	if err := CheckIDs(id); err != nil {
		return "", fmt.Errorf("board %s was created, but its id from Fizzy cannot be used (%w), so %s", PyRepr(name), err, strings.Join(lost, " and "))
	}
	if postponeDays != 0 {
		if _, err := f.C.Write(http.MethodPut, "/boards/"+url.PathEscape(id), wrap("board", struct {
			Days int `json:"auto_postpone_period_in_days"`
		}{postponeDays})); err != nil {
			return id, err
		}
	}
	for _, col := range columns {
		if _, err := f.CreateColumn(id, col, ""); err != nil {
			return id, err
		}
	}
	return id, nil
}

// Columns lists a board's custom columns.
func (f *Fizzy) Columns(boardID string) ([]Column, error) {
	if err := CheckIDs(boardID); err != nil {
		return nil, err
	}
	return list[Column](f.C, "/boards/"+url.PathEscape(boardID)+"/columns")
}

// Column finds a column by id, then by exact name, then by name ignoring
// case. A name that two columns share ignoring case, with no exact match,
// is a Refusal.
func (f *Fizzy) Column(boardID, nameOrID string) (Column, error) {
	cols, err := f.Columns(boardID)
	if err != nil {
		return Column{}, err
	}
	for _, c := range cols {
		if c.ID == nameOrID {
			return c, nil
		}
	}
	c, n := pickByName(cols, nameOrID, func(c Column) string { return c.Name })
	switch {
	case n == 1:
		return c, nil
	case n > 1:
		return Column{}, refusef("%d columns are named %s; use the column id", n, PyRepr(nameOrID))
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return Column{}, fmt.Errorf("no column %s (have: %s)", PyRepr(nameOrID), strings.Join(names, ", "))
}

// pickByName returns the one item whose name is want, or failing that the
// one whose name matches ignoring case, and how many matched. An exact match
// wins over any number of case-insensitive ones.
func pickByName[T any](items []T, want string, name func(T) string) (T, int) {
	var zero T
	for _, it := range items {
		if name(it) == want {
			return it, 1
		}
	}
	var found []T
	for _, it := range items {
		if strings.EqualFold(name(it), want) {
			found = append(found, it)
		}
	}
	if len(found) == 1 {
		return found[0], 1
	}
	return zero, len(found)
}

// CreateColumn creates a column. ColumnColorValue turns the color into the
// value sent, and refuses an unknown one before anything is sent. An empty
// color is left out.
func (f *Fizzy) CreateColumn(boardID, name, color string) (string, error) {
	if err := CheckIDs(boardID); err != nil {
		return "", err
	}
	body := struct {
		Name  string `json:"name"`
		Color string `json:"color,omitempty"`
	}{Name: name}
	if color != "" {
		v, err := ColumnColorValue(color)
		if err != nil {
			return "", err
		}
		body.Color = v
	}
	return f.C.Create("/boards/"+url.PathEscape(boardID)+"/columns", wrap("column", body))
}

// ColumnCards lists the cards in one column, following every page.
func (f *Fizzy) ColumnCards(boardID, columnID string) ([]Card, error) {
	if err := CheckIDs(boardID, columnID); err != nil {
		return nil, err
	}
	return list[Card](f.C, "/boards/"+url.PathEscape(boardID)+"/columns/"+url.PathEscape(columnID)+"/cards")
}

// -- cards ----------------------------------------------------------------

// Cards lists cards. The query is board_ids[] first when boardID is set, then
// filters in slice order. The caller drops filters it does not want.
func (f *Fizzy) Cards(boardID string, filters []Param) ([]Card, error) {
	var parts []string
	add := func(k, v string) { parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v)) }
	if boardID != "" {
		add("board_ids[]", boardID)
	}
	for _, p := range filters {
		add(p.Key, p.Value)
	}
	path := "/cards"
	if len(parts) > 0 {
		path += "?" + strings.Join(parts, "&")
	}
	return list[Card](f.C, path)
}

// Card fetches one card.
func (f *Fizzy) Card(n int) (Card, error) {
	var c Card
	err := f.C.Get("/cards/"+strconv.Itoa(n), &c)
	return c, err
}

// CreateCard creates a card and returns its number, 0 in dry-run mode.
func (f *Fizzy) CreateCard(boardID, title, descHTML, status string) (int, error) {
	if err := CheckIDs(boardID); err != nil {
		return 0, err
	}
	body := struct {
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
		Status      string `json:"status,omitempty"`
	}{title, descHTML, status}
	id, err := f.C.Create("/boards/"+url.PathEscape(boardID)+"/cards", wrap("card", body))
	if err != nil || id == "" {
		return 0, err
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return 0, fmt.Errorf("card id %q is not a number", id)
	}
	return n, nil
}

// UpdateCard changes the fields that are set.
func (f *Fizzy) UpdateCard(n int, fields CardFields) error {
	if fields == (CardFields{}) {
		return refusef("nothing to update")
	}
	_, err := f.C.Write(http.MethodPut, "/cards/"+strconv.Itoa(n), wrap("card", fields))
	return err
}

// MoveCard moves a card to maybe, not-now, done, or a column of the card's
// board. boardID may be empty; the card is fetched to find its board.
func (f *Fizzy) MoveCard(n int, target, boardID string) error {
	if boardID == "" {
		c, err := f.Card(n)
		if err != nil {
			return err
		}
		boardID = c.Board.ID
	}
	t, err := f.ResolveMoveTarget(boardID, target)
	if err != nil {
		return err
	}
	return f.MoveCardTo(n, t)
}

// MoveTarget is a checked place to move a card to: a move keyword, or a
// column on one board. ResolveMoveTarget makes one.
type MoveTarget struct {
	keyword  string // maybe, notNow or done; "" for a column
	columnID string
}

// IsMaybe reports whether the target is the Maybe? keyword, where a new
// card already is.
func (t MoveTarget) IsMaybe() bool { return t.keyword == maybe }

// ResolveMoveTarget checks target against the board's columns and sends
// nothing. A keyword that a custom column is also named, a column name two
// columns share, and a name no column has are each an error, so a caller
// can check every target before its first write.
func (f *Fizzy) ResolveMoveTarget(boardID, target string) (MoveTarget, error) {
	if err := f.CheckMoveTarget(boardID, target); err != nil {
		return MoveTarget{}, err
	}
	if kw := moveKeyword(target); kw != "" {
		return MoveTarget{keyword: kw}, nil
	}
	col, err := f.Column(boardID, target)
	if err != nil {
		return MoveTarget{}, err
	}
	return MoveTarget{columnID: col.ID}, nil
}

// MoveCardTo moves a card to a target from ResolveMoveTarget.
func (f *Fizzy) MoveCardTo(n int, t MoveTarget) error {
	p := "/cards/" + strconv.Itoa(n)
	var err error
	switch t.keyword {
	case maybe:
		_, err = f.C.Write(http.MethodDelete, p+"/triage", nil)
	case notNow:
		_, err = f.C.Write(http.MethodPost, p+"/not_now", nil)
	case done:
		_, err = f.C.Write(http.MethodPost, p+"/closure", nil)
	default:
		_, err = f.C.Write(http.MethodPost, p+"/triage", struct {
			ColumnID string `json:"column_id"`
		}{t.columnID})
	}
	return err
}

// moveKeyword returns maybe, notNow or done when target is that move keyword:
// case, the spaces around it and a trailing ? are ignored, and "not now" is
// notNow. Any other target gives "".
func moveKeyword(target string) string {
	switch t := strings.TrimRight(strings.ToLower(strings.TrimSpace(target)), "?"); t {
	case maybe, done:
		return t
	case notNow, "not now":
		return notNow
	}
	return ""
}

// CheckMoveTarget refuses a move keyword that a custom column on the board
// is also named, such as a column called Done, because fz cannot tell
// whether the user means the column or the keyword. A target that is not a
// keyword is not checked and reads nothing.
func (f *Fizzy) CheckMoveTarget(boardID, target string) error {
	kw := moveKeyword(target)
	if kw == "" {
		return nil
	}
	cols, err := f.Columns(boardID)
	if err != nil {
		return err
	}
	for _, col := range cols {
		if moveKeyword(col.Name) == kw {
			return refusef("the board has a column named '%s', so '%s' could mean that column or the %s keyword; "+
				"to move the card into the column, use --to with the column id (%s); %s",
				col.Name, strings.TrimSpace(target), kw, col.ID, keywordHint[kw])
		}
	}
	return nil
}

// keywordHint says how to get each move keyword's effect while a column
// has its name.
var keywordHint = map[string]string{
	maybe:  "to send the card back to Maybe? with the keyword, rename the column first",
	notNow: "to move the card to Not now with the keyword, rename the column first",
	done:   "to close the card, use fz card close NUMBER",
}

// CloseCard closes a card, the same call as a move to done, without reading
// the board's columns. It reads the card first and sends nothing when the
// card is already closed. It reports whether it sent the closure.
func (f *Fizzy) CloseCard(n int) (bool, error) {
	c, err := f.Card(n)
	if err != nil || c.Closed {
		return false, err
	}
	_, err = f.C.Write(http.MethodPost, "/cards/"+strconv.Itoa(n)+"/closure", nil)
	return err == nil, err
}

// MoveCardToBoard moves a card to another board.
func (f *Fizzy) MoveCardToBoard(n int, boardID string) error {
	_, err := f.C.Write(http.MethodPut, "/cards/"+strconv.Itoa(n)+"/board", struct {
		BoardID string `json:"board_id"`
	}{boardID})
	return err
}

// ReopenCard reopens a closed card.
func (f *Fizzy) ReopenCard(n int) error {
	_, err := f.C.Write(http.MethodDelete, "/cards/"+strconv.Itoa(n)+"/closure", nil)
	return err
}

// SetGolden turns the golden mark on or off.
func (f *Fizzy) SetGolden(n int, on bool) error {
	method := http.MethodDelete
	if on {
		method = http.MethodPost
	}
	_, err := f.C.Write(method, "/cards/"+strconv.Itoa(n)+"/goldness", nil)
	return err
}

// DeleteCard deletes a card.
func (f *Fizzy) DeleteCard(n int) error {
	_, err := f.C.Write(http.MethodDelete, "/cards/"+strconv.Itoa(n), nil)
	return err
}

// -- tags (idempotent over Fizzy's toggle) ---------------------------------

// TagTitle is the title fz sends for a TAG argument: the spaces around it
// and a leading # removed, in lower case.
func TagTitle(tag string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(tag), "#")))
}

func (f *Fizzy) toggleTag(n int, tag string, wantPresent bool) (bool, error) {
	tag = TagTitle(tag)
	if tag == "" {
		return false, refusef("the tag cannot be empty")
	}
	c, err := f.Card(n)
	if err != nil {
		return false, err
	}
	// A reply without the tags key says nothing about the card's tags, and
	// the toggle could remove one it has. card assign refuses the same way.
	var keys map[string]json.RawMessage
	if json.Unmarshal(c.Raw(), &keys) != nil || keys["tags"] == nil {
		return false, refusef("card #%d has no tags list in Fizzy's reply, so fz cannot tell whether it has the tag %s", n, PyRepr(tag))
	}
	present := false
	for _, t := range c.Tags {
		present = present || strings.ToLower(t) == tag
	}
	if present == wantPresent {
		return false, nil
	}
	_, err = f.C.Write(http.MethodPost, "/cards/"+strconv.Itoa(n)+"/taggings", struct {
		TagTitle string `json:"tag_title"`
	}{tag})
	return err == nil, err
}

// AddTag adds a tag and reports whether it changed anything. A leading # and
// case are ignored.
func (f *Fizzy) AddTag(n int, tag string) (bool, error) { return f.toggleTag(n, tag, true) }

// RemoveTag removes a tag and reports whether it changed anything.
func (f *Fizzy) RemoveTag(n int, tag string) (bool, error) { return f.toggleTag(n, tag, false) }

// Tags lists every tag on the account.
func (f *Fizzy) Tags() ([]Tag, error) { return list[Tag](f.C, "/tags") }

// -- steps and comments ----------------------------------------------------

// AddStep adds a step and returns its id, empty in dry-run mode.
func (f *Fizzy) AddStep(n int, content string, done bool) (string, error) {
	return f.C.Create("/cards/"+strconv.Itoa(n)+"/steps", wrap("step", struct {
		Content   string `json:"content"`
		Completed bool   `json:"completed"`
	}{content, done}))
}

// UpdateStep changes a step. It sends only the fields that are non-nil.
func (f *Fizzy) UpdateStep(n int, id string, content *string, completed *bool) error {
	if err := CheckIDs(id); err != nil {
		return err
	}
	if content == nil && completed == nil {
		return refusef("nothing to update")
	}
	_, err := f.C.Write(http.MethodPut, "/cards/"+strconv.Itoa(n)+"/steps/"+url.PathEscape(id), wrap("step", struct {
		Content   *string `json:"content,omitempty"`
		Completed *bool   `json:"completed,omitempty"`
	}{content, completed}))
	return err
}

// DeleteStep deletes a step.
func (f *Fizzy) DeleteStep(n int, id string) error {
	if err := CheckIDs(id); err != nil {
		return err
	}
	_, err := f.C.Write(http.MethodDelete, "/cards/"+strconv.Itoa(n)+"/steps/"+url.PathEscape(id), nil)
	return err
}

// AddComment adds a comment and returns its id, empty in dry-run mode.
func (f *Fizzy) AddComment(n int, bodyHTML string) (string, error) {
	return f.C.Create("/cards/"+strconv.Itoa(n)+"/comments", wrap("comment", struct {
		Body string `json:"body"`
	}{bodyHTML}))
}

// -- uploads (ActiveStorage direct upload) ---------------------------------

// Upload sends a file and returns the sgid to embed as
// <action-text-attachment sgid=...>. In dry-run mode it prints the blob
// request and returns "DRY-RUN-SGID". When the reply has no attachable_sgid
// it returns signed_id and prints a warning on the client's output.
func (f *Fizzy) Upload(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := md5.Sum(data) //nolint:gosec // ActiveStorage requires MD5.
	name := filepath.Base(path)
	ctype := "application/octet-stream"
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		if mt, _, err := mime.ParseMediaType(t); err == nil {
			ctype = mt
		}
	}
	blob := wrap("blob", struct {
		Filename    string `json:"filename"`
		ByteSize    int    `json:"byte_size"`
		Checksum    string `json:"checksum"`
		ContentType string `json:"content_type"`
	}{name, len(data), base64.StdEncoding.EncodeToString(sum[:]), ctype})
	status, _, payload, err := f.C.Do(http.MethodPost, "/rails/active_storage/direct_uploads", blob)
	if err != nil {
		return "", err
	}
	if status == 0 {
		return "DRY-RUN-SGID", nil
	}
	var info struct {
		SignedID       string `json:"signed_id"`
		AttachableSgid string `json:"attachable_sgid"`
		DirectUpload   struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"direct_upload"`
	}
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&info); err != nil {
		return "", err
	}
	if info.DirectUpload.URL == "" {
		return "", errors.New("direct upload reply has no direct_upload.url")
	}
	if info.SignedID == "" && info.AttachableSgid == "" {
		return "", errors.New("direct upload reply has no signed_id or attachable_sgid")
	}
	// The storage URL is absolute for cloud services; a disk service may
	// return a site-relative path, which already carries any account prefix
	// it needs.
	_, _, _, err = f.C.Do(http.MethodPut, info.DirectUpload.URL, nil,
		client.Raw(data), client.Headers(info.DirectUpload.Headers), client.NoAuth(), client.NotAccountScoped(), client.Upload())
	if err != nil {
		return "", err
	}
	// Rich text needs the attachable sgid (purpose "attachable"). Fizzy's API
	// docs say signed_id, but that has purpose "blob_id" and renders as a
	// broken attachment. Rails adds attachable_sgid to the blob JSON
	// (ActionText::Attachable).
	if info.AttachableSgid != "" {
		return info.AttachableSgid, nil
	}
	fmt.Fprintf(f.C.Out(), "fz: warning: Fizzy sent no attachable_sgid for %s, so fz used its signed_id; the attachment may not render\n", name)
	return info.SignedID, nil
}
