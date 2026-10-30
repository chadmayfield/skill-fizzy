package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/chadmayfield/skill-fizzy/client"
	"github.com/chadmayfield/skill-fizzy/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addWebhookCommands) }

func addWebhookCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "webhook",
		Short: "manage a board's webhooks (account admins only)",
		Long: "Work with a board's webhooks. A webhook sends card data to an outside URL, so create, edit and delete need --yes and print the URL and actions first.\n\n" +
			"Only account admins can use these commands. The server answers 403 to anyone else.\n\n" +
			"The signing secret is shown once, when you create a webhook. List and show hide it, also with --json. Use show --json --reveal-secret to print it again; text output never includes it.\n\n" +
			"Actions: " + strings.Join(fizzy.WebhookActions, ", ") + ".",
	}
	root.AddCommand(g)
	g.AddCommand(&cobra.Command{
		Use:     "list BOARD",
		Short:   "list a board's webhooks",
		Long:    "List the webhooks on a board, given by name or id. Prints one line per webhook. The signing secret is hidden, also with --json.",
		Example: examples("fz webhook list Gate"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(webhookList),
	})
	show := &cobra.Command{
		Use:     "show BOARD ID",
		Short:   "show one webhook",
		Long:    "Show one webhook. Text output never includes the signing secret, and --json drops it unless you add --reveal-secret.",
		Example: examples("fz webhook show Gate 03abc123def456ghi789jkl0mn", "fz webhook show Gate 03abc123def456ghi789jkl0mn --json --reveal-secret"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookShow),
	}
	show.Flags().Bool("reveal-secret", false, "include the signing secret in --json output")
	g.AddCommand(show)
	create := &cobra.Command{
		Use:   "create BOARD",
		Short: "create a webhook (needs --yes)",
		Long: "Create a webhook that sends the listed actions to a URL. The URL must be https, or http for localhost. " +
			"It cannot change after creation. An empty or blank name exits 2 and sends nothing. A repeated action is sent once. Prints the board, the URL and the actions and refuses without --yes. " +
			"The reply includes the signing secret. Create prints it once, to stderr next to the warning to save it, so stdout stays free of it. With --json it is in the JSON on stdout, because a script needs it, and the warning goes to stderr. List and show hide it afterwards.",
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
		Use:   "edit BOARD ID",
		Short: "change a webhook's name or actions (needs --yes)",
		Long: "Change a webhook's name or the actions it sends. The URL cannot change. " +
			"An empty or blank name exits 2 and sends nothing. Prints the URL and the new actions and refuses without --yes. " +
			"When the webhook already has the name and actions given, it says no change is needed, sends nothing and exits 0.",
		Example: examples("fz webhook edit Gate 03abc123def456ghi789jkl0mn --actions card_closed --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(webhookEdit),
	}
	edit.Flags().String("name", "", "new name")
	edit.Flags().String("actions", "", "comma-separated actions to send")
	edit.Flags().Bool("yes", false, "really change what this webhook sends")
	g.AddCommand(edit)
	del := &cobra.Command{
		Use:     "delete BOARD ID",
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
	b, err := findWebhookBoard(fz, arg)
	return b.ID, err
}

// findWebhookBoard returns the board named by arg. A 403 here comes from
// the board list, not from a webhook call, so it is not blamed on the admin
// role.
func findWebhookBoard(fz *fizzy.Fizzy, arg string) (fizzy.Board, error) {
	return fz.Board(arg)
}

func webhookLine(w fizzy.Webhook) string {
	state := "active"
	if !w.Active {
		state = "inactive"
	}
	return line(fmt.Sprintf("%s  [%s] %s -> %s  %s", w.ID, state, w.Name, w.PayloadURL, strings.Join(w.SubscribedActions, ",")))
}

func deliveryLine(d fizzy.WebhookDelivery) string {
	result := "no response yet"
	if d.Response != nil {
		result = fmt.Sprintf("HTTP %d", d.Response.Code)
		if d.Response.Error != nil && *d.Response.Error != "" {
			result += ": " + *d.Response.Error
		}
	}
	return line(fmt.Sprintf("%s  %s  %s  %s", d.ID, d.State, d.Event.Action, result))
}

// stripSecret drops signing_secret from a webhook's raw JSON and keeps every
// other key, value and the key order. It returns raw unchanged if raw is not
// a JSON object.
func stripSecret(raw json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return raw
	}
	var out obj
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return raw
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return raw
		}
		if k.(string) != "signing_secret" {
			out = append(out, kv{k.(string), v})
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return b
}

// splitActions splits a comma-separated flag, dropping blanks and repeats.
func splitActions(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ",") {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(out, a) {
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
	stripped := make([]json.RawMessage, len(hooks))
	for i, h := range hooks {
		stripped[i] = stripSecret(h.Raw())
	}
	return emit(c.cmd, stripped, listText(hooks, webhookLine, "(no webhooks)"))
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
	raw := h.Raw()
	if reveal, _ := c.cmd.Flags().GetBool("reveal-secret"); !reveal {
		raw = stripSecret(raw)
	}
	return emit(c.cmd, raw, webhookLine(h))
}

// refuseWithoutYes prints what would be sent and where, then refuses unless
// --yes is set.
func refuseWithoutYes(c *cmdContext, summary string) error {
	fmt.Fprintln(c.errw(), line(summary))
	if yes, _ := c.cmd.Flags().GetBool("yes"); !yes {
		return refuse(c)
	}
	return nil
}

func webhookCreate(c *cmdContext, a []string) error {
	name, _ := c.cmd.Flags().GetString("name")
	name = strings.TrimSpace(name)
	hookURL, _ := c.cmd.Flags().GetString("url")
	actions := splitActions(flagValue(c.cmd, "actions"))
	switch {
	case !c.cmd.Flags().Changed("name"):
		return usagef("--name is required")
	case name == "":
		return checkName("--name", name)
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
	b, err := findWebhookBoard(fz, a[0])
	if err != nil {
		return err
	}
	if err := refuseWithoutYes(c, fmt.Sprintf("about to create webhook %s: card data on board %s (%s) goes to %s for %s",
		pyRepr(name), b.Name, b.ID, hookURL, strings.Join(actions, ","))); err != nil {
		return err
	}
	h, err := fz.CreateWebhook(b.ID, name, hookURL, actions)
	if err != nil {
		return adminOnly(err)
	}
	if c.dryRun {
		return emit(c.cmd, c.dry(obj{{"id", nil}}), c.wrote("", "create webhook "+pyRepr(name)))
	}
	text := "webhook created: " + line(orNone(h.ID))
	if h.SigningSecret != "" {
		// The secret goes to stderr, so stdout can be logged without it.
		// With --json it stays in the JSON, because a script needs it.
		fmt.Fprintln(c.errw(), "save the signing secret now; fz hides it from list and show")
		if flagValue(c.cmd, "json") != "true" {
			fmt.Fprintln(c.errw(), "signing secret: "+line(h.SigningSecret))
		}
	}
	if h.Raw() != nil {
		return emit(c.cmd, h.Raw(), text)
	}
	return emit(c.cmd, obj{{"id", orNull(h.ID)}}, text)
}

func webhookEdit(c *cmdContext, a []string) error {
	var name *string
	if c.cmd.Flags().Changed("name") {
		n, _ := c.cmd.Flags().GetString("name")
		n = strings.TrimSpace(n)
		if err := checkName("--name", n); err != nil {
			return err
		}
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
	if (name == nil || *name == h.Name) && (actions == nil || sameIDs(actions, h.SubscribedActions)) {
		return emit(c.cmd, c.dry(obj{{"id", h.ID}, {"changed", false}}),
			noChange(c, fmt.Sprintf("webhook %s already has these values; no change needed", line(h.ID))))
	}
	var changes []string
	if name != nil {
		changes = append(changes, "name "+pyRepr(*name))
	}
	if actions != nil {
		changes = append(changes, "actions "+strings.Join(actions, ","))
	}
	if err := refuseWithoutYes(c, fmt.Sprintf("about to change webhook %s sending to %s: %s", line(h.ID), line(h.PayloadURL), strings.Join(changes, "; "))); err != nil {
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
	text := c.wrote(fmt.Sprintf("webhook %s updated: %s", a[1], strings.Join(fields, ", ")),
		fmt.Sprintf("update webhook %s: %s", a[1], strings.Join(fields, ", ")))
	if upd.Raw() != nil {
		return emit(c.cmd, stripSecret(upd.Raw()), text)
	}
	return emit(c.cmd, c.dry(obj{{"updated", a[1]}}), text)
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
	if err := refuseWithoutYes(c, line(fmt.Sprintf("about to delete webhook %s %s: %s sending %s", h.ID, pyRepr(h.Name), h.PayloadURL, strings.Join(h.SubscribedActions, ",")))); err != nil {
		return err
	}
	if err := fz.DeleteWebhook(board, a[1]); err != nil {
		return adminOnly(err)
	}
	return emit(c.cmd, c.dry(obj{{"deleted", a[1]}}), c.wrote("webhook "+a[1]+" deleted", "delete webhook "+a[1]))
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
	switch {
	case !changed:
		text = noChange(c, "webhook "+a[1]+" already active")
	case c.dryRun:
		text = c.wrote("", "activate webhook "+a[1])
	}
	return emit(c.cmd, c.dry(obj{{"id", a[1]}, {"changed", changed}}), text)
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
	return emit(c.cmd, rawList(ds), listText(ds, deliveryLine, "(no deliveries)"))
}
