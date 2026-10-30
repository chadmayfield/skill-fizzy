package fizzy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// An id typed by the user lands in the URL path. Each one is escaped, so a
// "?" stays in the path instead of starting a query string.
func TestPlan2PathsEscapeIDs(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if strings.HasSuffix(r.URL.Path, "/webhooks") || strings.HasSuffix(r.URL.Path, "/reactions") || strings.HasSuffix(r.URL.Path, "/deliveries") {
				io.WriteString(w, "[]")
				return
			}
			io.WriteString(w, "{}")
			return
		}
		if r.Method == http.MethodPatch {
			io.WriteString(w, `{"id":"c?2"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	const b, id, x = "b?1", "c?2", "r?3"
	name := "Gate"
	steps := []struct {
		want string
		run  func() error
	}{
		{"/1/cards/87/reactions/c?2", func() error { return f.RemoveBoost(87, id) }},
		{"/1/cards/87/comments/c?2", func() error { _, err := f.Comment(87, id); return err }},
		{"/1/cards/87/comments/c?2", func() error { return f.EditComment(87, id, "<p>x</p>") }},
		{"/1/cards/87/comments/c?2", func() error { return f.DeleteComment(87, id) }},
		{"/1/cards/87/comments/c?2/reactions", func() error { _, err := f.CommentReactions(87, id); return err }},
		{"/1/cards/87/comments/c?2/reactions/r?3", func() error { return f.DeleteCommentReaction(87, id, x) }},
		{"/1/boards/b?1/webhooks", func() error { _, err := f.Webhooks(b); return err }},
		{"/1/boards/b?1/webhooks/c?2", func() error { _, err := f.Webhook(b, id); return err }},
		{"/1/boards/b?1/webhooks/c?2/deliveries", func() error { _, err := f.WebhookDeliveries(b, id); return err }},
		{"/1/boards/b?1/webhooks/c?2", func() error { _, err := f.UpdateWebhook(b, id, &name, nil); return err }},
		{"/1/boards/b?1/webhooks/c?2", func() error { return f.DeleteWebhook(b, id) }},
		{"/1/boards/b?1/webhooks/c?2", func() error { _, err := f.ActivateWebhook(b, id); return err }},
		{"/1/boards/b?1", func() error { _, err := f.BoardDetail(b); return err }},
		{"/1/boards/b?1", func() error { return f.UpdateBoard(b, BoardFields{Name: &name}) }},
		{"/1/boards/b?1", func() error { return f.SetBoardAccess(b, true, nil) }},
		{"/1/boards/b?1/publication", func() error { _, err := f.PublishBoard(b); return err }},
		{"/1/boards/b?1/publication", func() error { return f.UnpublishBoard(b) }},
		{"/1/boards/b?1/accesses", func() error { _, err := f.BoardAccesses(b); return err }},
		{"/1/boards/b?1", func() error { return f.DeleteBoard(b) }},
		{"/1/boards/b?1/columns/c?2", func() error { _, err := f.GetColumn(b, id); return err }},
		{"/1/boards/b?1/columns/c?2", func() error { return f.UpdateColumn(b, id, ColumnFields{Name: &name}) }},
		{"/1/boards/b?1/columns/c?2", func() error { return f.DeleteColumn(b, id) }},
	}
	for _, st := range steps {
		s.calls = nil
		if err := st.run(); err != nil {
			t.Errorf("%s: %v", st.want, err)
			continue
		}
		if len(s.calls) == 0 || s.calls[0].Path != st.want {
			t.Errorf("want %s, got %+v", st.want, s.calls)
		}
		for _, c := range s.calls {
			if !strings.HasPrefix(c.Path, st.want) || c.Query != "" {
				t.Errorf("want %s, got %s %s?%s", st.want, c.Method, c.Path, c.Query)
			}
		}
	}
}
