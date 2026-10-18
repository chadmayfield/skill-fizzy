package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	// The embedded zone database lets user timezone check names on a system
	// without zoneinfo files.
	_ "time/tzdata"

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
		Long:    "Set your timezone, which Fizzy uses for times in notification emails. NAME is an IANA name such as America/Denver, spelled with its exact case. fz checks it before sending.",
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
	if !validTimezone(name) {
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

// zoneDirs are the system zoneinfo directories time.LoadLocation reads, in
// order, before its embedded copy (platformZoneSources in Go's time package).
var zoneDirs = []string{"/usr/share/zoneinfo", "/usr/share/lib/zoneinfo", "/usr/lib/locale/TZ", "/etc/zoneinfo"}

// validTimezone reports whether name is an IANA zone name spelled exactly.
// LoadLocation accepts "" (UTC), "Local" and the posixrules and Factory
// files, which the server would not know. On a case-insensitive disk it
// also loads "america/denver", so the case of each part of the path is
// checked in the first zone directory that holds it. A name found in no
// directory came from the embedded copy, which matches case exactly.
func validTimezone(name string) bool {
	switch name {
	case "", "Local", "posixrules", "Factory":
		return false
	}
	loc, err := time.LoadLocation(name)
	if err != nil || loc.String() != name {
		return false
	}
	for _, dir := range zoneDirs {
		if exact, found := zoneFileCase(dir, name); found {
			return exact
		}
	}
	return true
}

// zoneFileCase looks for name under dir one path part at a time. found
// reports whether it exists in any case; exact whether every part matches
// case too.
func zoneFileCase(dir, name string) (exact, found bool) {
	cur := dir
	for _, part := range strings.Split(name, "/") {
		entries, err := os.ReadDir(cur)
		if err != nil {
			return false, false
		}
		match := ""
		for _, e := range entries {
			if e.Name() == part {
				match = part
				break
			}
			if match == "" && strings.EqualFold(e.Name(), part) {
				match = e.Name()
			}
		}
		if match == "" {
			return false, false
		}
		if match != part {
			return false, true
		}
		cur = filepath.Join(cur, part)
	}
	return true, true
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
