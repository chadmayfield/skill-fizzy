package cli

import (
	"os"

	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/spf13/cobra"
)

func init() { register(addUploadCommand) }

func addUploadCommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "upload FILE",
		Short: "upload a file and print its sgid",
		Long: "Upload a file through Fizzy's direct upload and print the attachable sgid for rich text. " +
			"FILE must be a regular file of at most 100 MiB. A missing file, a folder, a device or a larger file exits 2 and sends nothing, in a dry run too. " +
			"The storage URL Fizzy names must be https, or a path on the configured server.",
		Example: examples("fz upload ~/Pictures/hinge.jpg"),
		Args:    args(cobra.ExactArgs(1)),
		RunE:    action(upload),
	})
}

func upload(c *cmdContext, a []string) error {
	path := config.ExpandTilde(a[0])
	fi, err := os.Stat(path)
	if err != nil {
		return usagef("%v", err)
	}
	if fi.IsDir() {
		return usagef("%s is a folder, not a file", path)
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	sgid, err := fz.Upload(path)
	if err != nil {
		return err
	}
	return emit(c.cmd, obj{{"sgid", orNull(sgid)}}, sgid)
}
