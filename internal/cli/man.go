package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func init() { register(addManCommand) }

// helpSection is one titled block of the root help and of man fz.
type helpSection struct {
	title string
	body  string
}

// rootSections holds the text that man fz shows after the command list.
// The draft format follows the package comment in internal/draft.
var rootSections = []helpSection{
	{"DRAFTS", `A draft is one Markdown file per card. fz draft check, preview and apply read it.

    ---
    board: Home
    column: Next
    tags: build, buy
    golden: false
    ---
    # Fix the gate

    ## Goal
    One line on what done looks like.

    ## Links & media
    - [Install guide](https://example.com/guide)
    ![Hinge](hinge.jpg)

    ## Steps
    - [ ] Measure the hinge
    - [x] Order the hinge

The board is required. The column, tags and golden lines are optional. Everything except the title and the Steps section becomes the card description. Each step becomes a Fizzy step. Relative image paths resolve against the draft's folder, then the inbox. After a successful apply, fz writes card: N back into the front matter so a draft is never created twice. It also writes synced:, a fingerprint of the card as Fizzy then holds it, so fz draft apply --update can tell when someone changed the card in Fizzy, and synced-fields:, a short hash of each part of the card, so the refusal can name what changed.`},
	{"FILES", `- ~/.config/fizzy-skill/config.json holds url, account, token_cmd, inbox and rules_card. The token itself comes only from FIZZY_TOKEN, or from the command that FIZZY_TOKEN_CMD or the token_cmd key names.
- ~/.local/state/fizzy-skill/actions.log gets one JSON line per write with the time, method, path and status. It never holds the token.
- ~/.fizzy-inbox/ is the default inbox. Draft images resolve against the draft's folder, then here.`},
	{"ENVIRONMENT", `- FIZZY_CONFIG is the path of the config file.
- FIZZY_URL is the server URL, for example https://fizzy.example.com.
- FIZZY_ACCOUNT is the account number, the N in https://fizzy.example.com/N/boards.
- FIZZY_TOKEN is the access token.
- FIZZY_TOKEN_CMD is a command that prints the token, for example a keychain lookup.
- FIZZY_INBOX is the inbox folder.
- FIZZY_RULES_CARD is the number of the card that holds your own conventions.`},
	{"EXIT STATUS", `- 0 means success.
- 1 means an API, config or draft error. The message says which.
- 2 means a usage error or any refusal. fz refuses a change that needs --yes when run without --yes, a name that matches more than one user, and a change it cannot check first. A refusal changes nothing.`},
}

const rootIntro = "fz is a command-line client for the Fizzy API. The server URL, account and token come from the config file or from environment variables."

// helpLong is the root Long text for fz --help.
func helpLong() string {
	var b strings.Builder
	b.WriteString(rootIntro)
	for _, s := range rootSections {
		fmt.Fprintf(&b, "\n\n%s\n\n%s", s.title, s.body)
	}
	return b.String()
}

// manLong is the root Long text for the man page. A "# TITLE" line becomes
// a section heading there.
func manLong() string {
	var b strings.Builder
	b.WriteString(rootIntro)
	for _, s := range rootSections {
		fmt.Fprintf(&b, "\n\n# %s\n\n%s", s.title, s.body)
	}
	return b.String()
}

func addManCommand(root *cobra.Command) {
	root.Long = helpLong()
	man := &cobra.Command{
		Use:   "man",
		Short: "install man pages",
		Long:  "Write one man page per command into DIR/man1, for example fz.1 and fz-card-list.1. The default DIR is ~/.local/share/man. This command needs no config file.",
		Example: examples(
			"fz man",
			"fz man --dir /tmp/fz-man",
		),
		Args: args(cobra.NoArgs),
		RunE: action(manInstall),
	}
	man.Flags().String("dir", "", "man directory (default ~/.local/share/man)")
	root.AddCommand(man)
}

func manInstall(c *cmdContext, _ []string) error {
	dir := flagValue(c.cmd, "dir")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot find the home folder: %v", err)
		}
		dir = filepath.Join(home, ".local", "share", "man")
	}
	if flagValue(c.cmd, "dry-run") == "true" {
		_, err := fmt.Fprintf(c.out(), "would write man pages to %s\n", filepath.Join(dir, "man1"))
		return err
	}
	n, err := writeManPages(c.cmd.Root(), dir)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.out(), "wrote %d man pages to %s\n", n, filepath.Join(dir, "man1"))
	return err
}

// writeManPages writes DIR/man1/<command>.1 for root and every available
// command below it. The pages hold no date, so two runs give the same bytes.
func writeManPages(root *cobra.Command, dir string) (int, error) {
	saved := root.Long
	root.Long = manLong()
	defer func() { root.Long = saved }()

	man1 := filepath.Join(dir, "man1")
	if err := os.MkdirAll(man1, 0o755); err != nil {
		return 0, err
	}
	header := &doc.GenManHeader{Section: "1", Source: "fz " + root.Version, Manual: "User Commands"}
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		secs, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid SOURCE_DATE_EPOCH: %v", err)
		}
		t := time.Unix(secs, 0).UTC()
		header.Date = &t
	}
	count := 0
	var gen func(cmd *cobra.Command) error
	gen = func(cmd *cobra.Command) error {
		for _, sub := range cmd.Commands() {
			if !sub.IsAvailableCommand() || sub.IsAdditionalHelpTopicCommand() {
				continue
			}
			if err := gen(sub); err != nil {
				return err
			}
		}
		h := *header
		h.Title = strings.ToUpper(strings.ReplaceAll(cmd.CommandPath(), " ", "-"))
		if cmd.Root() == cmd {
			h.Title = "FZ"
		}
		var buf bytes.Buffer
		if err := doc.GenMan(cmd, &h, &buf); err != nil {
			return err
		}
		name := strings.ReplaceAll(cmd.CommandPath(), " ", "-") + ".1"
		if err := os.WriteFile(filepath.Join(man1, name), dropManDate(buf.Bytes()), 0o644); err != nil {
			return err
		}
		count++
		return nil
	}
	root.DisableAutoGenTag = true
	if err := gen(root); err != nil {
		return 0, err
	}
	return count, nil
}

// dropManDate blanks the date field of the .TH line. Cobra fills it with the
// current month when SOURCE_DATE_EPOCH is unset, which would make every run
// differ. With SOURCE_DATE_EPOCH set the date stays, so builds stay
// reproducible.
func dropManDate(page []byte) []byte {
	if os.Getenv("SOURCE_DATE_EPOCH") != "" {
		return page
	}
	start := bytes.Index(page, []byte("\n.TH "))
	if start < 0 {
		return page
	}
	start++
	end := start + bytes.IndexByte(page[start:], '\n')
	line := page[start:end]
	fields := bytes.Split(line, []byte(`" "`))
	if len(fields) != 5 {
		return page
	}
	fields[2] = nil
	out := append([]byte{}, page[:start]...)
	out = append(out, bytes.Join(fields, []byte(`" "`))...)
	return append(out, page[end:]...)
}
