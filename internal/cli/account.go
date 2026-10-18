package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addAccountCommands) }

func addAccountCommands(root *cobra.Command) {
	acct := &cobra.Command{
		Use:   "account",
		Short: "show the account and its auto-postpone default",
		Long:  "Work with the configured account.",
	}
	postpone := &cobra.Command{
		Use:     "postpone DAYS",
		Short:   "set the account's auto-postpone default (needs --yes)",
		Long:    "Set how many days without activity move a card to Not now, for boards that do not set their own period. DAYS is one of 3, 7, 11, 30, 90 or 365. Prints the current and new period, and refuses without --yes. Needs the admin role.",
		Example: examples("fz account postpone 30 --yes"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(accountPostpone),
	}
	postpone.Flags().Bool("yes", false, "really change it")
	acct.AddCommand(&cobra.Command{
		Use:     "show",
		Short:   "show the account",
		Long:    "Print the account's name, id, card count, auto-postpone default and creation time.",
		Example: examples("fz account show", "fz --json account show"),
		Args:    args(cobra.NoArgs),
		RunE:    action(accountShow),
	}, postpone)

	export := &cobra.Command{
		Use:   "export",
		Short: "start and check data exports",
		Long:  "Exports run on the server. Start one, then check its status until it is completed and has a download link. The file expires after 24 hours.",
	}
	status := &cobra.Command{
		Use:     "status ID",
		Short:   "check an export",
		Long:    "Print an export's id, status and creation time, and the download link once it is completed. Add --mine for a personal data export.",
		Example: examples("fz export status 03abc123def456ghi789jkl0mn", "fz export status 03abc123def456ghi789jkl0mn --mine"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(exportStatus),
	}
	status.Flags().Bool("mine", false, "the export is a personal data export")
	export.AddCommand(&cobra.Command{
		Use:     "account",
		Short:   "start an account export",
		Long:    "Start an export of the whole account. Only admins and owners can.",
		Example: examples("fz export account"),
		Args:    args(cobra.NoArgs),
		RunE:    action(exportAccount),
	}, &cobra.Command{
		Use:     "mine",
		Short:   "start a personal data export",
		Long:    "Start an export of your own data on this account. Your user id comes from /my/identity.",
		Example: examples("fz export mine"),
		Args:    args(cobra.NoArgs),
		RunE:    action(exportMine),
	}, status)

	root.AddCommand(acct, export)
}

func postponeText(days *int) string {
	if days == nil {
		return "none"
	}
	return strconv.Itoa(*days) + " days"
}

func accountShow(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	a, err := fz.AccountSettings()
	if err != nil {
		return err
	}
	text := fmt.Sprintf("%s  %s\ncards: %d\nauto-postpone: %s\ncreated: %s",
		a.Name, a.ID, a.CardsCount, postponeText(a.AutoPostponePeriodInDays), a.CreatedAt)
	return emit(c.cmd, a.Raw(), text)
}

func accountPostpone(c *cmdContext, a []string) error {
	days, err := strconv.Atoi(a[0])
	if err != nil {
		return usagef("argument DAYS: invalid int value: %s", pyRepr(a[0]))
	}
	if err := fizzy.ValidPostponeDays(days); err != nil {
		return usageError{err}
	}
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	cur, err := fz.AccountSettings()
	if err != nil {
		return err
	}
	from := "none"
	if cur.AutoPostponePeriodInDays != nil {
		from = strconv.Itoa(*cur.AutoPostponePeriodInDays)
	}
	fmt.Fprintf(c.errw(), "about to change the auto-postpone period of %s from %s to %d days\n", cur.Name, from, days)
	if !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	updated, err := fz.PostponeAccount(days)
	if err != nil {
		return err
	}
	var data any = obj{{"auto_postpone_period_in_days", days}}
	if updated.Raw() != nil {
		data = updated.Raw()
	}
	return emit(c.cmd, c.dry(data), c.wrote(fmt.Sprintf("account auto-postpone set to %d days", days), fmt.Sprintf("set account auto-postpone to %d days", days)))
}

func exportLine(e fizzy.Export) string {
	return strings.TrimSpace(strings.Join([]string{e.ID, e.Status, e.CreatedAt, e.DownloadURL}, " "))
}

// emitStarted prints a new export, or a dry-run note when none was made.
// what names the export for that note.
func emitStarted(c *cmdContext, e fizzy.Export, what string) error {
	if e.Raw() == nil {
		return emit(c.cmd, c.dry(obj{{"id", nil}}), c.wrote("export started", "start "+what))
	}
	return emit(c.cmd, e.Raw(), exportLine(e))
}

func exportAccount(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	e, err := fz.StartAccountExport()
	if err != nil {
		return err
	}
	return emitStarted(c, e, "an account export")
}

func exportMine(c *cmdContext, _ []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	e, err := fz.StartMyExport()
	if err != nil {
		return err
	}
	return emitStarted(c, e, "your export")
}

func exportStatus(c *cmdContext, a []string) error {
	mine, _ := c.cmd.Flags().GetBool("mine")
	fz, err := c.api()
	if err != nil {
		return err
	}
	get := fz.AccountExport
	if mine {
		get = fz.MyExport
	}
	e, err := get(a[0])
	if err != nil {
		return err
	}
	return emit(c.cmd, e.Raw(), exportLine(e))
}
