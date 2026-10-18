package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/markdown"
	"github.com/spf13/cobra"
)

// cardStates are the values Fizzy's indexed_by filter takes.
var cardStates = []string{"all", "maybe", "closed", "not_now", "stalled", "postponing_soon", "golden"}

// cardSorts are the values Fizzy's sorted_by filter takes.
var cardSorts = []string{"latest", "newest", "oldest"}

// cardWindows are the values Fizzy's creation and closure filters take.
var cardWindows = []string{"today", "yesterday", "thisweek", "lastweek", "thismonth", "lastmonth", "thisyear", "lastyear"}

func init() { register(addCardCommands) }

func addCardCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "card",
		Short: "list, show, create, edit and move cards",
		Long:  "Work with cards. A card is addressed by its number, the N in /cards/N.",
	}
	root.AddCommand(g)

	list := &cobra.Command{
		Use:   "list",
		Short: "list cards (every page)",
		Long: "List cards, newest activity first. Fizzy returns pages; fz reads all of them. " +
			"Filters combine with AND. --column needs --board. --assignee takes a user id, a name or an email address, matched as card assign matches USER.",
		Example: examples("fz card list --board Home", "fz card list --state golden", `fz card list --search "first aid"`,
			"fz card list --board Home --column Next --tag waiting", "fz card list --unassigned --created thisweek --sort oldest"),
		Args: args(cobra.NoArgs),
		RunE: action(cardList),
	}
	list.Flags().String("board", "", "board name or id")
	list.Flags().String("state", "", "Fizzy's index filter (default all)")
	list.Flags().String("search", "", "search terms")
	list.Flags().String("column", "", "column name or id on --board")
	list.Flags().String("tag", "", "tag name")
	list.Flags().String("assignee", "", "assigned to this user: id, name or email")
	list.Flags().Bool("unassigned", false, "only cards with no assignee")
	list.Flags().String("sort", "", "latest (default), newest or oldest")
	list.Flags().String("created", "", "created in a window: "+strings.Join(cardWindows, ", "))
	list.Flags().String("closed", "", "closed in a window: "+strings.Join(cardWindows, ", "))

	show := &cobra.Command{
		Use:     "show NUMBER",
		Short:   "show one card with its steps",
		Long:    "Print a card's title, board, link, description and steps.",
		Example: examples("fz card show 87", "fz --json card show 87"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardShow),
	}

	create := &cobra.Command{
		Use:     "create BOARD TITLE",
		Short:   "create a card",
		Long:    "Create a card on a board. It lands in Maybe? unless --column names another lane. An empty or blank title exits 2 and sends nothing.",
		Example: examples(`fz card create Home "Fix the gate"`, `fz card create Home "Fix the gate" --column Next --tag buy`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(cardCreate),
	}
	create.Flags().String("description", "", "description as Markdown")
	create.Flags().String("description-file", "", "read the Markdown description from a file")
	create.Flags().String("column", "", "column name, or maybe (default)")
	create.Flags().StringArray("tag", nil, "tag to add; repeat for more")

	move := &cobra.Command{
		Use:     "move NUMBER",
		Short:   "move a card to a column or another board",
		Long:    "Move a card. --to takes a column name, or maybe, not-now or done. --board moves it to another board.",
		Example: examples("fz card move 87 --to Now", "fz card move 87 --to done", "fz card move 87 --board Home --to Next"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardMove),
	}
	move.Flags().String("to", "", "column name, or maybe / not-now / done")
	move.Flags().String("board", "", "board name or id to move the card to")

	reopen := &cobra.Command{
		Use:     "reopen NUMBER",
		Short:   "reopen a done card",
		Long:    "Move a closed card out of Done.",
		Example: examples("fz card reopen 87"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardReopen),
	}

	golden := &cobra.Command{
		Use:     "golden NUMBER",
		Short:   "mark a card golden, or --off",
		Long:    "Golden cards are a board's current priorities.",
		Example: examples("fz card golden 87", "fz card golden 87 --off"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardGolden),
	}
	golden.Flags().Bool("off", false, "remove golden")

	tag := &cobra.Command{
		Use:     "tag NUMBER TAG",
		Short:   "add a tag, or --remove",
		Long:    "Add or remove a tag. Fizzy's tag call toggles, so fz reads the card first and only sends a change when one is needed.",
		Example: examples("fz card tag 87 waiting", "fz card tag 87 waiting --remove"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(cardTag),
	}
	tag.Flags().Bool("remove", false, "remove the tag")

	del := &cobra.Command{
		Use:     "delete NUMBER",
		Short:   "delete a card (needs --yes)",
		Long:    "Delete a card for good. Prints the card first and refuses without --yes.",
		Example: examples("fz card delete 87 --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardDelete),
	}
	del.Flags().Bool("yes", false, "really delete")

	g.AddCommand(list, show, create, move, reopen, golden, tag, del)
}

func cardList(c *cmdContext, _ []string) error {
	fl := c.cmd.Flags()
	state, _ := fl.GetString("state")
	if fl.Changed("state") {
		if err := choice("state", state, cardStates); err != nil {
			return err
		}
	}
	// fixed lists the flags that take a fixed set of values, with their query keys.
	fixed := []struct {
		flag, key string
		allowed   []string
	}{{"sort", "sorted_by", cardSorts}, {"created", "creation", cardWindows}, {"closed", "closure", cardWindows}}
	for _, w := range fixed {
		if v, _ := fl.GetString(w.flag); fl.Changed(w.flag) {
			if err := choice(w.flag, v, w.allowed); err != nil {
				return err
			}
		}
	}
	board, _ := fl.GetString("board")
	column, _ := fl.GetString("column")
	assignee, _ := fl.GetString("assignee")
	unassigned, _ := fl.GetBool("unassigned")
	if column != "" && board == "" {
		return usagef("--column needs --board")
	}
	if assignee != "" && unassigned {
		return usagef("use --assignee or --unassigned, not both")
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	boardID := ""
	if board != "" {
		b, err := fz.Board(board)
		if err != nil {
			return err
		}
		boardID = b.ID
	}
	var filters []fizzy.Param
	if fl.Changed("state") {
		filters = append(filters, fizzy.Param{Key: "indexed_by", Value: state})
	}
	if fl.Changed("search") {
		s, _ := fl.GetString("search")
		filters = append(filters, fizzy.Param{Key: "terms[]", Value: s})
	}
	if column != "" {
		col, err := fz.Column(boardID, column)
		if err != nil {
			return err
		}
		filters = append(filters, fizzy.Param{Key: "column_ids[]", Value: col.ID})
	}
	if name, _ := fl.GetString("tag"); name != "" {
		t, err := fz.FindTag(name)
		if err != nil {
			return err
		}
		filters = append(filters, fizzy.Param{Key: "tag_ids[]", Value: t.ID})
	}
	if assignee != "" {
		u, err := fz.MatchUser(assignee, false)
		if err != nil {
			return err
		}
		filters = append(filters, fizzy.Param{Key: "assignee_ids[]", Value: u.ID})
	}
	if unassigned {
		filters = append(filters, fizzy.Param{Key: "assignment_status", Value: "unassigned"})
	}
	for _, w := range fixed {
		if v, _ := fl.GetString(w.flag); fl.Changed(w.flag) {
			filters = append(filters, fizzy.Param{Key: w.key, Value: v})
		}
	}
	cards, err := fz.Cards(boardID, filters)
	if err != nil {
		return err
	}
	lines := make([]string, len(cards))
	for i, card := range cards {
		lines[i] = cardLine(card)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no cards)"
	}
	return emit(c.cmd, rawList(cards), text)
}

func cardShow(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	card, err := fz.Card(n)
	if err != nil {
		return err
	}
	if flagValue(c.cmd, "json") == "true" {
		return printJSON(c.out(), card.Raw())
	}
	w := c.out()
	fmt.Fprintln(w, cardLine(card))
	fmt.Fprintf(w, "board: %s   url: %s\n", card.Board.Name, rawString(card.Raw(), "url"))
	if card.Description != "" {
		fmt.Fprintln(w, "\n"+pyStrip(card.Description))
	}
	for _, s := range card.Steps {
		fmt.Fprintf(w, "  [%s] %s\n", tick(s.Completed), s.Content)
	}
	return nil
}

// rawString returns a string field of a JSON object as Python's f-string
// prints c.get(key): the text, or None when the key is missing or null.
func rawString(raw json.RawMessage, key string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return "None"
	}
	switch v := m[key].(type) {
	case nil:
		return "None"
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func tick(done bool) string {
	if done {
		return "x"
	}
	return " "
}

// isMaybe reports whether a column name means Maybe?, the way the Python
// handlers test it: lower case, trailing ? removed.
func isMaybe(column string) bool {
	return strings.TrimRight(strings.ToLower(column), "?") == "maybe"
}

func cardCreate(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	if err := checkName("the card title", a[1]); err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	desc, _ := fl.GetString("description")
	if file, _ := fl.GetString("description-file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		desc = strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n")
	}
	html := ""
	if desc != "" {
		html = markdown.ToHTML(desc)
	}
	n, err := fz.CreateCard(b.ID, a[1], html, "")
	if err != nil {
		return err
	}
	if n != 0 {
		if col, _ := fl.GetString("column"); col != "" && !isMaybe(col) {
			if err := fz.MoveCard(n, col, b.ID); err != nil {
				return err
			}
		}
		tags, _ := fl.GetStringArray("tag")
		for _, t := range tags {
			if _, err := fz.AddTag(n, t); err != nil {
				return err
			}
		}
	}
	text := "dry-run: card not created"
	var number any
	if n != 0 {
		text = fmt.Sprintf("card #%d created on %s", n, b.Name)
		number = n
	}
	return emit(c.cmd, obj{{"number", number}}, text)
}

func cardMove(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if name, _ := fl.GetString("board"); name != "" {
		b, err := fz.Board(name)
		if err != nil {
			return err
		}
		if err := fz.MoveCardToBoard(n, b.ID); err != nil {
			return err
		}
	}
	if to, _ := fl.GetString("to"); to != "" {
		if err := fz.MoveCard(n, to, ""); err != nil {
			return err
		}
	}
	return emit(c.cmd, obj{{"number", n}}, fmt.Sprintf("card #%d moved", n))
}

func cardReopen(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.ReopenCard(n); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"number", n}}, fmt.Sprintf("card #%d reopened", n))
}

