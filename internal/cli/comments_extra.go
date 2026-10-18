package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/chadmayfield/skill-fizzy/internal/markdown"
	"github.com/spf13/cobra"
)

func init() { register(addCommentExtraCommands) }

// addCommentExtraCommands adds the comment subcommands beyond add. The comment
// group is registered first, because comments.go sorts before this file.
func addCommentExtraCommands(root *cobra.Command) {
	var g *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "comment" {
			g = c
		}
	}
	if g == nil {
		return
	}
	del := &cobra.Command{
		Use:     "delete NUMBER COMMENT_ID",
		Short:   "delete a comment (needs --yes)",
		Long:    "Delete a comment. Only its author can. Prints the comment first and refuses without --yes.",
		Example: examples("fz comment delete 87 03abc123def456ghi789jkl0mn --yes"),
		Args:    args(cobra.ExactArgs(2)),
		RunE:    action(commentDelete),
	}
	del.Flags().Bool("yes", false, "really delete")
	g.AddCommand(
		&cobra.Command{
			Use:     "list NUMBER",
			Short:   "list a card's comments",
			Long:    "List a card's comments, oldest first: id, author, time and the first line of the text.",
			Example: examples("fz comment list 87", "fz --json comment list 87"),
			Args:    args(cobra.ExactArgs(1)),
			RunE:    action(commentList),
		},
		&cobra.Command{
			Use:     "show NUMBER COMMENT_ID",
			Short:   "show one comment",
			Long:    "Show one comment: its id, author and time, then the full text.",
			Example: examples("fz comment show 87 03abc123def456ghi789jkl0mn"),
			Args:    args(cobra.ExactArgs(2)),
			RunE:    action(commentShow),
		},
		&cobra.Command{
			Use:     "edit NUMBER COMMENT_ID TEXT",
			Short:   "replace a comment's text",
			Long:    "Replace a comment's text with new Markdown. Only the comment's author can, and the server's refusal is printed as is.",
			Example: examples(`fz comment edit 87 03abc123def456ghi789jkl0mn "Gate latches now."`),
			Args:    args(cobra.ExactArgs(3)),
			RunE:    action(commentEdit),
		},
		del,
		&cobra.Command{
			Use:     "react NUMBER COMMENT_ID TEXT",
			Short:   "react to a comment",
			Long:    "Add a reaction to a comment. The text is at most 16 characters.",
			Example: examples(`fz comment react 87 03abc123def456ghi789jkl0mn "ok"`),
			Args:    args(cobra.ExactArgs(3)),
			RunE:    action(commentReact),
		},
		&cobra.Command{
			Use:     "reactions NUMBER COMMENT_ID",
			Short:   "list a comment's reactions",
			Long:    "List the reactions on a comment: id, who reacted and the text.",
			Example: examples("fz comment reactions 87 03abc123def456ghi789jkl0mn"),
			Args:    args(cobra.ExactArgs(2)),
			RunE:    action(commentReactions),
		},
		&cobra.Command{
			Use:     "unreact NUMBER COMMENT_ID REACTION_ID",
			Short:   "remove your reaction from a comment",
			Long:    "Remove a reaction from a comment. Only the person who reacted can.",
			Example: examples("fz comment unreact 87 03abc123def456ghi789jkl0mn 03def456ghi789jkl0mn123abc"),
			Args:    args(cobra.ExactArgs(3)),
			RunE:    action(commentUnreact),
		},
	)
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

func commentHead(c fizzy.Comment) string {
	return fmt.Sprintf("%s  %s  %s", c.ID, c.Creator.Name, c.CreatedAt)
}

func commentList(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	cs, err := fz.Comments(n)
	if err != nil {
		return err
	}
	lines := make([]string, len(cs))
	for i, cm := range cs {
		lines[i] = commentHead(cm) + "  " + firstLine(cm.Body.PlainText)
	}
	return emit(c.cmd, rawList(cs), strings.Join(lines, "\n"))
}

func commentShow(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	cm, err := fz.Comment(n, a[1])
	if err != nil {
		return err
	}
	return emit(c.cmd, json.RawMessage(cm.Raw()), commentHead(cm)+"\n"+strings.TrimRight(cm.Body.PlainText, "\n"))
}

func commentEdit(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.EditComment(n, a[1], markdown.ToHTML(a[2])); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"updated", a[1]}}, c.wrote("comment "+a[1]+" updated", "update comment "+a[1]))
}

func commentDelete(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	yes, _ := c.cmd.Flags().GetBool("yes")
	fz, err := c.api()
	if err != nil {
		return err
	}
	cm, err := fz.Comment(n, a[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(c.errw(), "about to delete comment %s  %s\n", commentHead(cm), firstLine(cm.Body.PlainText))
	if !yes {
		fmt.Fprintln(c.errw(), "refusing without --yes")
		return exitCode(2)
	}
	if err := fz.DeleteComment(n, a[1]); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"deleted", a[1]}}, c.wrote("comment "+a[1]+" deleted", "delete comment "+a[1]))
}

func commentReact(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	id, err := fz.ReactToComment(n, a[1], a[2])
	if err != nil {
		return err
	}
	return emit(c.cmd, obj{{"id", orNull(id)}}, c.wrote("reaction added: "+orNone(id), "add a reaction to comment "+a[1]))
}

func commentReactions(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	rs, err := fz.CommentReactions(n, a[1])
	if err != nil {
		return err
	}
	lines := make([]string, len(rs))
	for i, r := range rs {
		lines[i] = fmt.Sprintf("%s  %s  %s", r.ID, r.Reacter.Name, r.Content)
	}
	return emit(c.cmd, rawList(rs), strings.Join(lines, "\n"))
}

func commentUnreact(c *cmdContext, a []string) error {
	n, err := cardNumber(a[0])
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	if err := fz.DeleteCommentReaction(n, a[1], a[2]); err != nil {
		return err
	}
	return emit(c.cmd, obj{{"removed", a[2]}}, c.wrote("reaction "+a[2]+" removed", "remove reaction "+a[2]))
}
