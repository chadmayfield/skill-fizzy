package cli

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addWebhookCommands) }

func addWebhookCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "webhook",
		Short: "manage a board's webhooks (account admins only)",
		Long: "Work with a board's webhooks. A webhook sends card data to an outside URL, so create, edit and delete need --yes and print the URL and actions first.\n\n" +
			"Only account admins can use these commands. The server answers 403 to anyone else.\n\n" +
			"Actions: " + strings.Join(fizzy.WebhookActions, ", ") + ".",
	}
	root.AddCommand(g)
	g.AddCommand(&cobra.Command{
		Use:     "list BOARD",
		Short:   "list a board's webhooks",
		Long:    "List the webhooks on a board, given by name or id. Prints one line per webhook, never the signing secret.",
		Example: examples("fz webhook list Gate"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(webhookList),
	})
	g.AddCommand(&cobra.Command{
		Use:     "show BOARD ID",
		Short:   "show one webhook",
		Long:    "Show one webhook. Text output never includes the signing secret; --json prints what the server sent.",
		Example: examples("fz webhook show Gate 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookShow),
	})
	create := &cobra.Command{
		Use:   "create BOARD --name NAME --url URL --actions A,B --yes",
		Short: "create a webhook (needs --yes)",
		Long: "Create a webhook that sends the listed actions to a URL. The URL must be https, or http for localhost. " +
			"It cannot change after creation. Prints the URL and actions and refuses without --yes.",
		Example: examples(`fz webhook create Gate --name "Hinge alerts" --url https://fizzy.example.com/hook --actions card_closed,comment_created --yes`),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(webhookCreate),
	}
	create.Flags().String("name", "", "webhook name")
	create.Flags().String("url", "", "URL that receives the events")
	create.Flags().String("actions", "", "comma-separated actions to send")
	create.Flags().Bool("yes", false, "really send card data to this URL")
	g.AddCommand(create)
	edit := &cobra.Command{
		Use:   "edit BOARD ID [--name NAME] [--actions A,B] --yes",
		Short: "change a webhook's name or actions (needs --yes)",
		Long: "Change a webhook's name or the actions it sends. The URL cannot change. " +
			"Prints the URL and the new actions and refuses without --yes.",
		Example: examples("fz webhook edit Gate 03abc123def456ghi789jkl0mn --actions card_closed --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookEdit),
	}
	edit.Flags().String("name", "", "new name")
	edit.Flags().String("actions", "", "comma-separated actions to send")
	edit.Flags().Bool("yes", false, "really change what this webhook sends")
	g.AddCommand(edit)
	del := &cobra.Command{
		Use:     "delete BOARD ID --yes",
		Short:   "delete a webhook (needs --yes)",
		Long:    "Delete a webhook. Prints its URL and actions and refuses without --yes.",
		Example: examples("fz webhook delete Gate 03abc123def456ghi789jkl0mn --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookDelete),
	}
	del.Flags().Bool("yes", false, "really delete")
	g.AddCommand(del)
	g.AddCommand(&cobra.Command{
		Use:     "activate BOARD ID",
		Short:   "reactivate a webhook",
		Long:    "Reactivate a deactivated webhook. Reads the webhook first and sends nothing when it is already active.",
		Example: examples("fz webhook activate Gate 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookActivate),
	})
	g.AddCommand(&cobra.Command{
		Use:     "deliveries BOARD ID",
		Short:   "list a webhook's deliveries",
		Long:    "List the attempts to send events to a webhook, one line each. Only account admins can read delivery history.",
		Example: examples("fz webhook deliveries Gate 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookDeliveries),
	})
}

// adminOnly adds a plain explanation to a 403, which is what a non-admin gets.
func adminOnly(err error) error {
	var ae *client.APIError
	if errors.As(err, &ae) && ae.Status == http.StatusForbidden {
		return fmt.Errorf("only account admins can use webhooks (HTTP 403): %w", err)
	}
	return err
}

// webhookBoard returns the id of the board named by arg.
func webhookBoard(fz *fizzy.Fizzy, arg string) (string, error) {
	b, err := fz.Board(arg)
	if err != nil {
		return "", adminOnly(err)
	}
	return b.ID, nil
}

func webhookLine(w fizzy.Webhook) string {
	state := "active"
	if !w.Active {
		state = "inactive"
	}
	return fmt.Sprintf("%s  [%s] %s -> %s  %s", w.ID, state, w.Name, w.PayloadURL, strings.Join(w.SubscribedActions, ","))
}

func deliveryLine(d fizzy.WebhookDelivery) string {
	result := "no response yet"
	if d.Response != nil {
		result = fmt.Sprintf("HTTP %d", d.Response.Code)
		if d.Response.Error != nil && *d.Response.Error != "" {
			result += ": " + *d.Response.Error
		}
	}
	return fmt.Sprintf("%s  %s  %s  %s", d.ID, d.State, d.Event.Action, result)
}

// splitActions splits a comma-separated flag, dropping blanks.
func splitActions(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func webhookList(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	hooks, err := fz.Webhooks(board)
	if err != nil {
		return adminOnly(err)
	}
	lines := make([]string, len(hooks))
	for i, h := range hooks {
		lines[i] = webhookLine(h)
	}
	return emit(c.cmd, rawList(hooks), strings.Join(lines, "\n"))
}

func webhookShow(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	h, err := fz.Webhook(board, a[1])
	if err != nil {
		return adminOnly(err)
	}
	return emit(c.cmd, h.Raw(), webhookLine(h))
}

// refuseWithoutYes prints what would be sent and where, then refuses unless
// --yes is set.
func refuseWithoutYes(c *cmdContext, summary string) error {
	fmt.Fprintln(c.errw(), summary)
	if yes, _ := c.cmd.Flags().GetBool("yes"); !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	return nil
}

func webhookCreate(c *cmdContext, a []string) error {
	name, _ := c.cmd.Flags().GetString("name")
	hookURL, _ := c.cmd.Flags().GetString("url")
	actions := splitActions(flagValue(c.cmd, "actions"))
	switch {
	case name == "":
		return usagef("--name is required")
	case hookURL == "":
		return usagef("--url is required")
	case len(actions) == 0:
		return usagef("--actions is required")
	}
	if err := fizzy.CheckWebhookURL(hookURL); err != nil {
		return usageError{err}
	}
	if err := fizzy.CheckWebhookActions(actions); err != nil {
		return usageError{err}
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	if err := refuseWithoutYes(c, fmt.Sprintf("about to create webhook %s: card data on board %s goes to %s for %s",
		pyRepr(name), a[0], hookURL, strings.Join(actions, ","))); err != nil {
		return err
	}
	h, err := fz.CreateWebhook(board, name, hookURL, actions)
	if err != nil {
		return adminOnly(err)
	}
	if h.Raw() != nil {
		return emit(c.cmd, h.Raw(), "webhook created: "+h.ID)
	}
	return emit(c.cmd, obj{{"id", orNull(h.ID)}}, "webhook created: "+orNone(h.ID))
}

func webhookEdit(c *cmdContext, a []string) error {
	var name *string
	if c.cmd.Flags().Changed("name") {
		n, _ := c.cmd.Flags().GetString("name")
		name = &n
	}
	var actions []string
	if c.cmd.Flags().Changed("actions") {
		actions = splitActions(flagValue(c.cmd, "actions"))
		if len(actions) == 0 {
			return usagef("--actions needs at least one action")
		}
	}
	if name == nil && actions == nil {
		return usagef("give --name or --actions")
	}
	if err := fizzy.CheckWebhookActions(actions); err != nil {
		return usageError{err}
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	h, err := fz.Webhook(board, a[1])
	if err != nil {
		return adminOnly(err)
	}
	var changes []string
	if name != nil {
		changes = append(changes, "name "+pyRepr(*name))
	}
	if actions != nil {
		changes = append(changes, "actions "+strings.Join(actions, ","))
	}
	if err := refuseWithoutYes(c, fmt.Sprintf("about to change webhook %s sending to %s: %s", h.ID, h.PayloadURL, strings.Join(changes, "; "))); err != nil {
		return err
	}
	upd, err := fz.UpdateWebhook(board, a[1], name, actions)
	if err != nil {
		return adminOnly(err)
	}
	var fields []string
	if name != nil {
		fields = append(fields, "name")
	}
	if actions != nil {
		fields = append(fields, "actions")
	}
	text := fmt.Sprintf("webhook %s updated: %s", a[1], strings.Join(fields, ", "))
	if upd.Raw() != nil {
		return emit(c.cmd, upd.Raw(), text)
	}
	return emit(c.cmd, obj{{"updated", a[1]}}, text)
}

func webhookDelete(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	h, err := fz.Webhook(board, a[1])
	if err != nil {
		return adminOnly(err)
	}
	if err := refuseWithoutYes(c, fmt.Sprintf("about to delete webhook %s: %s sending %s", h.ID, h.PayloadURL, strings.Join(h.SubscribedActions, ","))); err != nil {
		return err
	}
	if err := fz.DeleteWebhook(board, a[1]); err != nil {
		return adminOnly(err)
	}
	return emit(c.cmd, obj{{"deleted", a[1]}}, "webhook "+a[1]+" deleted")
}

func webhookActivate(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	changed, err := fz.ActivateWebhook(board, a[1])
	if err != nil {
		return adminOnly(err)
	}
	text := "webhook " + a[1] + " activated"
	if !changed {
		text = "webhook " + a[1] + " already active"
	}
	return emit(c.cmd, obj{{"id", a[1]}, {"changed", changed}}, text)
}

func webhookDeliveries(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	board, err := webhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	ds, err := fz.WebhookDeliveries(board, a[1])
	if err != nil {
		return adminOnly(err)
	}
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = deliveryLine(d)
	}
	return emit(c.cmd, rawList(ds), strings.Join(lines, "\n"))
}
