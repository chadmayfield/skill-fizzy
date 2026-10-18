package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chadmayfield/skill-fizzy/internal/client"
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
		Use:   "check DRAFT...",
		Short: "validate drafts, and --links",
		Long: "Parse each draft, resolve its images, and with --links fetch every link outside code. " +
			"It needs no server URL or account. " +
			"An image must be a path relative to the draft's folder or the inbox that stays inside one of them after symlinks are resolved; " +
			"any other image is refused with exit 2. " +
			"A link to a loopback, private, link-local, multicast, unspecified or other special-purpose address, such as 100.64.0.0/10, or one that redirects there, is skipped and printed as skipped (local address).",
		Example: examples("fz draft check ~/.fizzy-inbox/drafts/", "fz draft check gate.md --links"),
		Args:    args(cobra.MinimumNArgs(1)),
		RunE:    action(draftCheck),
	}
	check.Flags().Bool("links", false, "fetch every link and report its status")
	preview := &cobra.Command{
		Use:     "preview DRAFT...",
		Short:   "show the card a draft would make",
		Long:    "Print the title, board, column, tags, description HTML and steps. Sends nothing. An image path outside the draft's folder and the inbox is refused with exit 2.",
		Example: examples("fz draft preview gate.md"),
		Args:    args(cobra.MinimumNArgs(1)),
		RunE:    action(draftPreview),
	}
	apply := &cobra.Command{
		Use:   "apply DRAFT...",
		Short: "create cards from drafts, or --update them",
		Long: "Create a card for each draft and write card: N and synced: back into it. " +
			"Just before the create, fz writes creating: and the time into the draft. card: N replaces it right after the create, before any step, tag, move or golden call. " +
			"If fz cannot tell whether the create reached Fizzy, as after a timeout, the line stays and fz exits 1 and names the board to check. " +
			"A draft that still has creating: is refused with exit 2 until you replace the line with card: N or delete it, so a draft never makes a second card. " +
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
// A draft named more than once, by any path, through a symlink or through
// a second hard link, is kept once, where it first appears.
func draftPaths(targets []string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	var files []os.FileInfo
	add := func(p string) {
		key := draftKey(p)
		if seen[key] {
			return
		}
		seen[key] = true
		// os.SameFile compares the device and inode, so it also catches a
		// hard link, which has a path of its own.
		if st, err := os.Stat(p); err == nil {
			for _, f := range files {
				if os.SameFile(f, st) {
					return
				}
			}
			files = append(files, st)
		}
		paths = append(paths, p)
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

// errLocalAddress stops a link check that would connect to this machine
// or a private network.
var errLocalAddress = errors.New("local address")

// linkDialBlocked reports whether a link check may not connect to a. It is
// a variable so tests can let it reach their loopback servers.
var linkDialBlocked = func(a netip.AddrPort) bool { return linkAddrBlocked(a.Addr()) }

// linkBlockedPrefixes are the special-purpose ranges a link check never
// reaches, beyond the loopback, private, link-local, multicast and
// unspecified ones netip names.
var linkBlockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",       // this network
		"100.64.0.0/10",   // shared address space: CGNAT, Tailscale
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // documentation
		"192.88.99.0/24",  // 6to4 relay anycast
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // documentation
		"203.0.113.0/24",  // documentation
		"240.0.0.0/4",     // reserved, and the broadcast address
		"::/96",           // IPv4-compatible
		"64:ff9b:1::/48",  // local-use NAT64
		"100::/64",        // discard only
		"2001::/23",       // IETF protocol assignments, Teredo among them
		"2001:db8::/32",   // documentation
		"fec0::/10",       // site-local
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
)

// linkAddrBlocked reports whether ip is local, private or special-purpose.
// A NAT64 or 6to4 address is judged by the IPv4 address inside it.
func linkAddrBlocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, p := range linkBlockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	b := ip.As16()
	switch {
	case !ip.Is6():
	case nat64Prefix.Contains(ip):
		return linkAddrBlocked(netip.AddrFrom4([4]byte(b[12:16])))
	case sixToFour.Contains(ip):
		return linkAddrBlocked(netip.AddrFrom4([4]byte(b[2:6])))
	}
	return false
}

// linkClient is the HTTP client for draft check --links. It checks the
// address of every connection it makes, after DNS and on every redirect
// hop, and refuses local and private ones before it connects. It uses no
// proxy, so the address it checks is the one it reaches. It never sends a
// token or a cookie.
func linkClient() *http.Client {
	d := &net.Dialer{
		Timeout: 15 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || linkDialBlocked(ap) {
				return errLocalAddress
			}
			return nil
		},
	}
	tr := &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSHandshakeTimeout: 15 * time.Second}
	return &http.Client{Timeout: 15 * time.Second, Transport: tr}
}

// linkStatus is an HTTP status, or the name of the error that stopped the
// request. skipped is set when the link, or a redirect from it, points at a
// local or private address, which fz does not fetch.
type linkStatus struct {
	code    int
	err     string
	skipped bool
}

func (s linkStatus) ok() bool { return s.skipped || (s.err == "" && s.code < 400) }

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
	if errors.Is(err, errLocalAddress) {
		return linkStatus{skipped: true}
	}
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

