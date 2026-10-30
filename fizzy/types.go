package fizzy

import "encoding/json"

// Board is a Fizzy board. AutoPostponePeriodInDays is nil when the server
// omits it.
type Board struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	AutoPostponePeriodInDays *int   `json:"auto_postpone_period_in_days"`

	raw json.RawMessage
}

// ColumnColor is a column's color. Some servers send a {name, value}
// object and Fizzy's API docs show a plain string such as
// "var(--color-card-4)". A string fills only Value.
type ColumnColor struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// UnmarshalJSON accepts the color as a string, an object or null.
func (c *ColumnColor) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		*c = ColumnColor{}
		return json.Unmarshal(data, &c.Value)
	}
	type plain ColumnColor
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = ColumnColor(p)
	return nil
}

// Column is a board column.
type Column struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Color ColumnColor `json:"color"`

	raw json.RawMessage
}

// CardStep is a step on a card. JSON keys match the field names without
// regard to case.
type CardStep struct {
	ID, Content string
	Completed   bool
}

// Card holds the card fields the commands read. Column is nil when the card
// is in Maybe? or Not now; Postponed is true only in Not now. List responses
// omit Steps.
type Card struct {
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Board           Board      `json:"board"`
	Column          *Column    `json:"column"`
	Closed          bool       `json:"closed"`
	Postponed       bool       `json:"postponed"`
	Golden          bool       `json:"golden"`
	Tags            []string   `json:"tags"`
	URL             string     `json:"url"`
	Description     string     `json:"description"`
	DescriptionHTML string     `json:"description_html"`
	LastActiveAt    string     `json:"last_active_at"`
	Steps           []CardStep `json:"steps"`

	raw json.RawMessage
}

// Tag is an entry from the account tag list.
type Tag struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url"`

	raw json.RawMessage
}

// Each type below keeps the server's bytes as received, with key order and
// unknown fields, so --json can print exactly what the server sent.

// Raw returns the server's JSON for this board.
func (b Board) Raw() json.RawMessage { return b.raw }

// Raw returns the server's JSON for this column.
func (c Column) Raw() json.RawMessage { return c.raw }

// Raw returns the server's JSON for this card.
func (c Card) Raw() json.RawMessage { return c.raw }

// Raw returns the server's JSON for this tag.
func (t Tag) Raw() json.RawMessage { return t.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (b *Board) UnmarshalJSON(data []byte) error {
	type plain Board
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = Board(p)
	b.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (c *Column) UnmarshalJSON(data []byte) error {
	type plain Column
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = Column(p)
	c.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (c *Card) UnmarshalJSON(data []byte) error {
	type plain Card
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = Card(p)
	c.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (t *Tag) UnmarshalJSON(data []byte) error {
	type plain Tag
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*t = Tag(p)
	t.raw = append(json.RawMessage(nil), data...)
	return nil
}

// CardFields are the fields UpdateCard can change. Nil fields are not sent.
type CardFields struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}

// Param is one query parameter. Cards sends them in slice order, because
// url.Values sorts keys and the Python client kept insertion order.
type Param struct{ Key, Value string }
