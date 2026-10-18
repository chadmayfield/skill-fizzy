package cli

import (
	"fmt"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addCardExtraCommands) }

// addCardExtraCommands adds assign, watch, pin, boost and image under the
// card group, which cards.go registers first.
func addCardExtraCommands(root *cobra.Command) {
	assign := &cobra.Command{
		Use:   "assign NUMBER USER",
		Short: "assign a user to a card",
		Long: "Assign a user to a card. USER is a user id, a name, or an email address. fz tries an exact id, then an exact name, then a name ignoring case, then an email ignoring case. If two users have the name, fz refuses and exits 2. " +
			"fz prints which user it matched and how. Fizzy's assign call toggles, so fz reads the card first and only sends a change when one is needed.",
		Example: examples("fz card assign 87 03abc123def456ghi789jkl0mn", `fz card assign 87 "Gate Keeper"`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(func(c *cmdContext, a []string) error { return cardAssign(c, a, true) }),
	}
	unassign := &cobra.Command{
		Use:     "unassign NUMBER USER",
		Short:   "remove a user from a card",
		Long:    "Remove a user from a card. USER is matched as in card assign. fz only sends a change when the user is assigned.",
		Example: examples(`fz card unassign 87 "Gate Keeper"`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(func(c *cmdContext, a []string) error { return cardAssign(c, a, false) }),
	}

	watch := simpleCardCommand("watch", "watch a card", "Subscribe to a card's notifications.",
		func(fz *fizzy.Fizzy, n int) error { return fz.Watch(n) }, "watched", "watching", true)
	unwatch := simpleCardCommand("unwatch", "stop watching a card", "Unsubscribe from a card's notifications.",
		func(fz *fizzy.Fizzy, n int) error { return fz.Unwatch(n) }, "unwatched", "watching", false)
	pin := simpleCardCommand("pin", "pin a card", "Pin a card for yourself. card pins lists your pinned cards.",
		func(fz *fizzy.Fizzy, n int) error { return fz.Pin(n) }, "pinned", "pinned", true)
	unpin := simpleCardCommand("unpin", "unpin a card", "Unpin a card for yourself.",
		func(fz *fizzy.Fizzy, n int) error { return fz.Unpin(n) }, "unpinned", "pinned", false)

	pins := &cobra.Command{
		Use:     "pins",
		Short:   "list your pinned cards",
		Long:    "List the cards you have pinned. Fizzy returns at most 100 and does not page them.",
		Example: examples("fz card pins", "fz --json card pins"),
		Args:    args(cobra.NoArgs),
		RunE:    action(cardPins),
	}

	boost := &cobra.Command{
		Use:   "boost NUMBER TEXT",
		Short: "add a boost to a card",
		Long: fmt.Sprintf("Add a boost, a short reaction, to a card. TEXT is at most %d characters. "+
			"fz counts characters as Fizzy does, one per Unicode code point, and refuses longer text before sending.", fizzy.MaxBoostChars),
		Example: examples(`fz card boost 87 "Nice hinge"`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(cardBoost),
	}
	boosts := &cobra.Command{
		Use:     "boosts NUMBER",
		Short:   "list a card's boosts",
		Long:    "List a card's boosts, one per line: the boost id, its text and who added it. Use the id with card unboost.",
		Example: examples("fz card boosts 87"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardBoosts),
	}
	unboost := &cobra.Command{
		Use:     "unboost NUMBER BOOST_ID",
		Short:   "remove a boost from a card",
		Long:    "Remove a boost from a card. card boosts prints the ids. Fizzy only lets you remove your own boosts.",
		Example: examples("fz card unboost 87 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(cardUnboost),
	}

	image := &cobra.Command{
		Use:     "image NUMBER",
		Short:   "remove a card's header image (needs --yes)",
		Long:    "Remove a card's header image. fz can only remove the image, so --remove is required. It prints the card first and refuses without --yes.",
		Example: examples("fz card image 87 --remove --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(cardImage),
	}
	image.Flags().Bool("remove", false, "remove the header image")
	image.Flags().Bool("yes", false, "really remove it")

	for _, c := range root.Commands() {
		if c.Name() == "card" {
			c.AddCommand(assign, unassign, watch, unwatch, pin, unpin, pins, boost, boosts, unboost, image)
		}
	}
}

// simpleCardCommand builds a command that takes a card number and sends one
// request. It prints "card #N <done>", or {number, <key>: <value>} as JSON.
func simpleCardCommand(name, short, long string, send func(*fizzy.Fizzy, int) error, done, key string, value bool) *cobra.Command {
	return &cobra.Command{
		Use:     name + " NUMBER",
		Short:   short,
		Long:    long,
		Example: examples("fz card " + name + " 87"),
		Args:    args(cobra.ExactArgs(1)),
		RunE: action(func(c *cmdContext, a []string) error {
			n, err := cardNumber(a[0])
			if err != nil {
				return err
			}
			fz, err := c.api()
			if err != nil {
				return err
			}
			if err := send(fz, n); err != nil {
				return err
			}
			return emit(c.cmd, c.dry(obj{{"number", n}, {key, value}}), c.wrote(fmt.Sprintf("card #%d %s", n, done), fmt.Sprintf("%s card #%d", name, n)))
		}),
	}
}

func cardAssign(c *cmdContext, a []string, assign bool) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	op, verb, already, would := fz.Unassign, "unassigned", "not assigned", "unassign %s from card #%d"
	if assign {
		op, verb, already, would = fz.Assign, "assigned", "already assigned", "assign %s to card #%d"
	}
	u, changed, err := op(n, a[1])
	if err != nil {
		return err
	}
	who := fmt.Sprintf("%s (%s, matched by %s)", u.Name, u.ID, u.By)
	text := fmt.Sprintf("card #%d: %s %s", n, who, already)
	if changed {
		text = c.wrote(fmt.Sprintf("card #%d: %s %s", n, verb, who), fmt.Sprintf(would, who, n))
	}
	return emit(c.cmd, c.dry(obj{{"number", n}, {"user_id", u.ID}, {"name", u.Name}, {"matched_by", u.By}, {"changed", changed}}), text)
}

func cardPins(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	cards, err := fz.Pins()
	if err != nil {
		return err
	}
	lines := make([]string, len(cards))
	for i, card := range cards {
		lines[i] = cardLine(card)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no pinned cards)"
	}
	return emit(c.cmd, rawList(cards), text)
}

func cardBoost(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	if err := fizzy.CheckBoost(a[1]); err != nil {
		return usageError{err}
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	raw, err := fz.AddBoost(n, a[1])
	if err != nil {
		return err
	}
	if c.dryRun {
		return emit(c.cmd, c.dry(obj{{"number", n}, {"id", nil}}), c.wrote("", fmt.Sprintf("add a boost to card #%d", n)))
	}
	if raw == nil {
		return emit(c.cmd, obj{{"number", n}, {"id", nil}}, fmt.Sprintf("boost added to card #%d", n))
	}
	return emit(c.cmd, raw, fmt.Sprintf("boost %s added to card #%d", rawString(raw, "id"), n))
}

func cardBoosts(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	bs, err := fz.Boosts(n)
	if err != nil {
		return err
	}
	lines := make([]string, len(bs))
	for i, b := range bs {
		lines[i] = fmt.Sprintf("%s  %s  (%s)", b.ID, b.Content, b.Reacter.Name)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no boosts)"
	}
	return emit(c.cmd, rawList(bs), text)
}

func cardUnboost(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.RemoveBoost(n, a[1]); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"number", n}, {"removed", a[1]}}), c.wrote(fmt.Sprintf("boost %s removed from card #%d", a[1], n), fmt.Sprintf("remove boost %s from card #%d", a[1], n)))
}

func cardImage(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fl := c.cmd.Flags()
	if remove, _ := fl.GetBool("remove"); !remove {
		return usagef("card image only removes an image; add --remove")
	}
	yes, _ := fl.GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	card, err := fz.Card(n)
	if err != nil {
		return err
	}
	if rawString(card.Raw(), "image_url") == "None" {
		return emit(c.cmd, c.dry(obj{{"number", n}, {"removed", false}}), fmt.Sprintf("card #%d has no image", n))
	}
	fmt.Fprintf(c.errw(), "about to remove the image from: %s (board %s)\n", cardLine(card), card.Board.Name)
	if !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	if err := fz.RemoveImage(n); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"number", n}, {"removed", true}}), c.wrote(fmt.Sprintf("card #%d image removed", n), fmt.Sprintf("remove the image from card #%d", n)))
}
