package fizzy

import (
	"net/http"
	"strings"
	"testing"
)

const commentJSON = `{"id":"c1","created_at":"2026-01-02T03:04:05Z","updated_at":"x","body":{"plain_text":"Gate latches.\nSecond line.","html":"<p>Gate latches.</p>"},"creator":{"id":"u1","name":"Pat Hinge","role":"owner"},"card":{"id":"k","url":"u"},"reactions_url":"r","url":"u"}`

func TestCommentsListAndShape(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Write([]byte("[" + commentJSON + "]"))
	})
	cs, err := f.Comments(87)
	if err != nil || len(cs) != 1 {
		t.Fatalf("%v %v", cs, err)
	}
	c := cs[0]
	if c.ID != "c1" || c.Creator.Name != "Pat Hinge" || c.Body.PlainText != "Gate latches.\nSecond line." || c.Body.HTML != "<p>Gate latches.</p>" || c.CreatedAt == "" {
		t.Errorf("%+v", c)
	}
	if string(c.Raw()) != commentJSON {
		t.Errorf("raw %s", c.Raw())
	}
	if s.calls[0].Path != "/1/cards/87/comments" {
		t.Errorf("path %s", s.calls[0].Path)
	}
}

func TestCommentShow(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Write([]byte(commentJSON))
	})
	c, err := f.Comment(87, "c1")
	if err != nil || c.ID != "c1" || s.calls[0].Path != "/1/cards/87/comments/c1" {
		t.Fatalf("%+v %v %v", c, err, s.calls)
	}
}

func TestEditAndDeleteComment(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {})
	if err := f.EditComment(87, "c1", "<p>New</p>"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteComment(87, "c1"); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Method != "PUT" || s.calls[0].Path != "/1/cards/87/comments/c1" || unHTML(s.calls[0].Body) != `{"comment":{"body":"<p>New</p>"}}` {
		t.Errorf("%+v", s.calls[0])
	}
	if s.calls[1].Method != "DELETE" || s.calls[1].Path != "/1/cards/87/comments/c1" {
		t.Errorf("%+v", s.calls[1])
	}
}

func TestEditCommentForbiddenSurfacesError(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusForbidden)
	})
	err := f.EditComment(87, "c1", "x")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("err %v", err)
	}
}

func TestCommentReactions(t *testing.T) {
	const rx = `{"id":"r1","content":"ok","reacter":{"id":"u1","name":"Pat Hinge"},"url":"u"}`
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.Method {
		case "GET":
			w.Write([]byte("[" + rx + "]"))
		case "POST":
			w.Header().Set("Location", "http://x/1/cards/87/comments/c1/reactions/r2")
			w.WriteHeader(201)
		}
	})
	rs, err := f.CommentReactions(87, "c1")
	if err != nil || len(rs) != 1 || rs[0].ID != "r1" || rs[0].Content != "ok" || rs[0].Reacter.Name != "Pat Hinge" || string(rs[0].Raw()) != rx {
		t.Fatalf("%+v %v", rs, err)
	}
	id, err := f.ReactToComment(87, "c1", "ok")
	if err != nil || id != "r2" {
		t.Fatalf("%q %v", id, err)
	}
	if err := f.DeleteCommentReaction(87, "c1", "r2"); err != nil {
		t.Fatal(err)
	}
	if s.calls[1].Path != "/1/cards/87/comments/c1/reactions" || string(s.calls[1].Body) != `{"reaction":{"content":"ok"}}` {
		t.Errorf("%+v", s.calls[1])
	}
	if s.calls[2].Method != "DELETE" || s.calls[2].Path != "/1/cards/87/comments/c1/reactions/r2" {
		t.Errorf("%+v", s.calls[2])
	}
}

func TestCommentWritesDryRun(t *testing.T) {
	f, s, out, srv := newFizzy(t, true, func(w http.ResponseWriter, r *http.Request, _ []byte) {})
	f.EditComment(87, "c1", "<p>x</p>")
	f.DeleteComment(87, "c1")
	f.ReactToComment(87, "c1", "ok")
	f.DeleteCommentReaction(87, "c1", "r1")
	if len(s.calls) != 0 {
		t.Errorf("dry run sent %v", s.calls)
	}
	p := "[dry-run] "
	for _, w := range []string{
		p + "PUT " + srv.URL + `/1/cards/87/comments/c1 {"comment": {"body": "<p>x</p>"}}`,
		p + "DELETE " + srv.URL + "/1/cards/87/comments/c1",
		p + "POST " + srv.URL + `/1/cards/87/comments/c1/reactions {"reaction": {"content": "ok"}}`,
		p + "DELETE " + srv.URL + "/1/cards/87/comments/c1/reactions/r1",
	} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("missing %q in %q", w, out.String())
		}
	}
}

func TestReactToCommentChecksContent(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusCreated)
	})
	for _, text := range []string{"", "seventeen letters"} {
		if _, err := f.ReactToComment(87, "c1", text); !IsRefusal(err) {
			t.Errorf("%q: want a Refusal, got %v", text, err)
		}
	}
	if len(s.calls) != 0 {
		t.Fatalf("sent %+v", s.calls)
	}
}
