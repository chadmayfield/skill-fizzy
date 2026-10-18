package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addBoardCommands) }

func addBoardCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "board",
		Short: "list and create boards",
		Long:  "Work with boards. A board is named or given by id.",
	}
	root.AddCommand(g)
	list := &cobra.Command{
		Use:     "list",
		Short:   "list boards",
		Long:    "Print each board's id, name and auto-postpone period.",
		Example: examples("fz board list"),
		Args:    args(cobra.NoArgs),
		RunE:    action(boardList),
	}
	create := &cobra.Command{
		Use:     "create NAME",
		Short:   "create a board",
		Long:    "Create a board, then set its auto-postpone period and custom columns. An empty or blank name exits 2 and sends nothing.",
		Example: examples("fz board create Garden --postpone 365 --columns Later,Next,Now"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardCreate),
	}
	create.Flags().Int("postpone", 0, "days of no activity before cards move to Not now")
	create.Flags().String("columns", "", "comma-separated custom columns, left to right")
	g.AddCommand(list, create)

	cg := &cobra.Command{
		Use:   "column",
		Short: "list a board's columns",
		Long:  "Work with columns. Not now, Maybe? and Done are fixed; only custom columns change.",
	}
	root.AddCommand(cg)
	cg.AddCommand(&cobra.Command{
		Use:     "list BOARD",
		Short:   "list a board's lanes, left to right",
		Long:    "Print the lanes from Not now to Done.",
		Example: examples("fz column list Home"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(columnList),
	})
}

func boardList(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	boards, err := fz.Boards()
	if err != nil {
		return err
	}
	lines := make([]string, len(boards))
	for i, b := range boards {
		days := "None"
		if b.AutoPostponePeriodInDays != nil {
			days = strconv.Itoa(*b.AutoPostponePeriodInDays)
		}
		lines[i] = fmt.Sprintf("%s  %s  (postpone %sd)", b.ID, b.Name, days)
	}
	return emit(c.cmd, rawList(boards), strings.Join(lines, "\n"))
}

// postponeDays checks --postpone against Fizzy's periods, as argparse's
// choices did. 0 means not given.
func postponeDays(c *cmdContext) (int, error) {
	fl := c.cmd.Flags()
	if !fl.Changed("postpone") {
		return 0, nil
	}
	n, _ := fl.GetInt("postpone")
	allowed := make([]string, len(fizzy.AutoPostponeDays))
	for i, d := range fizzy.AutoPostponeDays {
		allowed[i] = strconv.Itoa(d)
		if d == n {
			return n, nil
		}
	}
	return 0, usagef("argument --postpone: invalid choice: %d (choose from %s)", n, strings.Join(allowed, ", "))
}

func boardCreate(c *cmdContext, a []string) error {
	name := strings.TrimSpace(a[0])
	if err := checkName("the board name", name); err != nil {
		return err
	}
	days, err := postponeDays(c)
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	id, err := fz.CreateBoard(name, days)
	if err != nil {
		return err
	}
	spec, _ := c.cmd.Flags().GetString("columns")
	cols := []string{}
	for _, name := range strings.Split(spec, ",") {
		if name = pyStrip(name); name != "" {
			cols = append(cols, name)
		}
	}
	if id != "" {
		for _, name := range cols {
			if _, err := fz.CreateColumn(id, name, ""); err != nil {
				return err
			}
		}
	}
	return emit(c.cmd, obj{{"id", orNull(id)}, {"columns", cols}},
		fmt.Sprintf("board %s created: %s", pyRepr(name), orNone(id)))
}

func columnList(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	cols, err := fz.Columns(b.ID)
	if err != nil {
		return err
	}
	names := make([]string, len(cols))
	for i, col := range cols {
		names[i] = col.Name
	}
	return emit(c.cmd, rawList(cols), "Not now → Maybe? → "+strings.Join(names, " → ")+" → Done")
}
