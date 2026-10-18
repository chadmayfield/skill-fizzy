package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/markdown"
	"github.com/spf13/cobra"
)

func init() { register(addBoardExtraCommands) }

func addBoardExtraCommands(root *cobra.Command) {
	g := subgroup(root, "board")
	show := &cobra.Command{
		Use:     "show BOARD",
		Short:   "show one board",
		Long:    "Print a board's id, name, auto-postpone period, who can access it, who created it and its public link, if it has one.",
		Example: examples("fz board show Home", "fz --json board show Home"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardShow),
	}
	edit := &cobra.Command{
		Use:   "edit BOARD",
		Short: "rename a board or change its postpone period",
		Long: "Change a board's name, its auto-postpone period or its public description. " +
			"The period must be one of 3, 7, 11, 30, 90 or 365 days. " +
			"The public description is Markdown, shown on the public page once the board is published. " +
			"An empty or blank --name exits 2 and sends nothing.",
		Example: examples(`fz board edit Home --name "Gate and hinge"`, "fz board edit Home --postpone 90", `fz board edit Home --public-description "Notes on the gate"`),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardEdit),
	}
	edit.Flags().String("name", "", "new name")
	edit.Flags().Int("postpone", 0, "days of no activity before cards move to Not now")
	edit.Flags().String("public-description", "", "description for the public page, as Markdown")
	del := &cobra.Command{
		Use:     "delete BOARD",
		Short:   "delete a board (needs --yes)",
		Long:    "Delete a board and its cards for good. Prints the board name, its card counts under Fizzy's all, closed and not_now filters, and its columns first, and refuses without --yes.",
		Example: examples("fz board delete Home --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardDelete),
	}
	del.Flags().Bool("yes", false, "really delete")
	publish := &cobra.Command{
		Use:   "publish BOARD",
		Short: "publish a board to a public link (needs --yes)",
		Long: "Publish a board. Anyone with the public link can read it without logging in. Prints the link. Refuses without --yes. " +
			"A board that is already published prints its link and that no change is needed, sends nothing and exits 0.",
		Example: examples("fz board publish Home --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardPublish),
	}
	publish.Flags().Bool("yes", false, "really publish")
	unpublish := &cobra.Command{
		Use:     "unpublish BOARD",
		Short:   "remove a board's public link",
		Long:    "Remove public access to a board. This needs no --yes, because it only closes access.",
		Example: examples("fz board unpublish Home"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardUnpublish),
	}
	access := &cobra.Command{
		Use:   "access BOARD",
		Short: "list or change who can access a board",
		Long: "Without a flag, list who can access a board. " +
			"--all opens the board to every user in the account. " +
			"--users USER,USER limits it to those users, and the list replaces the whole set, so anyone not named loses access. " +
			"Each USER is a user id, a name or an email, matched as card assign matches one. " +
			"fz refuses a USER that matches nobody or more than one user, and sends nothing. " +
			"Either change prints what it would do, naming each user with their id, and refuses without --yes. " +
			"The --users preview lists who keeps access, who gains it and who loses it, or says the current list is unknown when Fizzy's reply has none. " +
			"--all on a board that every user can already reach, or --users with the board's current list, says no change is needed and sends nothing, without --yes.",
		Example: examples("fz board access Home", "fz board access Home --all --yes", "fz board access Home --users 03abc123def456ghi789jkl0mn --yes", "fz board access Home --users Alex,Sam --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(boardAccess),
	}
	access.Flags().Bool("all", false, "open the board to every user in the account")
	access.Flags().String("users", "", "comma-separated user ids, names or emails; replaces the whole list")
	access.Flags().Bool("yes", false, "really change access")
	g.AddCommand(show, edit, del, publish, unpublish, access)
}

func boardShow(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	d, err := fz.BoardDetail(b.ID)
	if err != nil {
		return err
	}
	postpone := "postpone off"
	if d.AutoPostponePeriodInDays != nil {
		postpone = "postpone " + strconv.Itoa(*d.AutoPostponePeriodInDays) + "d"
	}
	lines := []string{
		line(fmt.Sprintf("%s  %s  (%s)", d.ID, d.Name, postpone)),
		"access: " + accessSummary(d),
	}
	if d.Creator.Name != "" {
		lines = append(lines, "creator: "+line(d.Creator.Name))
	}
	if d.PublicURL != "" {
		lines = append(lines, "public: "+line(d.PublicURL))
	}
	return emit(c.cmd, d.Raw(), strings.Join(lines, "\n"))
}

func accessSummary(d fizzy.BoardInfo) string {
	if d.AllAccess {
		return "all users"
	}
	// A reply without user_ids does not say who has access.
	if d.UserIDs == nil {
		return "selected users"
	}
	if len(d.UserIDs) == 1 {
		return "1 user"
	}
	return fmt.Sprintf("%d users", len(d.UserIDs))
}

func boardEdit(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	days, err := postponeDays(c)
	if err != nil {
		return err
	}
	var fields fizzy.BoardFields
	var changed []string
	if fl.Changed("name") {
		name, _ := fl.GetString("name")
		name = strings.TrimSpace(name)
		if err := checkName("--name", name); err != nil {
			return err
		}
		fields.Name = &name
		changed = append(changed, "name")
	}
	if fl.Changed("postpone") {
		fields.PostponeDays = &days
		changed = append(changed, "postpone")
	}
	if fl.Changed("public-description") {
		md, _ := fl.GetString("public-description")
		html := markdown.ToHTML(md)
		fields.PublicDescription = &html
		changed = append(changed, "public-description")
	}
	if len(changed) == 0 {
		return usagef("give --name, --postpone or --public-description")
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	if err := fz.UpdateBoard(b.ID, fields); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"changed", changed}}),
		c.wrote(fmt.Sprintf("board %s updated: %s", line(b.Name), strings.Join(changed, ", ")),
			fmt.Sprintf("update board %s: %s", line(b.Name), strings.Join(changed, ", "))))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func boardDelete(c *cmdContext, a []string) error {
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	var counts [3]int
	for i, state := range []string{"all", "closed", "not_now"} {
		cards, err := fz.Cards(b.ID, []fizzy.Param{{Key: "indexed_by", Value: state}})
		if err != nil {
			return err
		}
		counts[i] = len(cards)
	}
	cols, err := fz.Columns(b.ID)
	if err != nil {
		return err
	}
	names := make([]string, len(cols))
	for i, col := range cols {
		names[i] = line(col.Name)
	}
	listed := "none"
	if len(names) > 0 {
		listed = strings.Join(names, ", ")
	}
	// Fizzy's API docs list "all" as the default filter but do not say
	// which cards it holds, so the count carries the filter's name.
	fmt.Fprintf(c.errw(), "about to delete board %s (%s): cards by Fizzy's filters: all %d, closed %d, not now %d; custom columns: %s\n", line(b.Name), line(b.ID), counts[0], counts[1], counts[2], listed)
	if !yes {
		return refuse(c)
	}
	if err := fz.DeleteBoard(b.ID); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"deleted", b.ID}}), c.wrote(fmt.Sprintf("board %s deleted", line(b.Name)), "delete board "+line(b.Name)))
}

