package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/markdown"
	"github.com/chadmayfield/skill-fizzy/internal/richtext"
	"github.com/spf13/cobra"
)

func init() { register(addCardEditCommand) }

// addCardEditCommand adds card edit under the card group. The group is
// registered first, because cards.go sorts before cards_edit.go.
func addCardEditCommand(root *cobra.Command) {
	edit := &cobra.Command{
		Use:   "edit NUMBER [OLD NEW]",
		Short: "change a card's title, description or status",
		Long: "Change a card. A new description replaces the whole thing, including embedded images. " +
			"--replace swaps one phrase in the current description and leaves the rest alone. " +
			"Give the phrase and its replacement after the number.",
		Example: examples(
			`fz card edit 87 --title "Fix the gate hinge"`,
			`fz card edit 87 --replace "50% off" "up to 50% off"`,
			"fz card edit 87 --description-file goal.md",
			"fz --dry-run card edit 87 --replace hinge hinges --all",
		),
		Args: args(cobra.MinimumNArgs(1)),
		RunE: action(cardEdit),
	}
	edit.Flags().String("title", "", "new title")
	edit.Flags().String("description", "", "new description as Markdown")
	edit.Flags().String("description-file", "", "read the new Markdown description from a file")
	// --replace takes OLD and NEW, which cobra cannot read as two values of
	// one flag. The flag takes none, and the two words follow the number.
	edit.Flags().Bool("replace", false, "swap one phrase in the description; give OLD and NEW after the number")
	edit.Flags().Bool("all", false, "with --replace, swap every match")
	edit.Flags().String("status", "", "card status, drafted or published")

	for _, c := range root.Commands() {
		if c.Name() == "card" {
			c.AddCommand(edit)
		}
	}
}

func cardEdit(c *cmdContext, a []string) error {
	fl := c.cmd.Flags()
	number, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	replace, _ := fl.GetBool("replace")
	all, _ := fl.GetBool("all")
	switch {
	case replace && len(a) != 3:
		return usagef("--replace needs OLD and NEW after the number")
	case !replace && len(a) != 1:
		return usagef("OLD and NEW only go with --replace")
	case all && !replace:
		return usagef("--all only goes with --replace")
	case fl.Changed("description") && fl.Changed("description-file"):
		return usagef("argument --description-file: not allowed with argument --description")
	}
	status, _ := fl.GetString("status")
	if fl.Changed("status") {
		if err := choice("status", status, []string{"drafted", "published"}); err != nil {
			return err
		}
	}

	var fields fizzy.CardFields
	var changed []string
	if title, _ := fl.GetString("title"); title != "" {
		fields.Title = &title
		changed = append(changed, "title")
	}
	var md *string
	if fl.Changed("description") {
		d, _ := fl.GetString("description")
		md = &d
	}
	if file, _ := fl.GetString("description-file"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		d := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n")
		md = &d
	}
	if md != nil {
		html := markdown.ToHTML(*md)
		fields.Description = &html
		changed = append(changed, "description")
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if replace {
		if fields.Description != nil {
			return errors.New("use --replace or a new description, not both")
		}
		card, err := fz.Card(number)
		if err != nil {
			return err
		}
		html, err := richtext.ReplaceText(richtext.UnwrapActionText(card.DescriptionHTML), a[1], a[2], all)
		if err != nil {
			return err
		}
		fields.Description = &html
		changed = append(changed, "description")
	}
	if status != "" {
		fields.Status = &status
		changed = append(changed, "status")
	}
	if err := fz.UpdateCard(number, fields); err != nil {
		return err
	}
	slices.Sort(changed)
	text := fmt.Sprintf("card #%d updated: %s", number, strings.Join(changed, ", "))
	return emit(c.cmd, obj{{"number", number}, {"changed", changed}}, text)
}
