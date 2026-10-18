package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addColumnCommands) }

func addColumnCommands(root *cobra.Command) {
	g := subgroup(root, "column")
	show := &cobra.Command{
		Use:     "show BOARD COLUMN",
		Short:   "show one column",
		Long:    "Print a column's id, name and color. The column is given by name or id.",
		Example: examples("fz column show Home Next", "fz --json column show Home Next"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(columnShow),
	}
	create := &cobra.Command{
		Use:     "create BOARD NAME",
		Short:   "add a column to a board",
		Long:    "Add a custom column at the right of the others. " + columnColorHelp,
		Example: examples(`fz column create Home "Gate repairs" --color Lime`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(columnCreate),
	}
	create.Flags().String("color", "", "column color: "+columnColorFlagHelp)
	edit := &cobra.Command{
		Use:     "edit BOARD COLUMN",
		Short:   "rename or recolor a column",
		Long:    "Change a custom column's name or color. " + columnColorHelp,
		Example: examples(`fz column edit Home Next --name "Hinge repairs"`, "fz column edit Home Next --color tan"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(columnEdit),
	}
	edit.Flags().String("name", "", "new name")
	edit.Flags().String("color", "", "new color: "+columnColorFlagHelp)
	del := &cobra.Command{
		Use:     "delete BOARD COLUMN",
		Short:   "delete a column (needs --yes)",
		Long:    "Delete a custom column. Prints the column and how many cards it holds first, and refuses without --yes.",
		Example: examples("fz column delete Home Next --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(columnDelete),
	}
	del.Flags().Bool("yes", false, "really delete")
	cards := &cobra.Command{
		Use:     "cards BOARD COLUMN",
		Short:   "list the open cards in a column",
		Long:    "Print one line for each open card in the column.",
		Example: examples("fz column cards Home Next"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(columnCards),
	}
	g.AddCommand(show, create, edit, del, cards)
}

// columnColorHelp is the --color paragraph of the column help.
var columnColorHelp = "--color takes a color name, in any case: " + strings.Join(fizzy.ColumnColorNames(), ", ") +
	". It also takes default for Blue, or a number from 1 for Gray to 8 for Pink. " +
	"It also takes the exact value Fizzy stores for one, from var(--color-card-default) for Blue to var(--color-card-8) for Pink. " +
	"fz refuses any other color and sends nothing."

// columnColorFlagHelp is the --color flag's one-line help.
var columnColorFlagHelp = strings.ToLower(strings.Join(fizzy.ColumnColorNames(), ", ")) + ", default, 1 to 8, or a var() value"

// findColumn resolves BOARD and COLUMN arguments.
func findColumn(c *cmdContext, boardArg, colArg string) (*fizzy.Fizzy, fizzy.Board, fizzy.Column, error) {
	fz, err := c.api()
	if err != nil {
		return nil, fizzy.Board{}, fizzy.Column{}, err
	}
	b, err := fz.Board(boardArg)
	if err != nil {
		return nil, b, fizzy.Column{}, err
	}
	col, err := fz.Column(b.ID, colArg)
	return fz, b, col, err
}

func colorLabel(col fizzy.Column) string {
	if col.Color.Name != "" {
		return col.Color.Name
	}
	return orNone(col.Color.Value)
}

func columnShow(c *cmdContext, a []string) error {
	fz, b, found, err := findColumn(c, a[0], a[1])
	if err != nil {
		return err
	}
	col, err := fz.GetColumn(b.ID, found.ID)
	if err != nil {
		return err
	}
	return emit(c.cmd, col.Raw(), fmt.Sprintf("%s  %s  (color %s)", col.ID, col.Name, colorLabel(col)))
}

func columnCreate(c *cmdContext, a []string) error {
	color, _ := c.cmd.Flags().GetString("color")
	if c.cmd.Flags().Changed("color") {
		v, err := fizzy.ColumnColorValue(color)
		if err != nil {
			return err
		}
		color = v
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	id, err := fz.CreateColumn(b.ID, a[1], color)
	if err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"id", orNull(id)}, {"name", a[1]}}),
		c.wrote(fmt.Sprintf("column %s created: %s", pyRepr(a[1]), orNone(id)), "create column "+pyRepr(a[1])))
}

func columnEdit(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	var fields fizzy.ColumnFields
	var changed []string
	if fl.Changed("name") {
		name, _ := fl.GetString("name")
		fields.Name = &name
		changed = append(changed, "name")
	}
	if fl.Changed("color") {
		color, _ := fl.GetString("color")
		v, err := fizzy.ColumnColorValue(color)
		if err != nil {
			return err
		}
		fields.Color = &v
		changed = append(changed, "color")
	}
	if len(changed) == 0 {
		return usagef("give --name or --color")
	}
	fz, b, col, err := findColumn(c, a[0], a[1])
	if err != nil {
		return err
	}
	if err := fz.UpdateColumn(b.ID, col.ID, fields); err != nil {
		return err
	}
	slices.Sort(changed)
	return emit(c.cmd, c.dry(obj{{"id", col.ID}, {"changed", changed}}),
		c.wrote(fmt.Sprintf("column %s updated: %s", col.Name, strings.Join(changed, ", ")),
			fmt.Sprintf("update column %s: %s", col.Name, strings.Join(changed, ", "))))
}

func columnDelete(c *cmdContext, a []string) error {
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, b, col, err := findColumn(c, a[0], a[1])
	if err != nil {
		return err
	}
	cards, err := fz.ColumnCards(b.ID, col.ID)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.errw(), "about to delete column %s (%s) on board %s: %s\n", col.Name, col.ID, b.Name, plural(len(cards), "open card", "open cards"))
	if !yes {
		return refuse(c)
	}
	if err := fz.DeleteColumn(b.ID, col.ID); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"deleted", col.ID}}), c.wrote(fmt.Sprintf("column %s deleted", col.Name), "delete column "+col.Name))
}

func columnCards(c *cmdContext, a []string) error {
	fz, b, col, err := findColumn(c, a[0], a[1])
	if err != nil {
		return err
	}
	cards, err := fz.ColumnCards(b.ID, col.ID)
	if err != nil {
		return err
	}
	return emit(c.cmd, rawList(cards), listText(cards, cardLine, "(no cards)"))
}
