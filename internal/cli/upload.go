package cli

import (
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/spf13/cobra"
)

func init() { register(addUploadCommand) }

func addUploadCommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:     "upload FILE",
		Short:   "upload a file and print its sgid",
		Long:    "Upload a file through Fizzy's direct upload and print the attachable sgid for rich text.",
		Example: examples("fz upload ~/Pictures/hinge.jpg"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(upload),
	})
}

func upload(c *cmdContext, a []string) error {
	fz, err := c.api()
	if err != nil {
		return err
	}
	sgid, err := fz.Upload(config.ExpandTilde(a[0]))
	if err != nil {
		return err
	}
	return emit(c.cmd, obj{{"sgid", orNull(sgid)}}, sgid)
}
