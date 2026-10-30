package fizzy

import (
	"fmt"
	"testing"
)

// Checks that stop a change before it is sent are refusals, so the CLI
// exits 2 on them.
func TestValidationErrorsAreRefusals(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, cardServer(`{"assignees":[{"id":"u1"}],"has_more_assignees":true}`))
	_, _, assignErr := f.Assign(87, "u2")
	empty := ""
	for name, err := range map[string]error{
		"assign past five":      assignErr,
		"empty boost":           CheckBoost(""),
		"long boost":            CheckBoost("seventeen letters"),
		"postpone days":         ValidPostponeDays(5),
		"board nothing":         f.UpdateBoard("B1", BoardFields{}),
		"board postpone":        f.UpdateBoard("B1", BoardFields{PostponeDays: new(5)}),
		"column nothing":        f.UpdateColumn("B1", "C1", ColumnFields{}),
		"email frequency":       f.SetEmailFrequency("hourly"),
		"webhook scheme":        CheckWebhookURL("ftp://fizzy.example.com/hook"),
		"webhook credentials":   CheckWebhookURL("https://a:b@fizzy.example.com/hook"),
		"webhook action":        CheckWebhookActions([]string{"gate_swung"}),
		"webhook no actions":    second(f.CreateWebhook("B1", "Gate", "https://fizzy.example.com/hook", nil)),
		"webhook update empty":  second(f.UpdateWebhook("B1", "W1", nil, nil)),
		"webhook update action": second(f.UpdateWebhook("B1", "W1", &empty, []string{})),
	} {
		if !IsRefusal(err) {
			t.Errorf("%s: %v (%T) is not a refusal", name, err, err)
		}
	}
	for _, c := range s.calls {
		if c.Method != "GET" {
			t.Errorf("sent %s %s", c.Method, c.Path)
		}
	}
	if IsRefusal(fmt.Errorf("plain")) {
		t.Error("a plain error is a refusal")
	}
}

func second[T any](_ T, err error) error { return err }