// refuseImage turns an image target the draft package refuses into a
// Refusal, so the command exits 2.
func refuseImage(err error) error {
	if errors.Is(err, draft.ErrImagePath) {
		return fizzy.Refuse(err.Error())
	}
	return err
}

func draftCheck(c *cmdContext, a []string) error {
	inbox, err := c.inbox()
	if err != nil {
		return err
	}
	links, _ := c.cmd.Flags().GetBool("links")
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	hc := linkClient()
	w := c.out()
	bad, refused := 0, 0
	for _, p := range paths {
		d, err := draft.Parse(p)
		if err == nil {
			err = d.CheckImages(inbox)
		}
		if errors.Is(err, draft.ErrDraft) {
			fmt.Fprintf(w, "✗ %s: %s\n", p, err)
			if errors.Is(err, draft.ErrImagePath) || errors.Is(err, draft.ErrTooLarge) {
				refused++
			}
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
				if s.skipped {
					fmt.Fprintf(w, "    skipped (local address) %s\n", u)
					continue
				}
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
	// A refused image path is a refusal, so it exits 2, as preview and
	// apply do for it.
	if refused > 0 {
		return exitCode(2)
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
	if errors.Is(err, draft.ErrBadValue) || errors.Is(err, draft.ErrTooLarge) {
		return nil, usageError{err}
	}
	return d, err
}

func draftPreview(c *cmdContext, a []string) error {
	paths, err := draftPaths(a)
	if err != nil {
		return err
	}
	inbox, err := c.inbox()
	if err != nil {
		return err
	}
	w := c.out()
	for _, p := range paths {
		d, err := parseDraft(p)
		if err != nil {
			return err
		}
		if err := d.CheckImagePaths(inbox); err != nil {
			return refuseImage(err)
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
	if !c.dryRun {
		// creating: marks a create that may have reached Fizzy. It is
		// replaced by card: on success and removed when the server
		// refused the create; an unknown outcome leaves it, and the next
		// apply refuses the draft, so it cannot make a second card.
		if err := draft.RecordCreating(d.Path, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return 0, fmt.Errorf("%s: fz could not write creating: into the draft (%v), so it sent no create", d.Path, err)
		}
	}
	n, err := fz.CreateCard(board.ID, d.Title, d.DescriptionHTML(sgids), "")
	if c.dryRun {
		if err != nil {
			return 0, err
		}
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
	switch {
	case errors.Is(err, fizzy.ErrNoLocation):
		return 0, &createUnknown{fmt.Sprintf("%s: the server's reply to the create had no card number (%s), "+
			"so a card may exist that the draft does not name", filepath.Base(d.Path), errorText(err)), d}
	case err != nil && createNotSent(err):
		if cerr := draft.ClearCreating(d.Path); cerr != nil {
			return 0, fmt.Errorf("%w; no card was created, but fz could not remove the creating: line from %s (%v); delete it by hand", err, d.Path, cerr)
		}
		return 0, err
	case err != nil:
		return 0, &createUnknown{fmt.Sprintf("%s: the create may have reached Fizzy (%s), so a card may exist that the draft does not name",
			filepath.Base(d.Path), errorText(err)), d}
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

// createUnknown is a card create whose outcome fz cannot tell: the request
// may have reached Fizzy, and a card may exist. The creating: line stays in
// the draft, so the next apply refuses it.
type createUnknown struct {
	what string
	d    *draft.Draft
}

func (e *createUnknown) Error() string {
	return fmt.Sprintf("%s. Look on board %s. The draft keeps its creating: line, so fz will not apply it again until you resolve it: "+
		"if the card is there, replace that line with card: N; if it is not, delete the line. Then apply the draft again", e.what, e.d.Board)
}

// createNotSent reports whether a failed card create surely made no card:
// the server refused it with a 4xx, or the request never left fz. Any
// other failure, such as a timeout, a reset, a 5xx or a 3xx, may have made
// one. A 3xx counts because a server can answer a create that ran with a
// redirect to the new record.
func createNotSent(err error) bool {
	var api *client.APIError
	if errors.As(err, &api) {
		return api.Status >= 400 && api.Status < 500
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	return errors.Is(err, config.ErrConfig) || fizzy.IsRefusal(err)
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
		if at, ok := d.Meta[draft.CreatingKey]; ok {
			return fizzy.Refuse(fmt.Sprintf("%s: the draft has creating: %s, left by an apply whose create may have made its card. "+
				"Look on board %s. If the card is there, replace the line with card: N; if it is not, delete the line. fz sends nothing until then",
				p, at, d.Board))
		}
		if err := d.CheckImages(cfg.InboxPath()); err != nil {
			return refuseImage(err)
		}
		if d.Card == 0 || update {
			if err := draft.CheckWritable(p); err != nil {
				msg := fmt.Sprintf("%s: fz cannot write card: and synced: back into this draft (%v), so it sends nothing", p, err)
				if errors.Is(err, draft.ErrLinked) {
					return fizzy.Refuse(msg)
				}
				return errors.New(msg)
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
		// A conflict is a refusal, so it exits 2.
		if len(refused) > 0 {
			return exitCode(2)
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
		var unknown *createUnknown
		switch {
		case errors.As(err, &unknown):
			summary[i] = name + ": a card may exist, creating: kept"
			return failApply(c, err, summary)
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
