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

The board is required. The column, tags and golden lines are optional. Tags are comma-separated, with or without [ ] around them, and quotes around a tag are dropped. golden: takes true, yes, on or 1, or false, no, off or 0. card: takes a card number, 87 or #87, and fz refuses any other value with exit 2. A # after a space starts a comment unless it is inside quotes. Everything except the title and the Steps section becomes the card description. Each step becomes a Fizzy step. Relative image paths resolve against the draft's folder, then the inbox. Right after it creates the card, before any step, tag, move or golden call, fz writes card: N back into the front matter so a draft is never created twice. If a later call fails, fz names the card and what was not sent, and fz draft apply --update FILE finishes it. It also writes synced:, a fingerprint of the card as Fizzy then holds it, so fz draft apply --update can tell when someone changed the card in Fizzy, and synced-fields:, a short hash of each part of the card, so the refusal can name what changed.`},
	{"FILES", `- ~/.config/fizzy-skill/config.json holds url, account, token_cmd, inbox and rules_card. It must be a regular file of at most 1 MiB. The token itself comes only from FIZZY_TOKEN, or from the command that FIZZY_TOKEN_CMD or the token_cmd key names.
- ~/.local/state/fizzy-skill/actions.log gets one JSON line per write with the time, method, path and status. For the upload of a file's bytes, the path is the storage host only. It never holds the token.
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
- 2 means a usage error or any refusal. fz refuses a change that needs --yes when run without --yes, a name that matches more than one user, board, column or tag, and a change it cannot check first. A refusal changes nothing.`},
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
		Long: "Write one man page per command into DIR/man1, for example fz.1 and fz-card-list.1. The default DIR is ~/.local/share/man. This command needs no config file. " +
			"If a page path is a symlink, fz man writes nothing and exits 1. After writing, it removes each fz.1 or fz-*.1 file in DIR/man1 that it did not write, such as the page of a removed command, and prints its path.",
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
	n, removed, err := writeManPages(c.cmd.Root(), dir)
	if err != nil {
		return err
	}
	for _, p := range removed {
		fmt.Fprintf(c.out(), "removed %s\n", p)
	}
	_, err = fmt.Fprintf(c.out(), "wrote %d man pages to %s\n", n, filepath.Join(dir, "man1"))
	return err
}

// writeManPages writes DIR/man1/<command>.1 for root and every command
// below it, then removes stale fz.1 and fz-*.1 pages and returns their paths.
// Pages are written by rename and never through a symlink.
func writeManPages(root *cobra.Command, dir string) (int, []string, error) {
	saved := root.Long
	root.Long = manLong()
	defer func() { root.Long = saved }()

	man1 := filepath.Join(dir, "man1")
	header := &doc.GenManHeader{Section: "1", Source: "fz " + root.Version, Manual: "User Commands"}
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		secs, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid SOURCE_DATE_EPOCH: %v", err)
		}
		t := time.Unix(secs, 0).UTC()
		header.Date = &t
	}
	type page struct {
		name string
		body []byte
	}
	var pages []page
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
		pages = append(pages, page{strings.ReplaceAll(cmd.CommandPath(), " ", "-") + ".1", dropManDate(buf.Bytes())})
		return nil
	}
	root.DisableAutoGenTag = true
	if err := gen(root); err != nil {
		return 0, nil, err
	}
	if err := os.MkdirAll(man1, 0o755); err != nil {
		return 0, nil, err
	}
	written := map[string]bool{}
	for _, p := range pages {
		path := filepath.Join(man1, p.name)
		fi, err := os.Lstat(path)
		switch {
		case err == nil && fi.Mode()&os.ModeSymlink != 0:
			return 0, nil, fmt.Errorf("%s is a symlink; fz man does not write through one, so it wrote nothing", path)
		case err == nil && !fi.Mode().IsRegular():
			return 0, nil, fmt.Errorf("%s is not a regular file, so fz man wrote nothing", path)
		case err != nil && !os.IsNotExist(err):
			return 0, nil, err
		}
		written[p.name] = true
	}
	for _, p := range pages {
		if err := writeReplacing(filepath.Join(man1, p.name), p.body); err != nil {
			return 0, nil, err
		}
	}
	entries, err := os.ReadDir(man1)
	if err != nil {
		return 0, nil, err
	}
	var removed []string
	for _, e := range entries {
		name := e.Name()
		ours := name == "fz.1" || (strings.HasPrefix(name, "fz-") && strings.HasSuffix(name, ".1"))
		if !ours || written[name] || e.IsDir() {
			continue
		}
		path := filepath.Join(man1, name)
		if err := os.Remove(path); err != nil {
			return 0, nil, err
		}
		removed = append(removed, path)
	}
	return len(pages), removed, nil
}

// writeReplacing writes data to a new file next to path, with mode 0644,
// and renames it over path. A symlink at path is replaced, not followed.
func writeReplacing(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fz-man-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(name, 0o644)
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
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
