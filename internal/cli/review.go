package cli

import (
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/review"
	"github.com/spf13/cobra"
)

func init() { register(addReviewCommand) }

func addReviewCommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "review [BOARD...]",
		Short: "list the last column before Done, count the other columns, and show golden, postponing-soon and stalled cards",
		Long: "For each board, list the cards in the last column (nearest Done), count the other columns and " +
			"Maybe?, and list golden, postponing-soon and stalled cards. Closed cards are left out.",
		Example: examples("fz review", "fz review Garage Home", "fz --json review Garage"),
		RunE:    action(reviewRun),
	})
}

func reviewRun(c *cmdContext, names []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	var boards []fizzy.Board
	if len(names) == 0 {
		if boards, err = fz.Boards(); err != nil {
			return err
		}
	}
	for _, n := range names {
		b, err := fz.Board(n)
		if err != nil {
			return err
		}
		boards = append(boards, b)
	}
	results := make([]review.Result, len(boards))
	texts := make([]string, len(boards))
	for i, b := range boards {
		if results[i], err = review.Gather(fz, b); err != nil {
			return err
		}
		texts[i] = review.FormatBoard(results[i])
	}
	out := make([]obj, len(results))
	for i, r := range results {
		lanes := make([]obj, len(r.Lanes))
		for j, l := range r.Lanes {
			lanes[j] = obj{{"name", l.Name}, {"cards", rawList(l.Cards)}}
		}
		var days any
		if r.PostponeDays != nil {
			days = *r.PostponeDays
		}
		out[i] = obj{{"board", r.Board}, {"postpone_days", days}, {"lanes", lanes}, {"maybe", rawList(r.Maybe)},
			{"golden", rawList(r.Golden)}, {"postponing_soon", rawList(r.PostponingSoon)}, {"stalled", rawList(r.Stalled)}}
	}
	return emit(c.cmd, out, strings.Join(texts, "\n\n"))
}
