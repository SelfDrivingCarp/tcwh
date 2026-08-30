package tcwh

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrBadValue  = errors.New("bad value")
	ErrForbidden = errors.New("forbidden")
	ErrNotFound  = errors.New("not found")
)

//go:embed default-template.tmpl
var DefaultTemplate string

//go:embed test-event.json
var TestEvent []byte

type UserState struct {
	User     string     `json:"user"`
	OAuth    *OAuth     `json:"oauth,omitempty"`
	Webhooks []*Webhook `json:"webhooks,omitempty"`
}

type OAuth struct {
	Sub           string    `json:"sub"`
	TwitchID      string    `json:"twitch_id"`
	TwitchLogin   string    `json:"twitch_login"`
	LastValidated time.Time `json:"last_validated"`
	RefreshToken  string    `json:"-"`
	AccessToken   string    `json:"-"`
	ExpiresAt     time.Time `json:"expires_at"`
	Scopes        []string  `json:"scopes"`
}

type Webhook struct {
	Sub          string `json:"sub"`
	Label        string `json:"label"`
	TemplateType string `json:"template_type"`
	Template     string `json:"template,omitempty"`
	URL          string `json:"url"`
	Enabled      bool   `json:"enabled"`
}

func (wh *Webhook) Send(ctx context.Context, msg string) error {
	b, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: msg})
	if err != nil {
		return fmt.Errorf("marshaling webhook body: %w", err)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("non-200 status: %s", resp.Status)
	}
	return nil
}

type WebhookCall struct {
	MessageID           string
	Retry               string
	Type                string
	Signature           string
	Timestamp           string
	SubscriptionType    string
	SubscriptionVersion string
	Body                []byte
}
