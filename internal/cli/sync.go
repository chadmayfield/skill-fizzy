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
// synced-fields: line, the refusal names the fields that changed.
func checkSynced(d *draft.Draft, card fizzy.Card) error {
	name := filepath.Base(d.Path)
	switch {
	case d.Synced == "":
		return fmt.Errorf("%s: no synced: line, so changes made in Fizzy cannot be detected; rerun with --force", name)
	case isTimestamp(d.Synced):
		return fmt.Errorf("%s: synced: is a time written by an older fz, so changes made in Fizzy cannot be detected; rerun with --force", name)
	case !isFingerprint(d.Synced):
		return fmt.Errorf("%s: synced: is not a card fingerprint, so changes made in Fizzy cannot be detected; rerun with --force", name)
	case cardFingerprint(card) != d.Synced:
		what := ""
		if fields := changedFields(d.Meta["synced-fields"], card); len(fields) > 0 {
			what = " (" + strings.Join(fields, ", ") + ")"
		}
		return fmt.Errorf("%s: card #%d changed in Fizzy since the last sync%s; check it, then rerun with --force", name, d.Card, what)
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

// syncDraft pushes a draft onto its existing card d.Card and returns one
// report line per change. Unless force is set, it refuses a draft with no
// usable synced: line, or one whose card changed in Fizzy since synced:,
// before sending any write, and uploads images before any card write. Steps are never unticked. With prune, unticked card
// steps and card tags the draft lacks are removed; ticked steps always stay.
func syncDraft(fz *fizzy.Fizzy, d *draft.Draft, inbox string, prune, force bool) ([]string, error) {
	card, err := fz.Card(d.Card)
	if err != nil {
		return nil, err
	}
	if !force {
		if err := checkSynced(d, card); err != nil {
			return nil, err
		}
	}
	var report []string
	board, err := fz.Board(d.Board)
	if err != nil {
		return nil, err
	}
	// Upload first, so a failed upload leaves the card untouched.
	sgids := map[string]string{}
	for _, img := range d.Images() {
		path, err := d.ResolveImage(img, inbox)
		if err != nil {
			return nil, err
		}
		if sgids[img], err = fz.Upload(path); err != nil {
			return nil, err
		}
	}
	if card.Board.ID != board.ID {
		if err := fz.MoveCardToBoard(d.Card, board.ID); err != nil {
			return nil, err
		}
		report = append(report, "moved to board "+board.Name)
	}
	title, desc := d.Title, d.DescriptionHTML(sgids)
	if err := fz.UpdateCard(d.Card, fizzy.CardFields{Title: &title, Description: &desc}); err != nil {
		return nil, err
	}
	report = append(report, "title and description sent")

	plan := draft.PlanSteps(d.Steps, card.Steps)
	for _, s := range plan.Add {
		if _, err := fz.AddStep(d.Card, s.Content, s.Done); err != nil {
			return nil, err
		}
		report = append(report, "step added: "+s.Content)
	}
	ticked := true
	for _, id := range plan.Tick {
		if err := fz.UpdateStep(d.Card, id, nil, &ticked); err != nil {
			return nil, err
		}
		report = append(report, "step ticked: "+id)
	}
	for _, s := range plan.Extra {
		if prune && !s.Completed {
			if err := fz.DeleteStep(d.Card, s.ID); err != nil {
				return nil, err
			}
			report = append(report, "step deleted: "+s.Content)
		} else {
			report = append(report, "step kept, not in draft: "+s.Content)
		}
	}

	for _, t := range d.Tags {
		changed, err := fz.AddTag(d.Card, t)
		if err != nil {
			return nil, err
		}
		if changed {
			report = append(report, "tag added: "+t)
		}
	}
	for _, t := range card.Tags {
		if hasTag(d.Tags, tagKey(t)) {
			continue
		}
		if !prune {
			report = append(report, "tag kept, not in draft: "+t)
			continue
		}
		changed, err := fz.RemoveTag(d.Card, t)
		if err != nil {
			return nil, err
		}
		if changed {
			report = append(report, "tag removed: "+t)
		}
	}

	switch {
	case card.Closed:
		report = append(report, "card is closed; column left alone")
	case cardLane(card) != lane(d.Column):
		if err := fz.MoveCard(d.Card, d.Column, board.ID); err != nil {
			return nil, err
		}
		report = append(report, "moved to "+d.Column)
	}
	if card.Golden != d.Golden {
		if err := fz.SetGolden(d.Card, d.Golden); err != nil {
			return nil, err
		}
		if d.Golden {
			report = append(report, "golden on")
		} else {
			report = append(report, "golden off")
		}
	}
	return report, nil
}

// tagKey normalizes a tag the way fizzy's AddTag and RemoveTag do, so a card
// tag written "#Buy" matches the draft tag "buy".
func tagKey(t string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimLeft(t, "#")))
}

func hasTag(tags []string, t string) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}