func boardPublish(c *cmdContext, a []string) error {
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	d, err := fz.BoardDetail(b.ID)
	if err != nil {
		return err
	}
	if d.PublicURL != "" {
		return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"public_url", d.PublicURL}, {"changed", false}}),
			noChange(c, fmt.Sprintf("board %s is already published: %s; no change needed", line(b.Name), line(d.PublicURL))))
	}
	fmt.Fprintf(c.errw(), "about to publish board %s (%s): anyone with the link can read it without logging in\n", line(b.Name), line(b.ID))
	if !yes {
		return refuse(c)
	}
	link, err := fz.PublishBoard(b.ID)
	if err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"public_url", orNull(link)}}),
		c.wrote(fmt.Sprintf("board %s published: %s", line(b.Name), line(orNone(link))), "publish board "+line(b.Name)))
}

func boardUnpublish(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	if err := fz.UnpublishBoard(b.ID); err != nil {
		return err
	}
	return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"published", false}}), c.wrote(fmt.Sprintf("board %s unpublished", line(b.Name)), "unpublish board "+line(b.Name)))
}

func boardAccess(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	all, _ := fl.GetBool("all")
	usersSpec, _ := fl.GetString("users")
	yes, _ := fl.GetBool("yes")
	changing := all || fl.Changed("users")
	switch {
	case all && fl.Changed("users"):
		return usagef("--all and --users cannot be used together")
	case yes && !changing:
		return usagef("--yes only goes with --all or --users")
	}
	var specs []string
	if fl.Changed("users") {
		for _, spec := range strings.Split(usersSpec, ",") {
			if spec = strings.TrimSpace(spec); spec != "" {
				specs = append(specs, spec)
			}
		}
		if len(specs) == 0 {
			return usagef("--users needs at least one user; use --all to open the board to everyone")
		}
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	b, err := fz.Board(a[0])
	if err != nil {
		return err
	}
	if !changing {
		return boardAccessList(c, fz, b)
	}
	var ids, named []string
	var users []fizzy.User
	if !all {
		users, err = fz.Users()
		if err != nil {
			return err
		}
		matched, err := fizzy.MatchUsersIn(users, specs)
		if err != nil {
			return err
		}
		for _, u := range matched {
			ids = append(ids, u.ID)
			named = append(named, line(fmt.Sprintf("%s (%s)", u.Name, u.ID)))
		}
	}
	d, err := fz.BoardDetail(b.ID)
	if err != nil {
		return err
	}
	if all && d.AllAccess {
		return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"all_access", true}, {"changed", false}}),
			noChange(c, fmt.Sprintf("board %s already gives all users access; no change needed", line(b.Name))))
	}
	if !all && !d.AllAccess && d.UserIDs != nil && sameIDs(ids, d.UserIDs) {
		return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"all_access", false}, {"user_ids", ids}, {"changed", false}}),
			noChange(c, fmt.Sprintf("board %s already gives access to exactly these users; no change needed", line(b.Name))))
	}
	w := c.errw()
	fmt.Fprintf(w, "about to change access to board %s (%s)\n  now: %s\n", line(b.Name), line(b.ID), accessSummary(d))
	if all {
		fmt.Fprintln(w, "  new: all users")
	} else {
		fmt.Fprintf(w, "  new: only these users: %s\n", strings.Join(named, ", "))
		accessChanges(w, d, users, ids)
		fmt.Fprintln(w, "  --users replaces the whole list; anyone not named loses access")
	}
	if !yes {
		return refuse(c)
	}
	if err := fz.SetBoardAccess(b.ID, all, ids); err != nil {
		return err
	}
	to := "all users"
	if !all {
		to = plural(len(ids), "user", "users")
	}
	text := c.wrote(fmt.Sprintf("board %s access set to %s", line(b.Name), to), fmt.Sprintf("set board %s access to %s", line(b.Name), to))
	return emit(c.cmd, c.dry(obj{{"id", b.ID}, {"all_access", all}, {"user_ids", ids}}), text)
}

