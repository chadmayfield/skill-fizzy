package cli

import (
	"fmt"
	"regexp"
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
		Long: "Change a card. An empty or blank --title exits 2 and sends nothing. A new description replaces the whole thing, including embedded images. " +
			"fz refuses a new description that is empty or only whitespace, or a --replace that leaves it so, unless --allow-empty is given. " +
			"--replace swaps one phrase in the current description and leaves the rest alone. " +
			"Give the phrase and its replacement after the number. The phrase must not be empty. " +
			"It matches only text, never tag names or attributes such as a link's address, " +
			"and a match cannot cross formatting. A phrase that is not found, or that matches more than once without --all, exits 2 and sends nothing.",
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
	edit.Flags().String("description-file", "", "read the new Markdown description from a file of at most 4 MiB")
	// --replace takes OLD and NEW, which cobra cannot read as two values of
	// one flag. The flag takes none, and the two words follow the number.
	edit.Flags().Bool("replace", false, "swap one phrase in the description; give OLD and NEW after the number")
	edit.Flags().Bool("all", false, "with --replace, swap every match")
	edit.Flags().Bool("allow-empty", false, "allow a new description that is empty, which clears it")
	edit.Flags().String("status", "", "card status, drafted or published")

	if g := subgroup(root, "card"); g != nil {
		g.AddCommand(edit)
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
	case replace && a[1] == "":
		return usagef("--replace: OLD must not be empty")
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
	if fl.Changed("title") {
		title, _ := fl.GetString("title")
		if err := checkName("the card title", title); err != nil {
			return err
		}
		fields.Title = &title
		changed = append(changed, "title")
	}
	var md *string
	if fl.Changed("description") {
		d, _ := fl.GetString("description")
		md = &d
	}
	if file, _ := fl.GetString("description-file"); file != "" {
		d, err := readDescriptionFile(file)
		if err != nil {
			return err
		}
		md = &d
	}
	allowEmpty, _ := fl.GetBool("allow-empty")
	switch {
	case allowEmpty && md == nil && !replace:
		return usagef("--allow-empty only goes with --description, --description-file or --replace")
	case md != nil && strings.TrimSpace(*md) == "" && !allowEmpty:
		return usagef("the new description is empty; add --allow-empty to clear it")
	case md != nil && replace:
		return usagef("use --replace or a new description, not both")
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
		card, err := fz.Card(number)
		if err != nil {
			return err
		}
		src := richtext.UnwrapActionText(card.DescriptionHTML)
		html, err := richtext.ReplaceText(src, a[1], a[2], all)
		if err != nil {
			// Every ReplaceText error stops the edit before anything is
			// sent, such as a phrase that is not found or that matches
			// more than once, so each is a refusal and exits 2.
			return fizzy.Refuse(err.Error())
		}
		if emptyDescription(html) && !allowEmpty {
			return usagef("the new description is empty; add --allow-empty to clear it")
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
	text := c.wrote(fmt.Sprintf("card #%d updated: %s", number, strings.Join(changed, ", ")),
		fmt.Sprintf("update card #%d: %s", number, strings.Join(changed, ", ")))
	return emit(c.cmd, c.dry(obj{{"number", number}, {"changed", changed}}), text)
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// emptyDescription reports whether html shows nothing: no text once tags and
// whitespace are gone, and no image or attachment.
func emptyDescription(html string) bool {
	if strings.Contains(html, "<img") || strings.Contains(html, "<action-text-attachment") {
		return false
	}
	return strings.TrimSpace(htmlTag.ReplaceAllString(html, "")) == ""
}
