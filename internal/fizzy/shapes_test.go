package fizzy

import (
	"encoding/json"
	"testing"
)

// These samples follow the key names and value types the real server sends,
// with invented gate/hinge values.
const (
	shapeUser   = `{"id":"03abc0000000000000000000a1","name":"Alex","role":"owner","active":true,"email_address":"alex@example.com","created_at":"2026-01-01T00:00:00.000Z","url":"https://fizzy.example.com/users/1","avatar_url":"https://fizzy.example.com/a.png"}`
	shapeBoard  = `{"id":"03abc0000000000000000000b1","name":"Gate","all_access":true,"created_at":"2026-01-01T00:00:00.000Z","auto_postpone_period_in_days":30,"url":"https://fizzy.example.com/boards/1","creator":` + shapeUser + `}`
	shapeColumn = `{"id":"03abc0000000000000000000c1","name":"Hinge","color":{"name":"Blue","value":"var(--color-card-default)"},"created_at":"2026-01-01T00:00:00.000Z","cards_url":"https://fizzy.example.com/c"}`
	shapeCardA  = `{"id":"03abc0000000000000000000d1","number":87,"title":"Oil the hinge","status":"published","description":"d","description_html":"<p>d</p>","image_url":null,"has_attachments":false,"tags":`
	shapeCardB  = `,"closed":false,"postponed":false,"golden":false,"last_active_at":"2026-01-01T00:00:00.000Z","created_at":"2026-01-01T00:00:00.000Z","url":"https://fizzy.example.com/87","board":` + shapeBoard
	shapeCardC  = `,"creator":` + shapeUser + `,"assignees":[],"has_more_assignees":false,"comments_url":"u","reactions_url":"u"`
)

func TestDecodeLiveShapes(t *testing.T) {
	cases := map[string]struct {
		json string
		into any
	}{
		"board":            {shapeBoard, &Board{}},
		"column":           {shapeColumn, &Column{}},
		"column card":      {shapeCardA + `[]` + shapeCardB + `,"column":` + shapeColumn + shapeCardC + `}`, &Card{}},
		"card with column": {shapeCardA + `["gate"]` + shapeCardB + `,"column":` + shapeColumn + shapeCardC + `}`, &Card{}},
		"card in maybe":    {shapeCardA + `["gate"]` + shapeCardB + shapeCardC + `}`, &Card{}},
		"card with steps":  {shapeCardA + `["gate"]` + shapeCardB + shapeCardC + `,"steps":[{"id":"03abc123def456ghi789jkl0mn","content":"Oil it","completed":true}]}`, &Card{}},
		"tag":              {`{"id":"03abc0000000000000000000e1","title":"gate","created_at":"2026-01-01T00:00:00.000Z","url":"u"}`, &Tag{}},
	}
	for name, c := range cases {
		if err := json.Unmarshal([]byte(c.json), c.into); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	var col Column
	if err := json.Unmarshal([]byte(shapeColumn), &col); err == nil {
		if col.Color.Name != "Blue" || col.Color.Value != "var(--color-card-default)" {
			t.Errorf("color = %+v", col.Color)
		}
		if string(col.Raw()) != shapeColumn {
			t.Errorf("raw changed")
		}
	}
}
