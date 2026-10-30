// Package review gathers and formats the weekly look at a board.
package review

import (
	"fmt"
	"strings"

	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/config"
)

// Lane is one custom column and its open cards.
type Lane struct {
	Name  string
	Cards []fizzy.Card
}

// Result is what the weekly look finds on one board. Lanes run left to
// right, the order the server lists columns.
type Result struct {
	Board          string
	PostponeDays   *int
	Lanes          []Lane
	Maybe          []fizzy.Card
	Golden         []fizzy.Card
	PostponingSoon []fizzy.Card
	Stalled        []fizzy.Card
}

// OpenOnly drops closed cards.
func OpenOnly(cards []fizzy.Card) []fizzy.Card {
	out := []fizzy.Card{}
	for _, c := range cards {
		if !c.Closed {
			out = append(out, c)
		}
	}
	return out
}

// Gather reads a board. It requests the columns, then each column's cards in
// column order, then the maybe, golden, postponing_soon and stalled lists.
func Gather(fz *fizzy.Fizzy, board fizzy.Board) (Result, error) {
	r := Result{Board: board.Name, PostponeDays: board.AutoPostponePeriodInDays, Lanes: []Lane{}}
	cols, err := fz.Columns(board.ID)
	if err != nil {
		return r, err
	}
	for _, col := range cols {
		cards, err := fz.ColumnCards(board.ID, col.ID)
		if err != nil {
			return r, err
		}
		r.Lanes = append(r.Lanes, Lane{Name: col.Name, Cards: OpenOnly(cards)})
	}
	for _, ix := range []struct {
		name string
		dst  *[]fizzy.Card
	}{{"maybe", &r.Maybe}, {"golden", &r.Golden}, {"postponing_soon", &r.PostponingSoon}, {"stalled", &r.Stalled}} {
		cards, err := fz.Cards(board.ID, []fizzy.Param{{Key: "indexed_by", Value: ix.name}})
		if err != nil {
			return r, err
		}
		*ix.dst = OpenOnly(cards)
	}
	return r, nil
}

func short(cards []fizzy.Card) string {
	if len(cards) == 0 {
		return "none"
	}
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = fmt.Sprintf("#%d %s", c.Number, config.TerminalLine(c.Title))
	}
	return strings.Join(parts, ", ")
}

// FormatBoard prints one board's block. The last lane (nearest Done) is
// listed in full, the other lanes are counted from right to left.
func FormatBoard(r Result) string {
	head := "== " + config.TerminalLine(r.Board)
	if r.PostponeDays != nil {
		head += fmt.Sprintf(" (postpone %dd)", *r.PostponeDays)
	}
	lines := []string{head}
	var counts []string
	if n := len(r.Lanes); n > 0 {
		last := r.Lanes[n-1]
		lines = append(lines, fmt.Sprintf("%s (%d):", config.TerminalLine(last.Name), len(last.Cards)))
		for _, c := range last.Cards {
			lines = append(lines, fmt.Sprintf("  #%d %s", c.Number, config.TerminalLine(c.Title)))
		}
		if len(last.Cards) == 0 {
			lines = append(lines, "  none")
		}
		for i := n - 2; i >= 0; i-- {
			counts = append(counts, fmt.Sprintf("%s %d", config.TerminalLine(r.Lanes[i].Name), len(r.Lanes[i].Cards)))
		}
	}
	counts = append(counts, fmt.Sprintf("Maybe? %d", len(r.Maybe)))
	lines = append(lines, strings.Join(counts, " · "))
	for _, ix := range []struct {
		label string
		cards []fizzy.Card
	}{{"Golden", r.Golden}, {"Postponing soon", r.PostponingSoon}, {"Stalled", r.Stalled}} {
		lines = append(lines, fmt.Sprintf("%s (%d): %s", ix.label, len(ix.cards), short(ix.cards)))
	}
	return strings.Join(lines, "\n")
}
