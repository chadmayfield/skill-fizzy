package fizzy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Samples with the live key names and value types, filled with invented
// gate and hinge values.
const (
	shapeAccount  = `{"id":"03abc0000000000000000000f1","name":"Gate Works","cards_count":5,"created_at":"2026-01-01T00:00:00.000Z","auto_postpone_period_in_days":30}`
	shapeIdentity = `{"id":"03abc0000000000000000000g1","accounts":[` +
		`{"id":"03abc0000000000000000000f2","name":"Other","slug":"/2","created_at":"2026-01-01T00:00:00.000Z","user":{"id":"U2","name":"Alex","role":"member","active":true,"email_address":"alex@example.com","created_at":"2026-01-01T00:00:00.000Z","url":"u","avatar_url":"a"}},` +
		`{"id":"03abc0000000000000000000f1","name":"Gate Works","slug":"/1","created_at":"2026-01-01T00:00:00.000Z","user":` + shapeUser + `}]}`
	// Notifications came back empty live, so this follows the API docs.
	shapeNotification = `{"id":"N1","read":false,"read_at":null,"created_at":"2026-01-02T00:00:00.000Z","title":"Oil the hinge","body":"Assigned to you","creator":` + shapeUser + `,"card":{"id":"03abc0000000000000000000d1","title":"Oil the hinge","status":"published","url":"https://fizzy.example.com/1/cards/87"},"url":"https://fizzy.example.com/1/notifications/N1"}`
	shapeActivity     = `{"id":"A1","action":"comment_created","created_at":"2026-01-03T00:00:00.000Z","description":"Alex commented","particulars":{},"url":"u","eventable_type":"Comment","eventable":{"id":"C1","created_at":"x","updated_at":"x","body":{"plain_text":"p","html":"h"},"creator":` + shapeUser + `,"card":{"id":"K1","url":"u"},"reactions_url":"u","url":"u"},"board":` + shapeBoard + `,"creator":` + shapeUser + `}`
	shapeCardEvent    = `{"id":"A2","action":"card_title_changed","created_at":"2026-01-03T00:00:00.000Z","description":"d","particulars":{"old_title":"Gate","new_title":"Hinge"},"url":"u","eventable_type":"Card","eventable":{"id":"K1","number":87,"title":"Hinge"},"board":` + shapeBoard + `,"creator":` + shapeUser + `}`
	shapeExport       = `{"id":"E1","status":"completed","created_at":"2026-01-01T00:00:00Z","download_url":"https://fizzy.example.com/e.zip"}`
)

func TestDecodeAccountShapes(t *testing.T) {
	var a Account
	if err := decodeKeep(shapeAccount, &a); err != nil || a.Name != "Gate Works" || a.CardsCount != 5 ||
		a.AutoPostponePeriodInDays == nil || *a.AutoPostponePeriodInDays != 30 || string(a.Raw()) != shapeAccount {
		t.Errorf("account = %+v err %v", a, err)
	}
	var id Identity
	if err := decodeKeep(shapeIdentity, &id); err != nil || len(id.Accounts) != 2 || id.Accounts[1].User.Name != "Alex" ||
		id.Accounts[1].Slug != "/1" || string(id.Raw()) != shapeIdentity {
		t.Errorf("identity = %+v err %v", id, err)
	}
	var n Notification
	if err := decodeKeep(shapeNotification, &n); err != nil || n.Read || n.Title != "Oil the hinge" ||
		n.Card == nil || n.Card.Title != "Oil the hinge" || n.Creator == nil || n.Creator.Name != "Alex" {
		t.Errorf("notification = %+v err %v", n, err)
	}
	var act Activity
	if err := decodeKeep(shapeActivity, &act); err != nil || act.EventableType != "Comment" || act.Board == nil ||
		act.Board.Name != "Gate" || act.Creator == nil || act.Eventable.Number != 0 || len(act.Particulars) != 0 {
		t.Errorf("activity = %+v err %v", act, err)
	}
	var ce Activity
	if err := decodeKeep(shapeCardEvent, &ce); err != nil || ce.Eventable.Number != 87 || ce.Particulars["new_title"] != "Hinge" {
		t.Errorf("card activity = %+v err %v", ce, err)
	}
	var e Export
	if err := decodeKeep(shapeExport, &e); err != nil || e.Status != "completed" || e.DownloadURL == "" || string(e.Raw()) != shapeExport {
		t.Errorf("export = %+v err %v", e, err)
	}
}

func jsonText(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, s)
}

func TestAccountSettingsAndPostpone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) { jsonText(w, shapeAccount) })
	a, err := f.AccountSettings()
	if err != nil || a.ID != "03abc0000000000000000000f1" {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	if _, err := f.PostponeAccount(90); err != nil {
		t.Fatal(err)
	}
	if s.calls[0].Method != "GET" || s.calls[0].Path != "/1/account/settings" {
		t.Errorf("first call %+v", s.calls[0])
	}
	c := s.calls[1]
	if c.Method != "PUT" || c.Path != "/1/account/entropy" || string(c.Body) != `{"entropy":{"auto_postpone_period_in_days":90}}` {
		t.Errorf("second call %+v body %s", c, c.Body)
	}
	if _, err := f.PostponeAccount(0); err == nil {
		t.Error("PostponeAccount(0) did not fail")
	}
}

