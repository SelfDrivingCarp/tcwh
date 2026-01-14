package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	URLAppTokenGrant  = "https://id.twitch.tv/oauth2/token"
	URLAppTokenRevoke = "https://id.twitch.tv/oauth2/revoke"
)

type AppToken struct {
	AccessToken string      `json:"access_token"`
	ExpiresIn   json.Number `json:"expires_in"`
	Type        string      `json:"token_type"`
	ExpiresAt   time.Time   `json:"-"`
}

func (ctrl *Controller) getAppToken() error {
	ctrl.log.Debug("getting twitch token")
	query := url.Values{}
	query.Set("client_id", ctrl.cfg.Twitch.ClientID)
	query.Set("client_secret", ctrl.cfg.Twitch.ClientSecret)
	query.Set("grant_type", "client_credentials")

	resp, err := http.PostForm(URLAppTokenGrant, query)
	if err != nil {
		return fmt.Errorf("requesting OAuth token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("non-200 status %d: %s", resp.StatusCode, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	var token AppToken
	if err := json.Unmarshal(b, &token); err != nil {
		return fmt.Errorf("unmarshalling response: %w", err)
	}

	ctx := context.Background()
	validated, err := ctrl.validateAccessToken(ctx, token.AccessToken)
	if err != nil {
		return fmt.Errorf("validating access token: %w", err)
	}

	expiresIn, err := token.ExpiresIn.Int64()
	if err != nil {
		return fmt.Errorf("parsing expires_in: %w", err)
	}
	token.ExpiresAt = time.Now().Add(time.Second * time.Duration(expiresIn))
	ctrl.token = &token
	ctrl.log.Debug("got twitch app token",
		"user_id", validated.UserID,
		"expires", ctrl.token.ExpiresAt)
	return nil
}

func (ctrl *Controller) revokeAppToken() error {
	query := url.Values{}
	query.Set("client_id", ctrl.cfg.Twitch.ClientID)
	query.Set("token", ctrl.token.AccessToken)

	ctrl.log.Debug("revoking twitch token")
	resp, err := http.PostForm(URLAppTokenRevoke, query)
	if err != nil {
		return fmt.Errorf("revoking OAuth token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("non-200 status %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}
