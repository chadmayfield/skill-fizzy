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
		Use:   "apply DRAFT...",
		Short: "create cards from drafts, or --update them",
		Long: "Create a card for each draft and write card: N and synced: back into it. " +
			"card: N is written right after the create, before any step, tag, move or golden call, so a draft never makes a second card. " +
			"If a later call fails, fz names the card and what was not sent; fz draft apply --update FILE finishes it. " +
			"Drafts that already have card: are skipped, unless --update is given.\n\n" +
			"With --update, a draft with card: N pushes its board, title, description, tags, column, golden mark and steps to card N. " +
			"Matching steps are left alone, a draft tick ticks a card step, and nothing is ever unticked. " +
			"New draft steps are added. Card steps and tags the draft does not have are kept and reported; " +
			"--prune deletes the unticked steps and removes the tags. A closed card is never reopened. " +
			"Images are uploaded again on every update.\n\n" +
			"After every apply, fz writes a fingerprint of the card's title, description, steps, tags, golden mark, " +
			"column, board, closed state and Not now state into the draft's synced: line, and a short hash of each into synced-fields:. " +
			"If the card changed in Fizzy since then, or the draft has no synced: fingerprint, --update refuses that draft unless --force is given. " +
			"The refusal names the fields that changed when synced-fields: is there.",
		Example: examples("fz draft apply ~/.fizzy-inbox/drafts/batch/", "fz --dry-run draft apply gate.md",
			"fz draft apply --update gate.md", "fz draft apply --update --prune gate.md"),
		Args: args(cobra.MinimumNArgs(1)),
		RunE: action(draftApply),
	}
	apply.Flags().Bool("update", false, "push changes to cards that drafts already made")
	apply.Flags().Bool("prune", false, "with --update, delete unticked steps and tags the draft no longer has")
	apply.Flags().Bool("force", false, "with --update, overwrite a card changed in Fizzy since the last sync")
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
// A draft named more than once, by any path or through a symlink, is kept
// once, where it first appears.
func draftPaths(targets []string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	add := func(p string) {
		key := draftKey(p)
		if !seen[key] {
			seen[key] = true
			paths = append(paths, p)
		}
	}
	for _, t := range targets {
		p := pyPath(config.ExpandTilde(t))
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			add(p)
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if ok, _ := filepath.Match("*.md", e.Name()); ok {
				add(pyPath(p + "/" + e.Name()))
			}
		}
	}
	return paths, nil
}

