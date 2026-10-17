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
		return Board{}, fmt.Errorf("no board named %q (have: %s)", nameOrID, strings.Join(names, ", "))
	}
	return Board{}, fmt.Errorf("%d boards are named %q; use the board id", len(matches), nameOrID)
}

// CreateBoard creates a board by name, then sets the auto-postpone period in
// a second request when postponeDays is not 0. It returns the new id, empty
// in dry-run mode.
func (f *Fizzy) CreateBoard(name string, postponeDays int) (string, error) {
	if postponeDays != 0 {
		ok := false
		for _, d := range AutoPostponeDays {
			ok = ok || d == postponeDays
		}
		if !ok {
			return "", fmt.Errorf("auto-postpone must be one of %v", AutoPostponeDays)
		}
	}
	id, err := f.C.Create("/boards", wrap("board", struct {
		Name string `json:"name"`
	}{name}))
	if err != nil {
		return "", err
	}
	if id != "" && postponeDays != 0 {
		_, err = f.C.Write(http.MethodPut, "/boards/"+id, wrap("board", struct {
			Days int `json:"auto_postpone_period_in_days"`
		}{postponeDays}))
	}
	return id, err
}

// Columns lists a board's custom columns.
func (f *Fizzy) Columns(boardID string) ([]Column, error) {
	return list[Column](f.C, "/boards/"+boardID+"/columns")
}

// Column finds a column by id, or by name ignoring case.
func (f *Fizzy) Column(boardID, nameOrID string) (Column, error) {
	cols, err := f.Columns(boardID)
	if err != nil {
		return Column{}, err
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		if c.ID == nameOrID || strings.EqualFold(c.Name, nameOrID) {
			return c, nil
		}
		names[i] = c.Name
	}
	return Column{}, fmt.Errorf("no column %q (have: %s)", nameOrID, strings.Join(names, ", "))
}

// CreateColumn creates a column. A color that does not start with "var(" is
// wrapped as var(--color-card-<color>); an empty color is left out.
func (f *Fizzy) CreateColumn(boardID, name, color string) (string, error) {
	body := struct {
		Name  string `json:"name"`
		Color string `json:"color,omitempty"`
	}{Name: name, Color: color}
	if color != "" && !strings.HasPrefix(color, "var(") {
		body.Color = "var(--color-card-" + color + ")"
	}
	return f.C.Create("/boards/"+boardID+"/columns", wrap("column", body))
}

// ColumnCards lists the cards in one column, following every page.
func (f *Fizzy) ColumnCards(boardID, columnID string) ([]Card, error) {
	return list[Card](f.C, "/boards/"+boardID+"/columns/"+columnID+"/cards")
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
	body := struct {
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
		Status      string `json:"status,omitempty"`
	}{title, descHTML, status}
	id, err := f.C.Create("/boards/"+boardID+"/cards", wrap("card", body))
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
		return errors.New("nothing to update")
	}
	_, err := f.C.Write(http.MethodPut, "/cards/"+strconv.Itoa(n), wrap("card", fields))
	return err
}

// MoveCard moves a card to maybe, not-now, done, or a column of the card's
// board. boardID may be empty; the card is fetched to find its board.
func (f *Fizzy) MoveCard(n int, target, boardID string) error {
	p := "/cards/" + strconv.Itoa(n)
	t := strings.TrimRight(strings.ToLower(strings.TrimSpace(target)), "?")
	switch t {
	case maybe:
		_, err := f.C.Write(http.MethodDelete, p+"/triage", nil)
		return err
	case notNow, "not now":
		_, err := f.C.Write(http.MethodPost, p+"/not_now", nil)
		return err
	case done:
		_, err := f.C.Write(http.MethodPost, p+"/closure", nil)
		return err
	}
	if boardID == "" {
		c, err := f.Card(n)
		if err != nil {
			return err
		}
		boardID = c.Board.ID
	}
	col, err := f.Column(boardID, target)
	if err != nil {
		return err
	}
	_, err = f.C.Write(http.MethodPost, p+"/triage", struct {
		ColumnID string `json:"column_id"`
	}{col.ID})
	return err
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

func (f *Fizzy) toggleTag(n int, tag string, wantPresent bool) (bool, error) {
	tag = strings.ToLower(strings.TrimSpace(strings.TrimLeft(tag, "#")))
	c, err := f.Card(n)
	if err != nil {
		return false, err
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
	if content == nil && completed == nil {
		return errors.New("nothing to update")
	}
	_, err := f.C.Write(http.MethodPut, "/cards/"+strconv.Itoa(n)+"/steps/"+id, wrap("step", struct {
		Content   *string `json:"content,omitempty"`
		Completed *bool   `json:"completed,omitempty"`
	}{content, completed}))
	return err
}

// DeleteStep deletes a step.
func (f *Fizzy) DeleteStep(n int, id string) error {
	_, err := f.C.Write(http.MethodDelete, "/cards/"+strconv.Itoa(n)+"/steps/"+id, nil)
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
// request and returns "DRY-RUN-SGID".
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
	// The storage URL is absolute for cloud services; a disk service may
	// return a site-relative path, which already carries any account prefix
	// it needs.
	_, _, _, err = f.C.Do(http.MethodPut, info.DirectUpload.URL, nil,
		client.Raw(data), client.Headers(info.DirectUpload.Headers), client.NoAuth(), client.NotAccountScoped())
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
	return info.SignedID, nil
}
