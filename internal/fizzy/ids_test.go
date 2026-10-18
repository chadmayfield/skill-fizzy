package fizzy

import (
	"net/http"
	"testing"
)

// An id of "", "." or ".." would make the path name a different endpoint
// once a proxy or server removes dot segments. Every call that puts a
// user-supplied id into a path refuses one before sending anything.
func TestBadIDsRefuseBeforeSending(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		jsonOK(w, map[string]any{"id": "u1"})
	})
	name := "Gate"
	done := true
	for _, bad := range []string{"", ".", ".."} {
		calls := map[string]func() error{
			"Comment":               func() error { _, err := f.Comment(87, bad); return err },
			"EditComment":           func() error { return f.EditComment(87, bad, "<p>x</p>") },
			"DeleteComment":         func() error { return f.DeleteComment(87, bad) },
			"CommentReactions":      func() error { _, err := f.CommentReactions(87, bad); return err },
			"ReactToComment":        func() error { _, err := f.ReactToComment(87, bad, "ok"); return err },
			"DeleteCommentReaction": func() error { return f.DeleteCommentReaction(87, bad, "r1") },
			"DeleteCommentReaction reaction": func() error {
				return f.DeleteCommentReaction(87, "c1", bad)
			},
			"Webhooks":          func() error { _, err := f.Webhooks(bad); return err },
			"Webhook":           func() error { _, err := f.Webhook("b1", bad); return err },
			"Webhook board":     func() error { _, err := f.Webhook(bad, "w1"); return err },
			"WebhookDeliveries": func() error { _, err := f.WebhookDeliveries("b1", bad); return err },
			"CreateWebhook": func() error {
				_, err := f.CreateWebhook(bad, "Gate", "https://hooks.example.com/x", []string{"card_closed"})
				return err
			},
			"UpdateWebhook":      func() error { _, err := f.UpdateWebhook("b1", bad, &name, nil); return err },
			"DeleteWebhook":      func() error { return f.DeleteWebhook("b1", bad) },
			"ActivateWebhook":    func() error { _, err := f.ActivateWebhook("b1", bad); return err },
			"RemoveBoost":        func() error { return f.RemoveBoost(87, bad) },
			"User":               func() error { _, err := f.User(bad); return err },
			"RenameUser":         func() error { _, err := f.RenameUser(bad, "Alex"); return err },
			"ReadNotification":   func() error { return f.ReadNotification(bad) },
			"UnreadNotification": func() error { return f.UnreadNotification(bad) },
			"AccountExport":      func() error { _, err := f.AccountExport(bad); return err },
			"MyExport":           func() error { _, err := f.MyExport(bad); return err },
			"BoardDetail":        func() error { _, err := f.BoardDetail(bad); return err },
			"UpdateBoard":        func() error { return f.UpdateBoard(bad, BoardFields{Name: &name}) },
			"SetBoardAccess":     func() error { return f.SetBoardAccess(bad, true, nil) },
			"PublishBoard":       func() error { _, err := f.PublishBoard(bad); return err },
			"UnpublishBoard":     func() error { return f.UnpublishBoard(bad) },
			"DeleteBoard":        func() error { return f.DeleteBoard(bad) },
			"BoardAccesses":      func() error { _, err := f.BoardAccesses(bad); return err },
			"GetColumn":          func() error { _, err := f.GetColumn("b1", bad); return err },
			"GetColumn board":    func() error { _, err := f.GetColumn(bad, "c1"); return err },
			"UpdateColumn":       func() error { return f.UpdateColumn("b1", bad, ColumnFields{Name: &name}) },
			"DeleteColumn":       func() error { return f.DeleteColumn("b1", bad) },
			"Columns":            func() error { _, err := f.Columns(bad); return err },
			"CreateColumn":       func() error { _, err := f.CreateColumn(bad, "Gate", ""); return err },
			"ColumnCards":        func() error { _, err := f.ColumnCards("b1", bad); return err },
			"ColumnCards board":  func() error { _, err := f.ColumnCards(bad, "c1"); return err },
			"UpdateStep":         func() error { return f.UpdateStep(87, bad, nil, &done) },
			"CreateCard":         func() error { _, err := f.CreateCard(bad, "Gate", "", ""); return err },
			"DeleteStep":         func() error { return f.DeleteStep(87, bad) },
		}
		for label, run := range calls {
			s.calls = nil
			err := run()
			if !IsRefusal(err) {
				t.Errorf("%s(%q): want a Refusal, got %v", label, bad, err)
			}
			if len(s.calls) != 0 {
				t.Errorf("%s(%q): sent %+v", label, bad, s.calls)
			}
		}
	}
}

// CreateColumn and ColumnCards escape their ids like every other path.
func TestColumnPathsEscapeIDs(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.Method == http.MethodPost {
			w.Header().Set("Location", "/1/boards/b/columns/c1.json")
			w.WriteHeader(http.StatusCreated)
			return
		}
		jsonOK(w, []any{})
	})
	if _, err := f.CreateColumn("b?1", "Gate", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ColumnCards("b?1", "c?2"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/1/boards/b?1/columns", "/1/boards/b?1/columns/c?2/cards"}
	if len(s.calls) != 2 {
		t.Fatalf("calls %+v", s.calls)
	}
	for i, w := range want {
		if s.calls[i].Path != w || s.calls[i].Query != "" {
			t.Errorf("call %d: want %s, got %s?%s", i, w, s.calls[i].Path, s.calls[i].Query)
		}
	}
}