// draftKey is the cleaned absolute path of p with symlinks resolved, or as
// much of that as the file system allows for a path that does not exist.
func draftKey(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
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

// parseDraft parses a draft for preview and apply. A front matter value fz
// will not guess at, such as card: 87abc, is refused with exit 2.
func parseDraft(path string) (*draft.Draft, error) {
	d, err := draft.Parse(path)
	if errors.Is(err, draft.ErrBadValue) {
		return nil, usageError{err}
	}
	return d, err
}

func draftPreview(c *cmdContext, a []string) error {
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	w := c.out()
	for _, p := range paths {
		d, err := parseDraft(p)
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

// unfinished is a failure after the card was created and card: written
// into the draft: one follow-up call failed, and the ones after it were
// not sent.
type unfinished struct {
	n       int
	failed  string   // what failed, such as "adding its steps"
	notSent []string // what was not sent after it
	err     error
}

func (u *unfinished) Error() string {
	msg := fmt.Sprintf("card #%d was created, but %s failed: %s", u.n, u.failed, errorText(u.err))
	if len(u.notSent) > 0 {
		msg += "; not sent: " + strings.Join(u.notSent, ", ")
	}
	return msg
}

func (u *unfinished) Unwrap() error { return u.err }

// applyDraft creates the card for one draft and returns its number, or 0 in
// dry-run mode, where it prints the follow-up calls against a placeholder
// card instead. Right after the create it writes card: N into the draft, so
// a failure in a later call can never lead to a second card. A failure
// after that returns the card number with an *unfinished error.
func applyDraft(c *cmdContext, fz *fizzy.Fizzy, d *draft.Draft, inbox string) (int, error) {
	board, err := fz.Board(d.Board)
	if err != nil {
		return 0, err
	}
	if !isMaybe(d.Column) {
		if err := fz.CheckMoveTarget(board.ID, d.Column); err != nil {
			return 0, err
		}
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
	if c.dryRun {
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
	if n == 0 {
		return 0, fmt.Errorf("%s: the server's reply to the create had no card number (no Location header), "+
			"so a card may exist that the draft does not name; look on board %s before you apply it again", filepath.Base(d.Path), d.Board)
	}
	if err := draft.RecordCard(d.Path, n); err != nil {
		return n, fmt.Errorf("card #%d was created, but fz could not write card: %d into %s (%v); "+
			"add that line to the front matter by hand before you apply the draft again", n, n, d.Path, err)
	}
	// Each call has what it does, for the failure, and what it sends, for
	// the list of what was not sent.
	type call struct {
		what, sends string
		run         func() error
	}
	var calls []call
	if len(d.Steps) > 0 {
		calls = append(calls, call{"adding its steps", "the steps", func() error {
			for _, s := range d.Steps {
				if _, err := fz.AddStep(n, s.Content, s.Done); err != nil {
					return err
				}
			}
			return nil
		}})
	}
	if len(d.Tags) > 0 {
		calls = append(calls, call{"adding its tags", "the tags", func() error {
			for _, t := range d.Tags {
				if _, err := fz.AddTag(n, t); err != nil {
					return err
				}
			}
			return nil
		}})
	}
	if !isMaybe(d.Column) {
		calls = append(calls, call{"moving it to " + d.Column, "the move to " + d.Column, func() error { return fz.MoveCard(n, d.Column, board.ID) }})
	}
	if d.Golden {
		calls = append(calls, call{"marking it golden", "the golden mark", func() error { return fz.SetGolden(n, true) }})
	}
	for i, cl := range calls {
		if err := cl.run(); err != nil {
			u := &unfinished{n: n, failed: cl.what, err: err}
			for _, rest := range calls[i+1:] {
				u.notSent = append(u.notSent, rest.sends)
			}
			return n, u
		}
	}
	return n, nil
}

// shellArg quotes a path for a command line the user can paste.
func shellArg(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-~+=,:@") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// failApply prints err the way the root command would, then the summary of
// a run over more than one draft, and returns the exit code err gets.
func failApply(c *cmdContext, err error, summary []string) error {
	fmt.Fprintf(c.errw(), "fz: %s\n", errorText(err))
	if len(summary) > 1 {
		fmt.Fprintln(c.errw(), "fz: stopped. The drafts in this run:")
		for _, line := range summary {
			fmt.Fprintf(c.errw(), "  %s\n", line)
		}
	}
	var u usageError
	if errors.As(err, &u) || fizzy.IsRefusal(err) {
		return exitCode(2)
	}
	return exitCode(1)
}

func draftApply(c *cmdContext, a []string) error {
	update, _ := c.cmd.Flags().GetBool("update")
	prune, _ := c.cmd.Flags().GetBool("prune")
	force, _ := c.cmd.Flags().GetBool("force")
	if !update {
		if prune {
			return usagef("--prune needs --update")
		}
		if force {
			return usagef("--force needs --update")
		}
	}
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
	// Validate every draft first: a parse error, a missing image or a draft
	// fz could not write card: back into stops the batch before any card is
	// created.
	var drafts []*draft.Draft
	for _, p := range paths {
		d, err := parseDraft(p)
		if err != nil {
			return err
		}
		if err := d.CheckImages(cfg.InboxPath()); err != nil {
			return err
		}
		if d.Card == 0 || update {
			if err := draft.CheckWritable(p); err != nil {
				return fmt.Errorf("%s: fz cannot write card: and synced: back into this draft (%v), so it sends nothing", p, err)
			}
		}
		drafts = append(drafts, d)
	}
	// With --update, check every card for changes made in Fizzy before any
	// write, so one refusal reports them all and nothing is half applied.
	if update && !force {
		refused, err := guardDrafts(fz, drafts)
		if err != nil {
			return err
		}
		for _, r := range refused {
			fmt.Fprintf(c.errw(), "fz: %s\n", r)
		}
		if len(refused) > 0 {
			return exitCode(1)
		}
	}
	// summary has one line per draft for the report printed if the run
	// stops early.
	summary := make([]string, len(drafts))
	for i, d := range drafts {
		summary[i] = filepath.Base(d.Path) + ": not reached"
	}
	w := c.out()
	for i, d := range drafts {
		name := filepath.Base(d.Path)
		if d.Card != 0 {
			if !update {
				summary[i] = fmt.Sprintf("%s: card #%d, skipped", name, d.Card)
				fmt.Fprintf(w, "skip %s: already card #%d (use --update to push changes)\n", name, d.Card)
				continue
			}
			say := func(line string) {
				if c.dryRun {
					line = dryRunLine(line)
				}
				fmt.Fprintf(w, "#%d %s\n", d.Card, line)
			}
			_, err := syncDraft(fz, d, cfg.InboxPath(), prune, force, say)
			var partial *partialSync
			if errors.As(err, &partial) && !c.dryRun {
				summary[i] = fmt.Sprintf("%s: card #%d, update not finished", name, d.Card)
				// The writes printed above went through. Record synced: from
				// the card as it is now, so the retry needs no --force; the
				// draft's changes not yet sent still differ from the card,
				// so the retry sends them.
				finish := "fz draft apply --update " + shellArg(d.Path)
				msg := fmt.Errorf("card #%d: the update stopped partway: %w\nfz: synced: is recorded from the card as it is now. Finish it with: %s",
					d.Card, err, finish)
				if rerr := recordSynced(fz, d.Path, d.Card); rerr != nil {
					msg = fmt.Errorf("card #%d: the update stopped partway: %w\nfz: reading the card back failed (%s), so synced: is unchanged; check the card, then finish it with: %s --force",
						d.Card, err, errorText(rerr), finish)
				}
				return failApply(c, msg, summary)
			}
			if err != nil {
				summary[i] = fmt.Sprintf("%s: card #%d, not updated", name, d.Card)
				return failApply(c, err, summary)
			}
			if !c.dryRun {
				if err := recordSynced(fz, d.Path, d.Card); err != nil {
					summary[i] = fmt.Sprintf("%s: card #%d, updated, synced: not recorded", name, d.Card)
					return failApply(c, err, summary)
				}
			}
			summary[i] = fmt.Sprintf("%s: card #%d, updated", name, d.Card)
			continue
		}
		n, err := applyDraft(c, fz, d, cfg.InboxPath())
		var u *unfinished
		switch {
		case errors.As(err, &u):
			summary[i] = fmt.Sprintf("%s: card #%d, not finished", name, n)
			// Record synced: from the card as it is now, so the retry with
			// --update needs no --force.
			finish := "fz draft apply --update " + shellArg(d.Path)
			msg := fmt.Errorf("%w\nfz: card: %d is written into %s. Finish it with: %s", err, n, name, finish)
			if rerr := recordSynced(fz, d.Path, n); rerr != nil {
				msg = fmt.Errorf("%w\nfz: card: %d is written into %s, but reading the card back failed (%s). Finish it with: fz draft apply --update --force %s",
					err, n, name, errorText(rerr), shellArg(d.Path))
			}
			return failApply(c, msg, summary)
		case err != nil && n != 0:
			summary[i] = fmt.Sprintf("%s: card #%d, card: not written", name, n)
			return failApply(c, err, summary)
		case err != nil:
			summary[i] = name + ": no card created"
			return failApply(c, err, summary)
		}
		if n == 0 {
			summary[i] = name + ": dry run"
			fmt.Fprintf(w, "[dry-run] %s → %s / %s\n", d.Title, d.Board, d.Column)
			continue
		}
		summary[i] = fmt.Sprintf("%s: card #%d", name, n)
		if err := recordSynced(fz, d.Path, n); err != nil {
			summary[i] = fmt.Sprintf("%s: card #%d, synced: not recorded", name, n)
			return failApply(c, fmt.Errorf("card #%d was created and card: %d written into %s, but reading it back for synced: failed (%s); the next --update needs --force",
				n, n, name, errorText(err)), summary)
		}
		fmt.Fprintf(w, "#%d %s → %s / %s   %s/%s/cards/%d\n", n, d.Title, d.Board, d.Column, cfg.URL, cfg.Account, n)
	}
	return nil
}

// recordSynced writes the card's fingerprint into the draft as synced:,
// and its per-field hashes as synced-fields:, read after every write so the
// next --update can tell whether someone changed the card in Fizzy since,
// and which fields.
func recordSynced(fz *fizzy.Fizzy, path string, n int) error {
	// Known limit: an edit made in Fizzy between fz's writes and this read is
	// folded into the new fingerprint, so the next --update cannot see it.
	card, err := fz.Card(n)
	if err != nil {
		return err
	}
	return draft.RecordFields(path,
		draft.KV{Key: "synced", Value: cardFingerprint(card)},
		draft.KV{Key: "synced-fields", Value: syncedFields(card)})
}