func TestIdentityIsNotAccountScopedAndMeMatchesAccount(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) { jsonText(w, shapeIdentity) })
	me, err := f.Me()
	if err != nil || me.ID != "03abc0000000000000000000a1" {
		t.Fatalf("me=%+v err=%v", me, err)
	}
	if s.calls[0].Path != "/my/identity" {
		t.Errorf("path = %s", s.calls[0].Path)
	}
	if f.C.Account() != "1" {
		t.Errorf("Account() = %q", f.C.Account())
	}
}

func TestMeMissingAccount(t *testing.T) {
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		jsonText(w, `{"accounts":[{"id":"x","slug":"/9","user":{"id":"U9"}}]}`)
	})
	if _, err := f.Me(); err == nil || !strings.Contains(err.Error(), "account 1") {
		t.Errorf("err = %v", err)
	}
}

func TestSetTimezone(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) { w.WriteHeader(204) })
	if err := f.SetTimezone("America/Denver"); err != nil {
		t.Fatal(err)
	}
	c := s.calls[0]
	if c.Method != "PATCH" || c.Path != "/1/my/timezone" || string(c.Body) != `{"timezone_name":"America/Denver"}` {
		t.Errorf("call %+v body %s", c, c.Body)
	}
}

func TestNotifications(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/1/notifications":
			jsonText(w, "["+shapeNotification+"]")
		case r.Method == "GET":
			jsonText(w, `{"bundle_email_frequency":"daily"}`)
		default:
			w.WriteHeader(204)
		}
	})
	ns, err := f.Notifications()
	if err != nil || len(ns) != 1 || ns[0].ID != "N1" {
		t.Fatalf("ns=%+v err=%v", ns, err)
	}
	if err := f.ReadNotification("N1"); err != nil {
		t.Fatal(err)
	}
	if err := f.UnreadNotification("N1"); err != nil {
		t.Fatal(err)
	}
	if err := f.ReadAllNotifications(); err != nil {
		t.Fatal(err)
	}
	st, err := f.NotificationSettings()
	if err != nil || st.BundleEmailFrequency != "daily" {
		t.Fatalf("st=%+v err=%v", st, err)
	}
	if err := f.SetEmailFrequency("weekly"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetEmailFrequency("hourly"); err == nil {
		t.Error("hourly was accepted")
	}
	want := []string{
		"GET /1/notifications ",
		"POST /1/notifications/N1/reading ",
		"DELETE /1/notifications/N1/reading ",
		"POST /1/notifications/bulk_reading ",
		"GET /1/notifications/settings ",
		`PUT /1/notifications/settings {"user_settings":{"bundle_email_frequency":"weekly"}}`,
	}
	if len(s.calls) != len(want) {
		t.Fatalf("calls = %+v", s.calls)
	}
	for i, c := range s.calls {
		if got := c.Method + " " + c.Path + " " + string(c.Body); got != want[i] {
			t.Errorf("call %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestActivitiesFilters(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		jsonText(w, "["+shapeActivity+","+shapeCardEvent+"]")
	})
	acts, err := f.Activities("B1", "U1")
	if err != nil || len(acts) != 2 {
		t.Fatalf("acts=%+v err=%v", acts, err)
	}
	if s.calls[0].Path != "/1/activities" || s.calls[0].Query != "board_ids%5B%5D=B1&creator_ids%5B%5D=U1" {
		t.Errorf("call %+v", s.calls[0])
	}
	if _, err := f.Activities("", ""); err != nil {
		t.Fatal(err)
	}
	if s.calls[1].Query != "" {
		t.Errorf("query = %q", s.calls[1].Query)
	}
}

func TestExports(t *testing.T) {
	f, s, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path == "/my/identity" {
			jsonText(w, shapeIdentity)
			return
		}
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			io.WriteString(w, `{"id":"E1","status":"pending","created_at":"2026-01-01T00:00:00Z"}`)
			return
		}
		jsonText(w, shapeExport)
	})
	e, err := f.StartAccountExport()
	if err != nil || e.ID != "E1" || e.Status != "pending" {
		t.Fatalf("e=%+v err=%v", e, err)
	}
	if e, err = f.StartMyExport(); err != nil || e.ID != "E1" {
		t.Fatalf("e=%+v err=%v", e, err)
	}
	if e, err = f.AccountExport("E1"); err != nil || e.Status != "completed" {
		t.Fatalf("e=%+v err=%v", e, err)
	}
	if e, err = f.MyExport("E1"); err != nil || e.DownloadURL == "" {
		t.Fatalf("e=%+v err=%v", e, err)
	}
	want := []string{
		"POST /1/account/exports",
		"GET /my/identity",
		"POST /1/users/03abc0000000000000000000a1/data_exports",
		"GET /1/account/exports/E1",
		"GET /my/identity",
		"GET /1/users/03abc0000000000000000000a1/data_exports/E1",
	}
	if len(s.calls) != len(want) {
		t.Fatalf("calls = %+v", s.calls)
	}
	for i, c := range s.calls {
		if got := c.Method + " " + c.Path; got != want[i] {
			t.Errorf("call %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestStartAccountExportDryRun(t *testing.T) {
	f, s, out, srv := newFizzy(t, true, func(w http.ResponseWriter, _ *http.Request, _ []byte) {})
	e, err := f.StartAccountExport()
	if err != nil || e.ID != "" || len(s.calls) != 0 {
		t.Fatalf("e=%+v err=%v calls=%v", e, err, s.calls)
	}
	if out.String() != "[dry-run] POST "+srv.URL+"/1/account/exports \n" {
		t.Errorf("out = %q", out.String())
	}
}

func decodeKeep(s string, v any) error { return json.Unmarshal([]byte(s), v) }
