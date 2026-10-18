package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func init() { register(addStepCommands) }

func addStepCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "step",
		Short: "add, list, edit, tick, untick and delete a card's steps",
		Long:  "Work with the checklist on a card. Steps are addressed by id; fz step list shows them.",
	}
	root.AddCommand(g)

	add := &cobra.Command{
		Use:     "add NUMBER CONTENT",
		Short:   "add a step",
		Long:    "Add a step to the end of a card's checklist.",
		Example: examples(`fz step add 87 "Order the hinge"`, `fz step add 87 "Measured the gap" --done`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(stepAdd),
	}
	add.Flags().Bool("done", false, "add it already ticked")

	list := &cobra.Command{
		Use:     "list NUMBER",
		Short:   "list steps with their ids",
		Long:    "Print each step's tick, id and text.",
		Example: examples("fz step list 87"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(stepList),
	}

	edit := &cobra.Command{
		Use:     "edit NUMBER STEP_ID CONTENT",
		Short:   "change a step's text",
		Long:    "Replace the text of one step. Empty or blank text exits 2 and sends nothing.",
		Example: examples(`fz step edit 87 03abc123def456ghi789jkl0mn "Order two hinges"`),
		Args:    args(cobra.ExactArgs(3)),
		RunE:    action(stepEdit),
	}

	check := &cobra.Command{
		Use:     "check NUMBER STEP_ID",
		Short:   "tick a step",
		Long:    "Mark a step done.",
		Example: examples("fz step check 87 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(stepSetter(true)),
	}

	uncheck := &cobra.Command{
		Use:     "uncheck NUMBER STEP_ID",
		Short:   "untick a step",
		Long:    "Mark a step not done.",
		Example: examples("fz step uncheck 87 03abc123def456ghi789jkl0mn"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(stepSetter(false)),
	}

	del := &cobra.Command{
		Use:     "delete NUMBER STEP_ID",
		Short:   "delete a step (needs --yes)",
		Long:    "Remove a step from the checklist. Without --yes it prints the step it would delete, deletes nothing and exits 2.",
		Example: examples("fz step delete 87 03abc123def456ghi789jkl0mn --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(stepDelete),
	}
	del.Flags().Bool("yes", false, "really delete")

	g.AddCommand(add, list, edit, check, uncheck, del)
}

func stepAdd(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	if err := checkName("the step text", a[1]); err != nil {
		return err
	}
	done, _ := c.cmd.Flags().GetBool("done")
	fz, err := c.api()
	if err != nil {
		return err
	}
	id, err := fz.AddStep(n, a[1], done)
	if err != nil {
		return err
	}
	return emit(c.cmd, obj{{"id", orNull(id)}}, "step added: "+orNone(id))
}

func stepList(c *cmdContext, a []string) error {
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(card.Raw(), &fields); err != nil {
		return err
	}
	raw := fields["steps"]
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("[]")
	}
	lines := make([]string, len(card.Steps))
	for i, s := range card.Steps {
		lines[i] = fmt.Sprintf("[%s] %s  %s", tick(s.Completed), line(s.ID), line(s.Content))
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no steps)"
	}
	return emit(c.cmd, raw, text)
}

func stepEdit(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	if err := checkName("the step text", a[2]); err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	content := a[2]
	if err := fz.UpdateStep(n, a[1], &content, nil); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"id", a[1]}}, "step updated")
}

// stepSetter ticks or unticks a step. The output matches fz 0.1's
// "step N --check ID": the text "step updated" and an empty JSON object.
func stepSetter(done bool) func(*cmdContext, []string) error {
	return func(c *cmdContext, a []string) error {
		n, err := cardNumber(a[0])
		if err != nil {
			return err
		}
		fz, err := c.api()
		if err != nil {
			return err
		}
		if err := fz.UpdateStep(n, a[1], nil, &done); err != nil {
			return err
		}
		return emit(c.cmd, obj{}, "step updated")
	}
}

func stepDelete(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if yes, _ := c.cmd.Flags().GetBool("yes"); !yes {
		// Show what would go, the way card delete does, then refuse.
		card, err := fz.Card(n)
		if err != nil {
			return err
		}
		preview := fmt.Sprintf("about to delete step %s, which card #%d does not list", a[1], n)
		for _, s := range card.Steps {
			if s.ID == a[1] {
				preview = fmt.Sprintf("about to delete step: [%s] %s  %s (card #%d)", tick(s.Completed), line(s.ID), line(s.Content), n)
				break
			}
		}
		fmt.Fprintln(c.errw(), preview)
		return refuse(c)
	}
	if err := fz.DeleteStep(n, a[1]); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"deleted", a[1]}}, "step deleted")
}
