package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/draft"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
)

// lane normalizes a column name for comparison: no column is Maybe?, case
// and a trailing ? are ignored, and "not-now" is the same lane as "Not now".
func lane(name string) string {
	if name == "" {
		name = "maybe"
	}
	l := strings.TrimRight(strings.ToLower(strings.TrimSpace(name)), "?")
	if l == "not-now" {
		l = "not now"
	}
	return l
}

// cardLane is the lane a card is in. Maybe? and Not now both have no
// column; only postponed tells them apart.
func cardLane(card fizzy.Card) string {
	switch {
	case card.Column != nil:
		return lane(card.Column.Name)
	case card.Postponed:
		return lane("Not now")
	}
	return lane("")
}

// checkSynced is the conflict guard. It refuses a draft whose synced: line
// is missing or is not a card fingerprint, or whose card's fingerprint
// differs from synced:, because then the card may hold changes made in Fizzy
// that the draft would overwrite. When the draft has a usable
// synced-fields: line, the refusal names the fields that changed. The
// error is a Refusal, so the command exits 2.
func checkSynced(d *draft.Draft, card fizzy.Card) error {
	name := filepath.Base(d.Path)
	switch {
	case d.Synced == "":
		return fizzy.Refuse(fmt.Sprintf("%s: no synced: line, so changes made in Fizzy cannot be detected; rerun with --force", name))
	case isTimestamp(d.Synced):
		return fizzy.Refuse(fmt.Sprintf("%s: synced: is a time written by an older fz, so changes made in Fizzy cannot be detected; rerun with --force", name))
	case !isFingerprint(d.Synced):
		return fizzy.Refuse(fmt.Sprintf("%s: synced: is not a card fingerprint, so changes made in Fizzy cannot be detected; rerun with --force", name))
	case cardFingerprint(card) != d.Synced:
		what := ""
		if fields := changedFields(d.Meta["synced-fields"], card); len(fields) > 0 {
			what = " (" + strings.Join(fields, ", ") + ")"
		}
		return fizzy.Refuse(fmt.Sprintf("%s: card #%d changed in Fizzy since the last sync%s; check it, then rerun with --force", name, d.Card, what))
	}
	return nil
}

// guardDrafts runs checkSynced for every draft that has a card, before any
// write, and returns every refusal in draft order.
func guardDrafts(fz *fizzy.Fizzy, drafts []*draft.Draft) ([]error, error) {
	var refused []error
	for _, d := range drafts {
		if d.Card == 0 {
			continue
		}
		card, err := fz.Card(d.Card)
		if err != nil {
			return nil, err
		}
		if err := checkSynced(d, card); err != nil {
			refused = append(refused, err)
		}
	}
	return refused, nil
}

// dryRunWords pairs each syncDraft report line that records a write with
// the words for the same change not yet made. Longer prefixes come first.
var dryRunWords = []struct{ done, would string }{
	{"moved to board ", "move to board "},
	{"moved to ", "move to "},
	{"title and description sent", "send the title and description"},
	{"step added: ", "add step: "},
	{"step ticked: ", "tick step: "},
	{"step deleted: ", "delete step: "},
	{"tag added: ", "add tag: "},
	{"tag removed: ", "remove tag: "},
	{"golden on", "turn golden on"},
	{"golden off", "turn golden off"},
}

// dryRunLine rewords a syncDraft report line for --dry-run, where nothing
// was sent. A line that records no write comes back as it is.
func dryRunLine(line string) string {
	for _, w := range dryRunWords {
		if rest, ok := strings.CutPrefix(line, w.done); ok {
			return "dry-run: nothing sent; would " + w.would + rest
		}
	}
	return line
}

// partialSync is a sync that failed after it began sending card writes.
// The writes before the failure may have gone through.
type partialSync struct{ err error }

func (p *partialSync) Error() string { return p.err.Error() }
func (p *partialSync) Unwrap() error { return p.err }

// syncDraft pushes a draft onto its existing card and returns one report
// line per change, also passed to say as it happens. Without force it refuses,
// before any write, when the card changed in Fizzy since synced:. An error
// after the first write is a *partialSync. Steps are never unticked; prune
// removes unticked steps and tags the draft lacks.
func syncDraft(fz *fizzy.Fizzy, d *draft.Draft, inbox string, prune, force bool, say func(string)) (report []string, err error) {
	return syncDraftHashed(fz, d, inbox, "", prune, force, say)
}