func cardGolden(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	off, _ := c.cmd.Flags().GetBool("off")
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.SetGolden(n, !off); err != nil {
		return err
	}
	state := "on"
	if off {
		state = "off"
	}
	return emit(c.cmd, obj{{"number", n}, {"golden", !off}}, fmt.Sprintf("card #%d golden=%s", n, state))
}

func cardTag(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	remove, _ := c.cmd.Flags().GetBool("remove")
	fz, err := c.api()
	if err != nil {
		return err
	}
	tag := a[1]
	var changed bool
	verb, already := "added", "present"
	if remove {
		changed, err = fz.RemoveTag(n, tag)
		verb, already = "removed", "absent"
	} else {
		changed, err = fz.AddTag(n, tag)
	}
	if err != nil {
		return err
	}
	text := fmt.Sprintf("tag %s already %s", tag, already)
	if changed {
		text = fmt.Sprintf("tag %s %s", tag, verb)
	}
	return emit(c.cmd, obj{{"changed", changed}}, text)
}

func cardDelete(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	card, err := fz.Card(n)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.errw(), "about to delete: %s (board %s)\n", cardLine(card), card.Board.Name)
	if !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	if err := fz.DeleteCard(n); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"deleted", n}}, fmt.Sprintf("card #%d deleted", n))
}
