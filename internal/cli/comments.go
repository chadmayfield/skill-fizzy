package cli

import (
	"github.com/chadmayfield/skill-fizzy/internal/markdown"
	"github.com/spf13/cobra"
)

func init() { register(addCommentCommands) }

func addCommentCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "comment",
		Short: "add, list, show, edit and delete comments, and react to them",
		Long:  "Work with a card's comments. Text is Markdown.",
	}
	root.AddCommand(g)
	g.AddCommand(&cobra.Command{
		Use:     "add NUMBER TEXT",
		Short:   "add a comment",
		Long:    "Add a Markdown comment to a card.",
		Example: examples(`fz comment add 87 "Measured: 42 mm"`),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(commentAdd),
	})
}

func commentAdd(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	if err := checkName("the comment text", a[1]); err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	id, err := fz.AddComment(n, markdown.ToHTML(a[1]))
	if err != nil {
		return err
	}
	return emit(c.cmd, obj{{"id", orNull(id)}}, "comment added: "+line(orNone(id)))
}
