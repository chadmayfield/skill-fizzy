package fizzy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chadmayfield/skill-fizzy/internal/client"
)

// WebhookActions are the values subscribed_actions accepts, from Fizzy's
// webhooks API documentation.
var WebhookActions = []string{
	"card_assigned", "card_closed", "card_postponed", "card_auto_postponed",
	"card_board_changed", "card_published", "card_reopened",
	"card_sent_back_to_triage", "card_triaged", "card_unassigned", "comment_created",
}

// Webhook is a board webhook. The shape comes from Fizzy's API docs, not
// from a live server, so decode only fields the docs name. SigningSecret is
// decoded but no text output prints it.
type Webhook struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	PayloadURL        string   `json:"payload_url"`
	Active            bool     `json:"active"`
	SigningSecret     string   `json:"signing_secret"`
	SubscribedActions []string `json:"subscribed_actions"`
	CreatedAt         string   `json:"created_at"`
	URL               string   `json:"url"`
	Board             Board    `json:"board"`

	raw json.RawMessage
}

// WebhookDelivery is one attempt to send an event to a webhook. Request and
// Response are nil while the delivery has not started or is in progress.
type WebhookDelivery struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Request   *struct {
		Headers map[string]string `json:"headers"`
	} `json:"request"`
	Response *struct {
		Code  int     `json:"code"`
		Error *string `json:"error"`
	} `json:"response"`
	Event struct {
		ID        string `json:"id"`
		Action    string `json:"action"`
		CreatedAt string `json:"created_at"`
	} `json:"event"`

	raw json.RawMessage
}

// Raw returns the server's JSON for this webhook.
func (w Webhook) Raw() json.RawMessage { return w.raw }