// syncDraftHashed is syncDraft with the draft's draftHash already taken,
// so a run reads each image once. An empty dh is taken here.
func syncDraftHashed(fz *fizzy.Fizzy, d *draft.Draft, inbox, dh string, prune, force bool, say func(string)) (report []string, err error) {
	card, err := fz.Card(d.Card)
	if err != nil {
		return report, err
	}
	if !force {
		if err := checkSynced(d, card); err != nil {
			return report, err
		}
	}
	add := func(line string) {
		report = append(report, line)
		if say != nil {
			say(line)
		}
	}
	// sending is set before the first card write, so a failure from then
	// on is reported as partial.
	sending := false
	defer func() {
		if err != nil && sending {
			err = &partialSync{err}
		}
	}()
	board, err := fz.Board(d.Board)
	if err != nil {
		return report, err
	}
	// After a board move the card's old column says nothing about where it
	// lands, so the column is always set on the new board.
	moveBoard := card.Board.ID != board.ID
	moveColumn := !card.Closed && (moveBoard || cardLane(card) != lane(d.Column))
	if moveColumn {
		if err := fz.CheckMoveTarget(board.ID, d.Column); err != nil {
			return report, err
		}
	}
	// When neither the draft's title, body and images nor the card's title
	// and description changed since the last sync, there is nothing to
	// upload or send for them.
	if dh == "" {
		if dh, err = draftHash(d, inbox); err != nil {
			return report, err
		}
	}
	// --force never skips: it is how a user puts the draft's text back.
	sameText := !force && d.Meta[syncedDraftKey] == dh && textSynced(d.Meta["synced-fields"], card)
	// Upload first, so a failed upload leaves the card untouched.
	sgids := map[string]string{}
	for _, img := range d.Images() {
		if sameText {
			break
		}
		path, err := d.ResolveImage(img, inbox)
		if err != nil {
			return report, err
		}
		// Name the file before it is sent, so the user sees which file
		// left the disk.
		add("image: " + line(path))
		if sgids[img], err = fz.Upload(path); err != nil {
			return report, err
		}
	}
	sending = true
	if moveBoard {
		if err := fz.MoveCardToBoard(d.Card, board.ID); err != nil {
			return report, err
		}
		add("moved to board " + line(board.Name))
	}
	if sameText {
		add("title and description unchanged")
	} else {
		title, desc := d.Title, d.DescriptionHTML(sgids)
		if err := fz.UpdateCard(d.Card, fizzy.CardFields{Title: &title, Description: &desc}); err != nil {
			return report, err
		}
		add("title and description sent")
	}

	plan := draft.PlanSteps(d.Steps, card.Steps)
	for _, s := range plan.Add {
		if _, err := fz.AddStep(d.Card, s.Content, s.Done); err != nil {
			return report, err
		}
		add("step added: " + s.Content)
	}
	ticked := true
	for _, id := range plan.Tick {
		if err := fz.UpdateStep(d.Card, id, nil, &ticked); err != nil {
			return report, err
		}
		add("step ticked: " + line(id))
	}
	for _, s := range plan.Extra {
		if prune && !s.Completed {
			if err := fz.DeleteStep(d.Card, s.ID); err != nil {
				return report, err
			}
			add("step deleted: " + line(s.Content))
		} else {
			add("step kept, not in draft: " + line(s.Content))
		}
	}

	for _, t := range d.Tags {
		changed, err := fz.AddTag(d.Card, t)
		if err != nil {
			return report, err
		}
		if changed {
			add("tag added: " + t)
		}
	}
	for _, t := range card.Tags {
		if hasTag(d.Tags, t) {
			continue
		}
		if !prune {
			add("tag kept, not in draft: " + line(t))
			continue
		}
		changed, err := fz.RemoveTag(d.Card, t)
		if err != nil {
			return report, err
		}
		if changed {
			add("tag removed: " + line(t))
		}
	}

	switch {
	case card.Closed:
		add("card is closed; column left alone")
	case moveColumn:
		if err := fz.MoveCard(d.Card, d.Column, board.ID); err != nil {
			return report, err
		}
		add("moved to " + d.Column)
	}
	if card.Golden != d.Golden {
		if err := fz.SetGolden(d.Card, d.Golden); err != nil {
			return report, err
		}
		if d.Golden {
			add("golden on")
		} else {
			add("golden off")
		}
	}
	return report, nil
}

// hasTag reports whether tags holds t, compared with fizzy.TagTitle as
// AddTag and RemoveTag compare them, so a card tag written "#Buy" matches
// the draft tag "buy".
func hasTag(tags []string, t string) bool {
	for _, x := range tags {
		if fizzy.TagTitle(x) == fizzy.TagTitle(t) {
			return true
		}
	}
	return false
}
