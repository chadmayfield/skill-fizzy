package cli

import (
	"strings"
	"testing"
)

// A card in Not now comes back with postponed true and no column. It prints
// as [not now], not as [maybe]. A closed card still prints as [done].
func TestCardLineNotNow(t *testing.T) {
	notNow := `{"number":88,"title":"Paint the gate","postponed":true,"column":null,"closed":false,"board":{"id":"b1","name":"Home"}}`
	closed := `{"number":89,"title":"Oil the hinge","postponed":true,"column":null,"closed":true,"board":{"id":"b1","name":"Home"}}`
	maybe := `{"number":90,"title":"Fit the latch","postponed":false,"column":null,"closed":false,"board":{"id":"b1","name":"Home"}}`
	acctServer(t, map[string]string{
		"GET /1/cards":    "[" + notNow + "," + closed + "," + maybe + "]",
		"GET /1/cards/88": notNow,
	})
	r := runGo([]string{"card", "list"})
	want := "#88    [not now] Paint the gate\n#89    [done] Oil the hinge\n#90    [maybe] Fit the latch\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("card list: %+v, want stdout %q", r, want)
	}
	r = runGo([]string{"card", "show", "88"})
	if r.code != 0 || !strings.HasPrefix(r.stdout, "#88    [not now] Paint the gate\n") {
		t.Errorf("card show: %+v", r)
	}
}
