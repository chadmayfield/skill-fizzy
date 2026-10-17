package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/draft"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

func init() { register(addDraftCommands) }

func addDraftCommands(root *cobra.Command) {
	g := &cobra.Command{
		Use:   "draft",
		Short: "create cards from Markdown drafts",
		Long:  "A draft is one Markdown file per card. See DRAFTS in man fz for the format.",
	}
	root.AddCommand(g)
	check := &cobra.Command{
		Use:     "check DRAFT...",
		Short:   "validate drafts, and --links",
		Long:    "Parse each draft, resolve its images, and with --links fetch every link.",
		Example: examples("fz draft check ~/.fizzy-inbox/drafts/", "fz draft check gate.md --links"),
		Args:    args(cobra.MinimumNArgs(1)),
		RunE:    action(draftCheck),
	}
	check.Flags().Bool("links", false, "fetch every link and report its status")
	preview := &cobra.Command{
		Use:     "preview DRAFT...",
		Short:   "show the card a draft would make",
		Long:    "Print the title, board, column, tags, description HTML and steps. Sends nothing.",
		Example: examples("fz draft preview gate.md"),
		Args:    args(cobra.MinimumNArgs(1)),
		RunE:    action(draftPreview),
	}
	apply := &cobra.Command{
		Use:     "apply DRAFT...",
		Short:   "create cards from drafts",
		Long:    "Create a card for each draft and write card: N back into it. Drafts that already have card: are skipped.",
		Example: examples("fz draft apply ~/.fizzy-inbox/drafts/batch/", "fz --dry-run draft apply gate.md"),
		Args:    args(cobra.MinimumNArgs(1)),
		RunE:    action(draftApply),
	}
	g.AddCommand(check, preview, apply)
}

// pyPath writes a path the way Python's str(Path(p)) does: repeated and
// trailing slashes and "." parts dropped, ".." kept.
func pyPath(p string) string {
	var parts []string
	for _, s := range strings.Split(p, "/") {
		if s != "" && s != "." {
			parts = append(parts, s)
		}
	}
	out := strings.Join(parts, "/")
	if strings.HasPrefix(p, "/") {
		return "/" + out
	}
	if out == "" {
		return "."
	}
	return out
}

// draftPaths expands targets: a folder becomes its *.md files in name order.
func draftPaths(targets []string) ([]string, error) {
	var paths []string
	for _, t := range targets {
		p := pyPath(config.ExpandTilde(t))
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			paths = append(paths, p)
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if ok, _ := filepath.Match("*.md", e.Name()); ok {
				paths = append(paths, pyPath(p+"/"+e.Name()))
			}
		}
	}
	return paths, nil
}

// linkStatus is an HTTP status, or the name of the error that stopped the
// request.
type linkStatus struct {
	code int
	err  string
}

func (s linkStatus) ok() bool { return s.err == "" && s.code < 400 }

func (s linkStatus) String() string {
	if s.err != "" {
		return s.err
	}
	return strconv.Itoa(s.code)
}

// checkLink GETs url, following redirects. Error names follow Python's
// urllib: a timeout is TimeoutError, any other failure is URLError.
func checkLink(hc *http.Client, url string) linkStatus {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return linkStatus{err: "ValueError"}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (fizzy-skill link check)")
	resp, err := hc.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return linkStatus{err: "TimeoutError"}
		}
		return linkStatus{err: "URLError"}
	}
	resp.Body.Close()
	return linkStatus{code: resp.StatusCode}
}

func draftCheck(c *cmdContext, a []string) error {
	cfg, err := c.config()
	if err != nil {
		return err
	}
	links, _ := c.cmd.Flags().GetBool("links")
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: 15 * time.Second}
	w := c.out()
	bad := 0
	for _, p := range paths {
		d, err := draft.Parse(p)
		if err == nil {
			err = d.CheckImages(cfg.InboxPath())
		}
		if errors.Is(err, draft.ErrDraft) {
			fmt.Fprintf(w, "✗ %s: %s\n", p, err)
			bad++
			continue
		}
		if err != nil {
			return err
		}
		line := fmt.Sprintf("✓ %s: %s → %s / %s, %d steps, %d images",
			filepath.Base(p), pyRepr(d.Title), d.Board, d.Column, len(d.Steps), len(d.Images()))
		if d.Card != 0 {
			line += fmt.Sprintf(" (already created: #%d)", d.Card)
		}
		fmt.Fprintln(w, line)
		if links {
			for _, u := range d.Links() {
				s := checkLink(hc, u)
				mark := "BAD"
				if s.ok() {
					mark = "ok "
				} else {
					bad++
				}
				fmt.Fprintf(w, "    %s %s %s\n", mark, s, u)
			}
		}
	}
	if bad > 0 {
		return exitCode(1)
	}
	return nil
}

