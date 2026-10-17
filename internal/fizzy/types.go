package fizzy

import "github.com/chadmayfield/skill-fizzy/internal/draft"

// Board is a Fizzy board. AutoPostponePeriodInDays is nil when the server
// omits it.
type Board struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	AutoPostponePeriodInDays *int   `json:"auto_postpone_period_in_days"`
}

// Column is a board column. Color is the CSS value the server returns.
type Column struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Card holds the card fields the commands read. Column is nil when the card
// is in Maybe?. List responses omit Steps.
type Card struct {
	Number          int              `json:"number"`
	Title           string           `json:"title"`
	Board           Board            `json:"board"`
	Column          *Column          `json:"column"`
	Closed          bool             `json:"closed"`
	Golden          bool             `json:"golden"`
	Tags            []string         `json:"tags"`
	URL             string           `json:"url"`
	Description     string           `json:"description"`
	DescriptionHTML string           `json:"description_html"`
	LastActiveAt    string           `json:"last_active_at"`
	Steps           []draft.CardStep `json:"steps"`
}

// Tag is an entry from the account tag list.
type Tag struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