// sameIDs reports whether a and b hold the same ids, in any order and
// ignoring repeats.
func sameIDs(a, b []string) bool {
	for _, id := range a {
		if !slices.Contains(b, id) {
			return false
		}
	}
	for _, id := range b {
		if !slices.Contains(a, id) {
			return false
		}
	}
	return true
}

// accessChanges prints who keeps, gains and loses access when the board's
// list becomes ids. On an all-access board every active user has access now.
// A reply without user_ids does not say who has access, so it says that.
func accessChanges(w io.Writer, d fizzy.BoardInfo, users []fizzy.User, ids []string) {
	names := map[string]string{}
	var current []string
	for _, u := range users {
		names[u.ID] = u.Name
		if d.AllAccess {
			current = append(current, u.ID)
		}
	}
	if !d.AllAccess {
		if d.UserIDs == nil {
			fmt.Fprintln(w, "  the current list is unknown, so fz cannot say who keeps or loses access")
			return
		}
		current = d.UserIDs
	}
	label := func(id string) string {
		if n := names[id]; n != "" {
			return line(fmt.Sprintf("%s (%s)", n, id))
		}
		return line(id)
	}
	var keeps, gains, loses []string
	for _, id := range ids {
		if slices.Contains(current, id) {
			keeps = append(keeps, label(id))
		} else {
			gains = append(gains, label(id))
		}
	}
	for _, id := range current {
		if !slices.Contains(ids, id) {
			loses = append(loses, label(id))
		}
	}
	for _, row := range []struct {
		what string
		who  []string
	}{{"keeps", keeps}, {"gains", gains}, {"loses", loses}} {
		list := "none"
		if len(row.who) > 0 {
			list = strings.Join(row.who, ", ")
		}
		fmt.Fprintf(w, "  %s access: %s\n", row.what, list)
	}
}

func boardAccessList(c *cmdContext, fz *fizzy.Fizzy, b fizzy.Board) error {
	acc, err := fz.BoardAccesses(b.ID)
	if err != nil {
		return err
	}
	lines := []string{"all access: " + map[bool]string{true: "yes", false: "no"}[acc.AllAccess]}
	for _, u := range acc.Users {
		state := "no access"
		switch {
		case u.Involvement == "access_only":
			state = "access only"
		case u.Involvement != "":
			state = u.Involvement
		case u.HasAccess:
			state = "access"
		}
		lines = append(lines, line(fmt.Sprintf("%s  (%s)  %s", u.Name, u.Role, state)))
	}
	return emit(c.cmd, obj{{"board_id", acc.BoardID}, {"all_access", acc.AllAccess}, {"users", rawList(acc.Users)}},
		strings.Join(lines, "\n"))
}