func draftPreview(c *cmdContext, a []string) error {
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	w := c.out()
	for _, p := range paths {
		d, err := draft.Parse(p)
		if err != nil {
			return err
		}
		tags := strings.Join(d.Tags, ", ")
		if tags == "" {
			tags = "-"
		}
		fmt.Fprintf(w, "=== %s\nboard: %s   column: %s   tags: %s   golden: %s\n", d.Title, d.Board, d.Column, tags, pyBool(d.Golden))
		fmt.Fprintln(w, d.DescriptionHTML(nil))
		for _, s := range d.Steps {
			fmt.Fprintf(w, "  [%s] %s\n", tick(s.Done), s.Content)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// applyDraft creates the card for one draft and returns its number, or 0 in
// dry-run mode, where it prints the follow-up calls against a placeholder
// card instead.
func applyDraft(c *cmdContext, fz *fizzy.Fizzy, d *draft.Draft, inbox string) (int, error) {
	board, err := fz.Board(d.Board)
	if err != nil {
		return 0, err
	}
	sgids := map[string]string{}
	for _, img := range d.Images() {
		path, err := d.ResolveImage(img, inbox)
		if err != nil {
			return 0, err
		}
		if sgids[img], err = fz.Upload(path); err != nil {
			return 0, err
		}
	}
	n, err := fz.CreateCard(board.ID, d.Title, d.DescriptionHTML(sgids), "")
	if err != nil {
		return 0, err
	}
	if n == 0 {
		for _, s := range d.Steps {
			if _, err := fz.AddStep(0, s.Content, s.Done); err != nil {
				return 0, err
			}
		}
		for _, t := range d.Tags {
			fmt.Fprintf(c.errw(), "[dry-run] tag new card: %s\n", t)
		}
		if !isMaybe(d.Column) {
			fmt.Fprintf(c.errw(), "[dry-run] move new card to column: %s\n", d.Column)
		}
		if d.Golden {
			fmt.Fprintln(c.errw(), "[dry-run] mark new card golden")
		}
		return 0, nil
	}
	for _, s := range d.Steps {
		if _, err := fz.AddStep(n, s.Content, s.Done); err != nil {
			return 0, err
		}
	}
	for _, t := range d.Tags {
		if _, err := fz.AddTag(n, t); err != nil {
			return 0, err
		}
	}
	if !isMaybe(d.Column) {
		if err := fz.MoveCard(n, d.Column, board.ID); err != nil {
			return 0, err
		}
	}
	if d.Golden {
		if err := fz.SetGolden(n, true); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func draftApply(c *cmdContext, a []string) error {
	cfg, err := c.config()
	if err != nil {
		return err
	}
	fz, err := c.api()
	if err != nil {
		return err
	}
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	// Validate every draft first: a parse error stops the batch before any
	// card is created.
	var drafts []*draft.Draft
	for _, p := range paths {
		d, err := draft.Parse(p)
		if err != nil {
			return err
		}
		if err := d.CheckImages(cfg.InboxPath()); err != nil {
			return err
		}
		drafts = append(drafts, d)
	}
	w := c.out()
	for _, d := range drafts {
		if d.Card != 0 {
			fmt.Fprintf(w, "skip %s: already card #%d\n", filepath.Base(d.Path), d.Card)
			continue
		}
		n, err := applyDraft(c, fz, d, cfg.InboxPath())
		if err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintf(w, "[dry-run] %s → %s / %s\n", d.Title, d.Board, d.Column)
			continue
		}
		if err := draft.RecordCard(d.Path, n); err != nil {
			return err
		}
		fmt.Fprintf(w, "#%d %s → %s / %s   %s/%s/cards/%d\n", n, d.Title, d.Board, d.Column, cfg.URL, cfg.Account, n)
	}
	return nil
}
