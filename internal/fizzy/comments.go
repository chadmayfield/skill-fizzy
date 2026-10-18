package fizzy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Person is the user named in a comment's creator or a reaction's reacter.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CommentBody is a comment's text, as plain text and as HTML.
type CommentBody struct {
	PlainText string `json:"plain_text"`
	HTML      string `json:"html"`
}

// Comment is a card comment.
type Comment struct {
	ID        string      `json:"id"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
	Body      CommentBody `json:"body"`
	Creator   Person      `json:"creator"`

	raw json.RawMessage
}

// Reaction is a reaction on a comment. Its shape comes from Fizzy's API
// docs; the live server had none to read on 2026-10-17.
type Reaction struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Reacter Person `json:"reacter"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this comment.
func (c Comment) Raw() json.RawMessage { return c.raw }

// Raw returns the server's JSON for this reaction.
func (r Reaction) Raw() json.RawMessage { return r.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (c *Comment) UnmarshalJSON(data []byte) error {
	type plain Comment
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = Comment(p)
	c.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (r *Reaction) UnmarshalJSON(data []byte) error {
	type plain Reaction
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = Reaction(p)
	r.raw = append(json.RawMessage(nil), data...)
	return nil
}

func commentPath(n int, id string) string {
	return "/cards/" + strconv.Itoa(n) + "/comments/" + url.PathEscape(id)
}

// Comments lists a card's comments, oldest first.
func (f *Fizzy) Comments(n int) ([]Comment, error) {
	return list[Comment](f.C, "/cards/"+strconv.Itoa(n)+"/comments")
}

// Comment fetches one comment.
func (f *Fizzy) Comment(n int, id string) (Comment, error) {
	var c Comment
	err := f.C.Get(commentPath(n, id), &c)
	return c, err
}

// EditComment replaces a comment's body. Only the comment's creator can.
func (f *Fizzy) EditComment(n int, id, bodyHTML string) error {
	_, err := f.C.Write(http.MethodPut, commentPath(n, id), wrap("comment", struct {
		Body string `json:"body"`
	}{bodyHTML}))
	return err
}

// DeleteComment deletes a comment. Only the comment's creator can.
func (f *Fizzy) DeleteComment(n int, id string) error {
	_, err := f.C.Write(http.MethodDelete, commentPath(n, id), nil)
	return err
}

// CommentReactions lists the reactions on a comment.
func (f *Fizzy) CommentReactions(n int, id string) ([]Reaction, error) {
	return list[Reaction](f.C, commentPath(n, id)+"/reactions")
}

// ReactToComment adds a reaction and returns its id, empty in dry-run mode
// or when the server sends no Location.
func (f *Fizzy) ReactToComment(n int, id, content string) (string, error) {
	return f.C.Create(commentPath(n, id)+"/reactions", wrap("reaction", struct {
		Content string `json:"content"`
	}{content}))
}

// DeleteCommentReaction removes a reaction from a comment.
func (f *Fizzy) DeleteCommentReaction(n int, id, reactionID string) error {
	_, err := f.C.Write(http.MethodDelete, commentPath(n, id)+"/reactions/"+url.PathEscape(reactionID), nil)
	return err
}
