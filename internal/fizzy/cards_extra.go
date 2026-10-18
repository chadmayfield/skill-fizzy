package fizzy

// Card extras: assignments, watching, pins, boosts, the header image and the
// lookups card list filters need.
//
// Notes on Fizzy behaviour (verified against basecamp/fizzy source,
// 2026-10-17):
//   - POST /cards/:n/assignments toggles. Assign and Unassign read the card's
//     assignees first so they are idempotent.
//   - The card JSON lists at most 5 assignees and sets has_more_assignees
//     when there are more (app/views/cards/_card.json.jbuilder).
//   - A boost is stored in a VARCHAR(16) column. MySQL counts that limit in
//     characters (code points), not bytes, and the model has no other check.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxBoostChars is the longest boost Fizzy stores, in characters.
const MaxBoostChars = 16

// BoostLength counts a boost's characters the way Fizzy's database does: one
// per Unicode code point.
func BoostLength(text string) int { return utf8.RuneCountInString(text) }

// -- assignments (idempotent over Fizzy's toggle) ----------------------------

func (f *Fizzy) setAssigned(n int, user string, want bool) (UserMatch, bool, error) {
	// Unassign passes an unknown id of the Fizzy form through, so a
	// deactivated user can still be removed. Assign does not, because a typo
	// there would assign someone.
	u, err := f.MatchUser(user, !want)
	if err != nil {
		return UserMatch{}, false, err
	}
	// Assignees is a pointer so a reply without the key is told apart from
	// an empty list.
	var card struct {
		Assignees *[]struct {
			ID string `json:"id"`
		} `json:"assignees"`
		HasMore bool `json:"has_more_assignees"`
	}
	if err := f.C.Get("/cards/"+strconv.Itoa(n), &card); err != nil {
		return u, false, err
	}
	if card.Assignees == nil {
		return u, false, refusef("card #%d has no assignees list in Fizzy's reply, so fz cannot tell whether %s is assigned", n, u.Label())
	}
	present := false
	for _, a := range *card.Assignees {
		present = present || a.ID == u.ID
	}
	if !present && card.HasMore {
		return u, false, refusef("card #%d has more than 5 assignees and Fizzy lists only the first 5, so fz cannot tell whether %s is assigned; change it in Fizzy",
			n, u.Label())
	}
	if present == want {
		return u, false, nil
	}
	_, err = f.C.Write(http.MethodPost, "/cards/"+strconv.Itoa(n)+"/assignments", struct {
		AssigneeID string `json:"assignee_id"`
	}{u.ID})
	return u, err == nil, err
}

// Assign assigns a user to a card and reports whether it changed anything.
// user is matched by MatchUser.
func (f *Fizzy) Assign(n int, user string) (UserMatch, bool, error) {
	return f.setAssigned(n, user, true)
}

// Unassign removes a user from a card and reports whether it changed
// anything. An id that matches no active user is passed through.
func (f *Fizzy) Unassign(n int, user string) (UserMatch, bool, error) {
	return f.setAssigned(n, user, false)
}

// -- watching and pins -------------------------------------------------------

func (f *Fizzy) cardWrite(method string, n int, sub string) error {
	_, err := f.C.Write(method, "/cards/"+strconv.Itoa(n)+sub, nil)
	return err
}

// Watch subscribes the current user to a card's notifications.
func (f *Fizzy) Watch(n int) error { return f.cardWrite(http.MethodPost, n, "/watch") }

// Unwatch unsubscribes the current user from a card's notifications.
func (f *Fizzy) Unwatch(n int) error { return f.cardWrite(http.MethodDelete, n, "/watch") }

// Pin pins a card for the current user.
func (f *Fizzy) Pin(n int) error { return f.cardWrite(http.MethodPost, n, "/pin") }

// Unpin unpins a card for the current user.
func (f *Fizzy) Unpin(n int) error { return f.cardWrite(http.MethodDelete, n, "/pin") }

// Pins lists the current user's pinned cards. The endpoint is not paginated
// and returns at most 100 cards.
func (f *Fizzy) Pins() ([]Card, error) {
	var cards []Card
	err := f.C.Get("/my/pins", &cards)
	return cards, err
}

// RemoveImage removes a card's header image.
func (f *Fizzy) RemoveImage(n int) error { return f.cardWrite(http.MethodDelete, n, "/image") }

// -- boosts (card reactions) -------------------------------------------------

// Boost is a reaction on a card.
type Boost struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Reacter struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"reacter"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this boost.
func (b Boost) Raw() json.RawMessage { return b.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (b *Boost) UnmarshalJSON(data []byte) error {
	type plain Boost
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = Boost(p)
	b.raw = append(json.RawMessage(nil), data...)
	return nil
}

// Boosts lists a card's boosts, oldest first.
func (f *Fizzy) Boosts(n int) ([]Boost, error) {
	return list[Boost](f.C, "/cards/"+strconv.Itoa(n)+"/reactions")
}

// CheckBoost returns a Refusal when text is empty, only spaces, or longer
// than Fizzy stores.
func CheckBoost(text string) error { return checkReactionText("a boost", text) }

// CheckReaction applies the CheckBoost rule to a comment reaction.
func CheckReaction(text string) error { return checkReactionText("a reaction", text) }

func checkReactionText(what, text string) error {
	if strings.TrimSpace(text) == "" {
		return refusef("%s cannot be empty", what)
	}
	if l := BoostLength(text); l > MaxBoostChars {
		return refusef("%s is at most %d characters; this one is %d", what, MaxBoostChars, l)
	}
	return nil
}

// AddBoost adds a boost to a card and returns the server's JSON for it, nil
// in dry-run mode or when the server sends none.
func (f *Fizzy) AddBoost(n int, text string) (json.RawMessage, error) {
	if err := CheckBoost(text); err != nil {
		return nil, err
	}
	return f.C.Write(http.MethodPost, "/cards/"+strconv.Itoa(n)+"/reactions", wrap("reaction", struct {
		Content string `json:"content"`
	}{text}))
}

// RemoveBoost removes one of the current user's boosts from a card.
func (f *Fizzy) RemoveBoost(n int, id string) error {
	if err := CheckIDs(id); err != nil {
		return err
	}
	return f.cardWrite(http.MethodDelete, n, "/reactions/"+url.PathEscape(id))
}

// -- lookups for card list filters ---------------------------------------------

// FindTag finds an account tag by title. A leading # is ignored. An exact
// title wins; otherwise case is ignored, and a title two tags share
// ignoring case is a Refusal.
func (f *Fizzy) FindTag(name string) (Tag, error) {
	want := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(name), "#"))
	tags, err := f.Tags()
	if err != nil {
		return Tag{}, err
	}
	t, n := pickByName(tags, want, func(t Tag) string { return t.Title })
	switch {
	case n == 1:
		return t, nil
	case n > 1:
		return Tag{}, refusef("%d tags are named %s; use the exact title", n, pyRepr(name))
	}
	titles := make([]string, len(tags))
	for i, t := range tags {
		titles[i] = t.Title
	}
	return Tag{}, fmt.Errorf("no tag %s (have: %s)", pyRepr(name), strings.Join(titles, ", "))
}
