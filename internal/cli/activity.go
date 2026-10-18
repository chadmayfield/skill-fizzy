package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addActivityCommands) }

func addActivityCommands(root *cobra.Command) {
	act := &cobra.Command{
		Use:   "activity",
		Short: "list the account's activity feed",
		Long:  "Work with the account's activity feed: cards published, moved, assigned, closed and commented on.",
	}
	list := &cobra.Command{
		Use:     "list",
		Short:   "list activity, newest first (every page)",
		Long:    "List activity, newest first, with the time, action, what it happened to and a short summary of the details. --creator takes a user id, an exact name or an email.",
		Example: examples("fz activity list", "fz activity list --board Home", "fz activity list --board Home --creator 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.NoArgs),
		RunE:    action(activityList),
	}
	list.Flags().String("board", "", "board name or id")
	list.Flags().String("creator", "", "user id, name or email")
	act.AddCommand(list)

	notes := &cobra.Command{
		Use:   "notification",
		Short: "list notifications and mark them read",
		Long:  "Work with your notifications and your notification email settings.",
	}
	read := &cobra.Command{
		Use:     "read [ID]",
		Short:   "mark a notification read, or --all",
		Long:    "Mark one notification read, or every unread one with --all.",
		Example: examples("fz notification read 03abc123def456ghi789jkl0mn", "fz notification read --all"),
		Args:    args(cobra.MaximumNArgs(1)),
		RunE:    action(notificationRead),
	}
	read.Flags().Bool("all", false, "mark every unread notification read")
	settings := &cobra.Command{
		Use:     "settings",
		Short:   "show or set how often notification emails go out",
		Long:    "Show how often Fizzy bundles notification emails, or set it with --email to never, every_few_hours, daily or weekly.",
		Example: examples("fz notification settings", "fz notification settings --email daily"),
		Args:    args(cobra.NoArgs),
		RunE:    action(notificationSettings),
	}
	settings.Flags().String("email", "", "never, every_few_hours, daily or weekly")
	notes.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "list notifications, unread first",
		Long:    "List your notifications in the order Fizzy sends them: unread first, then read.",
		Example: examples("fz notification list", "fz --json notification list"),
		Args:    args(cobra.NoArgs),
		RunE:    action(notificationList),
	}, read, &cobra.Command{
		Use:     "unread ID",
		Short:   "mark a notification unread",
		Long:    "Mark one notification unread again.",
		Example: examples("fz notification unread 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(notificationUnread),
	}, settings)

	root.AddCommand(act, notes)
}

// particularsText summarizes an activity's particulars in key order. A pair
// old_X and new_X prints as "X: old to new"; a list prints comma-separated.
func particularsText(p map[string]any) string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if rest, ok := strings.CutPrefix(k, "new_"); ok {
			if _, paired := p["old_"+rest]; paired {
				continue
			}
		}
		if rest, ok := strings.CutPrefix(k, "old_"); ok {
			if n, paired := p["new_"+rest]; paired {
				parts = append(parts, fmt.Sprintf("%s: %s to %s", rest, valueText(p[k]), valueText(n)))
				continue
			}
		}
		parts = append(parts, k+": "+valueText(p[k]))
	}
	return strings.Join(parts, "; ")
}

func valueText(v any) string {
	if list, ok := v.([]any); ok {
		s := make([]string, len(list))
		for i, x := range list {
			s[i] = valueText(x)
		}
		return strings.Join(s, ", ")
	}
	if v == nil {
		return "None"
	}
	return fmt.Sprint(v)
}

func activityLine(a fizzy.Activity) string {
	what := a.EventableType
	if a.Eventable.Number != 0 {
		what += fmt.Sprintf(" #%d", a.Eventable.Number)
	}
	line := fmt.Sprintf("%s  %s  %s", a.CreatedAt, a.Action, what)
	if s := particularsText(a.Particulars); s != "" {
		line += "  " + s
	}
	return line
}

// activityCreator resolves --creator to a user id: an id, then an exact name
// ignoring case, then an email ignoring case.
func activityCreator(fz *fizzy.Fizzy, ref string) (string, error) {
	users, err := fz.Users()
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.ID == ref {
			return u.ID, nil
		}
	}
	var named []fizzy.User
	for _, u := range users {
		if strings.EqualFold(u.Name, ref) {
			named = append(named, u)
		}
	}
	switch len(named) {
	case 1:
		return named[0].ID, nil
	case 0:
	default:
		return "", fmt.Errorf("%d users are named %s; use the user id", len(named), pyRepr(ref))
	}
	for _, u := range users {
		if u.EmailAddress != "" && strings.EqualFold(u.EmailAddress, ref) {
			return u.ID, nil
		}
	}
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Name
	}
	return "", fmt.Errorf("no user %s (have: %s)", pyRepr(ref), strings.Join(names, ", "))
}

func activityList(c *cmdContext, _ []string) error {
	fl := c.cmd.Flags()
	fz, err := c.api()
	if err != nil {
		return err
	}
	boardID, creatorID := "", ""
	if name, _ := fl.GetString("board"); name != "" {
		b, err := fz.Board(name)
		if err != nil {
			return err
		}
		boardID = b.ID
	}
	if ref, _ := fl.GetString("creator"); ref != "" {
		if creatorID, err = activityCreator(fz, ref); err != nil {
			return err
		}
	}
	acts, err := fz.Activities(boardID, creatorID)
	if err != nil {
		return err
	}
	return emit(c.cmd, rawList(acts), listText(acts, activityLine, "(no activity)"))
}

func notificationLine(n fizzy.Notification) string {
	state := "unread"
	if n.Read {
		state = "read"
	}
	line := fmt.Sprintf("%s [%s] %s  %s", n.ID, state, n.CreatedAt, n.Title)
	if n.Body != "" {
		line += ": " + n.Body
	}
	return line
}

func notificationList(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	ns, err := fz.Notifications()
	if err != nil {
		return err
	}
	return emit(c.cmd, rawList(ns), listText(ns, notificationLine, "(no notifications)"))
}

func notificationRead(c *cmdContext, a []string) error {
	all, _ := c.cmd.Flags().GetBool("all")
	if all == (len(a) == 1) {
		return usagef("notification read needs an ID or --all, not both")
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if all {
		if err := fz.ReadAllNotifications(); err != nil {
			return err
		}
		return emit(c.cmd, obj{{"read", "all"}}, "all notifications marked read")
	}
	if err := fz.ReadNotification(a[0]); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"read", a[0]}}, "notification "+a[0]+" marked read")
}

func notificationUnread(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.UnreadNotification(a[0]); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"unread", a[0]}}, "notification "+a[0]+" marked unread")
}

func notificationSettings(c *cmdContext, _ []string) error {
	fl := c.cmd.Flags()
	if fl.Changed("email") {
		freq, _ := fl.GetString("email")
		if err := choice("email", freq, fizzy.EmailFrequencies); err != nil {
			return err
		}
		fz, err := c.api()
		if err != nil {
			return err
		}
		if err := fz.SetEmailFrequency(freq); err != nil {
			return err
		}
		return emit(c.cmd, obj{{"bundle_email_frequency", freq}}, "email set to "+freq)
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	s, err := fz.NotificationSettings()
	if err != nil {
		return err
	}
	return emit(c.cmd, s.Raw(), "email: "+s.BundleEmailFrequency)
}