// Raw returns the server's JSON for this delivery.
func (d WebhookDelivery) Raw() json.RawMessage { return d.raw }

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (w *Webhook) UnmarshalJSON(data []byte) error {
	type plain Webhook
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*w = Webhook(p)
	w.raw = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON decodes the known fields and keeps the raw bytes.
func (d *WebhookDelivery) UnmarshalJSON(data []byte) error {
	type plain WebhookDelivery
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*d = WebhookDelivery(p)
	d.raw = append(json.RawMessage(nil), data...)
	return nil
}

// CheckWebhookURL accepts an https URL with a host, or an http URL whose
// host is localhost, 127.0.0.1 or ::1.
func CheckWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return refusef("webhook url must start with https://")
	}
	if u.User != nil {
		return refusef("webhook url must not contain credentials")
	}
	if u.Hostname() == "" {
		return refusef("webhook url has no host")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		switch strings.ToLower(u.Hostname()) {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return refusef("webhook url must start with https:// (http:// is allowed only for localhost)")
}

// CheckWebhookActions rejects an action that is not in WebhookActions.
func CheckWebhookActions(actions []string) error {
	for _, a := range actions {
		ok := false
		for _, k := range WebhookActions {
			ok = ok || k == a
		}
		if !ok {
			return refusef("unknown webhook action %q (choose from %s)", a, strings.Join(WebhookActions, ", "))
		}
	}
	return nil
}

func webhookPath(boardID, id string) (string, error) {
	if err := CheckIDs(boardID, id); err != nil {
		return "", err
	}
	return "/boards/" + url.PathEscape(boardID) + "/webhooks/" + url.PathEscape(id), nil
}

// Webhooks lists a board's webhooks. Only account admins may.
func (f *Fizzy) Webhooks(boardID string) ([]Webhook, error) {
	if err := CheckIDs(boardID); err != nil {
		return nil, err
	}
	return list[Webhook](f.C, "/boards/"+url.PathEscape(boardID)+"/webhooks")
}

// Webhook fetches one webhook.
func (f *Fizzy) Webhook(boardID, id string) (Webhook, error) {
	var w Webhook
	p, err := webhookPath(boardID, id)
	if err != nil {
		return w, err
	}
	err = f.C.Get(p, &w)
	return w, err
}

// WebhookDeliveries lists a webhook's deliveries. Only account admins may.
func (f *Fizzy) WebhookDeliveries(boardID, id string) ([]WebhookDelivery, error) {
	p, err := webhookPath(boardID, id)
	if err != nil {
		return nil, err
	}
	return list[WebhookDelivery](f.C, p+"/deliveries")
}

// CreateWebhook validates the url and actions, then creates the webhook. It
// returns the webhook the server sends back. In dry-run mode the result is
// empty. When the reply has no body, only ID is filled, from Location.
func (f *Fizzy) CreateWebhook(boardID, name, hookURL string, actions []string) (Webhook, error) {
	if err := CheckIDs(boardID); err != nil {
		return Webhook{}, err
	}
	if err := CheckWebhookURL(hookURL); err != nil {
		return Webhook{}, err
	}
	if len(actions) == 0 {
		return Webhook{}, refusef("give at least one action")
	}
	if err := CheckWebhookActions(actions); err != nil {
		return Webhook{}, err
	}
	status, hdr, payload, err := f.C.Do(http.MethodPost, "/boards/"+url.PathEscape(boardID)+"/webhooks", wrap("webhook", struct {
		Name    string   `json:"name"`
		URL     string   `json:"url"`
		Actions []string `json:"subscribed_actions"`
	}{name, hookURL, actions}))
	if err != nil || status == 0 {
		return Webhook{}, err
	}
	var w Webhook
	if json.Valid(payload) && json.Unmarshal(payload, &w) == nil && w.ID != "" {
		return w, nil
	}
	return Webhook{ID: client.IDFromLocation(hdr.Get("Location"))}, nil
}

// UpdateWebhook changes the name and the subscribed actions. Nil name and
// nil actions are not sent. The url cannot change after creation. The result
// is empty in dry-run mode. A reply that is not a webhook with an id is an
// error, because fz cannot show what the server now holds.
func (f *Fizzy) UpdateWebhook(boardID, id string, name *string, actions []string) (Webhook, error) {
	p, err := webhookPath(boardID, id)
	if err != nil {
		return Webhook{}, err
	}
	if name == nil && actions == nil {
		return Webhook{}, refusef("nothing to update")
	}
	if actions != nil {
		if len(actions) == 0 {
			return Webhook{}, refusef("give at least one action")
		}
		if err := CheckWebhookActions(actions); err != nil {
			return Webhook{}, err
		}
	}
	status, _, payload, err := f.C.Do(http.MethodPatch, p, wrap("webhook", struct {
		Name    *string  `json:"name,omitempty"`
		Actions []string `json:"subscribed_actions,omitempty"`
	}{name, actions}))
	if err != nil || status == 0 {
		return Webhook{}, err
	}
	var w Webhook
	if json.Unmarshal(payload, &w) != nil || w.ID == "" {
		return Webhook{}, fmt.Errorf("webhook %s: the server's reply to the update is not a webhook with an id, so fz cannot show the result; check it with webhook show", id)
	}
	return w, nil
}

// DeleteWebhook deletes a webhook.
func (f *Fizzy) DeleteWebhook(boardID, id string) error {
	p, err := webhookPath(boardID, id)
	if err != nil {
		return err
	}
	_, err = f.C.Write(http.MethodDelete, p, nil)
	return err
}

// ActivateWebhook reactivates a webhook and reports whether it sent the
// request. It reads the webhook first and sends nothing when it is active.
func (f *Fizzy) ActivateWebhook(boardID, id string) (bool, error) {
	w, err := f.Webhook(boardID, id)
	if err != nil {
		return false, err
	}
	if w.Active {
		return false, nil
	}
	p, err := webhookPath(boardID, id)
	if err != nil {
		return false, err
	}
	_, err = f.C.Write(http.MethodPost, p+"/activation", nil)
	return err == nil, err
}
