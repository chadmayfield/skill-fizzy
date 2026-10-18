package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addPeopleCommands) }

func addPeopleCommands(root *cobra.Command) {
	// "fz tag 87 buy" is rewritten to card tag before cobra sees it, so this
	// group only ever receives its subcommands.
	tags := &cobra.Command{
		Use:   "tag",
		Short: "list the account's tags",
		Long:  "Work with the account's tags. To tag a card, use fz card tag.",
	}
	tags.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "list every tag",
		Long:    "List every tag on the account, sorted by title, with its id.",
		Example: examples("fz tag list", "fz --json tag list"),
		Args:    args(cobra.NoArgs),
		RunE:    action(tagList),
	})

	users := &cobra.Command{
		Use:   "user",
		Short: "list, show and rename users",
		Long:  "Work with the people on the account.",
	}
	edit := &cobra.Command{
		Use:     "edit ID",
		Short:   "rename a user (needs --yes)",
		Long:    "Change a user's display name. Prints the current and new name, and refuses without --yes.",
		Example: examples(`fz user edit 03abc123def456ghi789jkl0mn --name "Gate Keeper" --yes`),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(userEdit),
	}
	edit.Flags().String("name", "", "the new display name")
	edit.Flags().Bool("yes", false, "really rename")
	users.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "list active users",
		Long:    "List the account's active users with their id, name, email and role.",
		Example: examples("fz user list", "fz --json user list"),
		Args:    args(cobra.NoArgs),
		RunE:    action(userList),
	}, &cobra.Command{
		Use:     "show ID",
		Short:   "show one user",
		Long:    "Print a user's id, name, email, role and link.",
		Example: examples("fz user show 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(userShow),
	}, edit, &cobra.Command{
		Use:     "timezone NAME",
		Short:   "set your timezone",
		Long:    "Set your timezone, which Fizzy uses for times in notification emails. NAME is an IANA name such as America/Denver, checked before it is sent.",
		Example: examples("fz user timezone America/Denver"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(userTimezone),
	})

	whoami := &cobra.Command{
		Use:     "whoami",
		Short:   "show who the token belongs to",
		Long:    "List every account the token can reach, with your user on each. A * marks the configured account.",
		Example: examples("fz whoami", "fz --json whoami"),
		Args:    args(cobra.NoArgs),
		RunE:    action(whoami),
	}
	root.AddCommand(tags, users, whoami)
}

func userLine(u fizzy.User) string {
	line := fmt.Sprintf("%s  %s <%s> %s", u.ID, u.Name, u.EmailAddress, u.Role)
	if !u.Active {
		line += " (inactive)"
	}
	return line
}

// listText joins one line per item, or the empty text when there are none.
func listText[T any](items []T, line func(T) string, empty string) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = line(it)
	}
	if len(out) == 0 {
		return empty
	}
	return strings.Join(out, "\n")
}

func tagList(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	tags, err := fz.Tags()
	if err != nil {
		return err
	}
	text := listText(tags, func(t fizzy.Tag) string { return t.ID + "  #" + t.Title }, "(no tags)")
	return emit(c.cmd, rawList(tags), text)
}

func userList(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	users, err := fz.Users()
	if err != nil {
		return err
	}
	return emit(c.cmd, rawList(users), listText(users, userLine, "(no users)"))
}

func userShow(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	u, err := fz.User(a[0])
	if err != nil {
		return err
	}
	return emit(c.cmd, u.Raw(), userLine(u)+"\nurl: "+u.URL)
}

func userEdit(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	name, _ := fl.GetString("name")
	name = strings.TrimSpace(name)
	if name == "" {
		return usagef("user edit needs --name")
	}
	yes, _ := fl.GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	u, err := fz.User(a[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(c.errw(), "about to rename user %s from %s to %s\n", a[0], u.Name, name)
	if !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	updated, err := fz.RenameUser(a[0], name)
	if err != nil {
		return err
	}
	var data any = obj{{"id", a[0]}, {"name", name}}
	if updated.Raw() != nil {
		data = updated.Raw()
	}
	return emit(c.cmd, data, fmt.Sprintf("user %s renamed to %s", a[0], name))
}

func userTimezone(c *cmdContext, a []string) error {
	name := a[0]
	// LoadLocation takes "" as UTC and "Local" as the machine's zone, which
	// are not IANA names the server would know.
	if name == "" || name == "Local" {
		return usagef("unknown timezone %s", pyRepr(name))
	}
	if _, err := time.LoadLocation(name); err != nil {
		return usagef("unknown timezone %s", pyRepr(name))
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.SetTimezone(name); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"timezone_name", name}}, "timezone set to "+name)
}

func whoami(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	id, err := fz.Identity()
	if err != nil {
		return err
	}
	acct := fz.C.Account()
	text := listText(id.Accounts, func(a fizzy.IdentityAccount) string {
		mark, slug := " ", strings.Trim(a.Slug, "/")
		if slug == acct {
			mark = "*"
		}
		u := a.User
		return fmt.Sprintf("%s %s  %s  %s <%s> %s", mark, slug, a.Name, u.Name, u.EmailAddress, u.Role)
	}, "(no accounts)")
	return emit(c.cmd, id.Raw(), text)
}
