package tcwh

import (
	"errors"
	"time"
)

var (
	ErrBadValue  = errors.New("bad value")
	ErrForbidden = errors.New("forbidden")
	ErrNotFound  = errors.New("not found")
)

func Default[T comparable](v, defaultV T) T {
	var zero T
	if v == zero {
		return defaultV
	}
	return v
}

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
