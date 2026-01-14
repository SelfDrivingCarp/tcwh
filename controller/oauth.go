package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/selfdrivingcarp/tcwh"
)

const (
	defaultTwitchRevokeURL   = "https://id.twitch.tv/oauth2/revoke"
	defaultTwitchTokenURL    = "https://id.twitch.tv/oauth2/token"
	defaultTwitchValidateURL = "https://id.twitch.tv/oauth2/validate"
)

type oauth struct {
	Sub           string `db:"sub"`
	TwitchID      string `db:"twitch_id"`
	TwitchLogin   string `db:"twitch_login"`
	LastValidated int64  `db:"last_validated"`
	RefreshToken  string `db:"refresh_token"`
	AccessToken   string `db:"access_token"`
	ExpiresAt     int64  `db:"expires_at"`
	Scopes        string `db:"scopes"`
}

func (dbOA oauth) Promote() *tcwh.OAuth {
	return &tcwh.OAuth{
		Sub:           dbOA.Sub,
		TwitchID:      dbOA.TwitchID,
		TwitchLogin:   dbOA.TwitchLogin,
		LastValidated: time.Unix(dbOA.LastValidated, 0),
		RefreshToken:  dbOA.RefreshToken,
		AccessToken:   dbOA.AccessToken,
		ExpiresAt:     time.Unix(dbOA.ExpiresAt, 0),
		Scopes:        strings.Split(dbOA.Scopes, " "),
	}
}

func dbOAuth(oa *tcwh.OAuth) *oauth {
	dbOA := &oauth{
		Sub:           oa.Sub,
		TwitchID:      oa.TwitchID,
		TwitchLogin:   oa.TwitchLogin,
		LastValidated: oa.LastValidated.Unix(),
		RefreshToken:  oa.RefreshToken,
		AccessToken:   oa.AccessToken,
		ExpiresAt:     oa.ExpiresAt.Unix(),
		Scopes:        strings.Join(oa.Scopes, " "),
	}
	return dbOA
}

func getOAuth(ctx context.Context, db dbGetter, sub string) (*tcwh.OAuth, error) {
	return getT[oauth](ctx, db, `
SELECT
	sub,
	twitch_id,
	twitch_login,
	last_validated,
	refresh_token,
	access_token,
	expires_at,
	scopes
FROM oauth_tokens
	WHERE sub = ?`,
		sub)
}

func (ctrl *Controller) deleteOAuthFor(ctx context.Context, db dbSetter, sub string) error {
	token, err := getOAuth(ctx, db, sub)
	if (token == nil && err == nil) || errors.Is(err, tcwh.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting existing token: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE access_token = ?`, token.AccessToken); err != nil {
		return err
	}
	ctrl.eventOAuthDeleted(token)
	return nil
}

func (ctrl *Controller) setOAuth(ctx context.Context, db dbSetter, token *tcwh.OAuth) error {
	_, err := db.NamedExecContext(ctx, `
INSERT INTO oauth_tokens (
	sub,
	twitch_id,
	twitch_login,
	last_validated,
	refresh_token,
	access_token,
	expires_at,
	scopes
) VALUES (
	:sub,
	:twitch_id,
	:twitch_login,
	:last_validated,
	:refresh_token,
	:access_token,
	:expires_at,
	:scopes
);
`, dbOAuth(token))
	if err != nil {
		return err
	}
	ctrl.eventOAuthAdded(token)
	return nil
}

func (ctrl *Controller) NewOAuthToken(ctx context.Context, sub, code string) error {
	query := url.Values{}
	query.Set("client_id", ctrl.cfg.Twitch.ClientID)
	query.Set("client_secret", ctrl.cfg.Twitch.ClientSecret)
	query.Set("code", code)
	query.Set("grant_type", "authorization_code")
	query.Set("redirect_uri", ctrl.cfg.Twitch.RedirURI)

	resp, err := http.PostForm(defaultTwitchTokenURL, query)
	if err != nil {
		return fmt.Errorf("requesting OAuth token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("non-200 status %d: %s", resp.StatusCode, resp.Status)
	}

	var tokenResp struct {
		AccessToken  string   `json:"access_token"`
		ExpiresIn    int      `json:"expires_in"`
		RefreshToken string   `json:"refresh_token"`
		Scopes       []string `json:"scope"`
		TokenType    string   `json:"token_type"`
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if err := json.Unmarshal(b, &tokenResp); err != nil {
		return fmt.Errorf("unmarshalling response: %w", err)
	}

	token := &tcwh.OAuth{
		Sub:          sub,
		RefreshToken: tokenResp.RefreshToken,
		AccessToken:  tokenResp.AccessToken,
		Scopes:       tokenResp.Scopes,
	}
	if err := ctrl.validateToken(ctx, token); err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if err := ctrl.inRWTx(ctx, func(ctx context.Context, tx dbSetter) error {
		if err := ctrl.deleteOAuthFor(ctx, tx, sub); err != nil {
			return fmt.Errorf("deleting existing token: %w", err)
		}
		if err := ctrl.setOAuth(ctx, tx, token); err != nil {
			return fmt.Errorf("storing token: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

type tokenValidationResposne struct {
	Login     string `json:"login"`
	UserID    string `json:"user_id"`
	ExpiresIn int    `json:"expires_in"`
}

func (ctrl *Controller) validateAccessToken(ctx context.Context, accessToken string) (*tokenValidationResposne, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultTwitchValidateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	r.Header.Set("Authorization", "OAuth "+accessToken)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("sending validation request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("non-200 status: %d: %s", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()

	var validationResp tokenValidationResposne
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if err := json.Unmarshal(b, &validationResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}
	ctrl.log.Debug("validated access token",
		"login", validationResp.Login,
		"user_id", validationResp.UserID,
		"expires_in", validationResp.ExpiresIn,
	)
	return &validationResp, nil
}

func (ctrl *Controller) validateToken(ctx context.Context, token *tcwh.OAuth) error {
	ctrl.log.Debug("validating token", "twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin)
	validationResp, err := ctrl.validateAccessToken(ctx, token.AccessToken)
	if err != nil {
		return fmt.Errorf("validating oauth token: %w", err)
	}
	now := time.Now()
	token.TwitchID = validationResp.UserID
	token.TwitchLogin = validationResp.Login
	token.LastValidated = time.Now()
	token.ExpiresAt = now.Add(time.Second * time.Duration(validationResp.ExpiresIn))

	err = ctrl.inRWTx(ctx, func(ctx context.Context, tx dbSetter) error {
		result, err := tx.NamedExecContext(ctx, `
UPDATE oauth_tokens
	SET
		last_validated = :last_validated
	WHERE twitch_id = :twitch_id
`, dbOAuth(token))
		if err != nil {
			return fmt.Errorf("updating %s token for validation: %w", token.TwitchID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("getting rows affected: %w", err)
		}
		if affected > 0 {
			ctrl.log.Debug("updated stored oauth token", "twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin)
		}
		return nil
	})

	return nil
}

func (ctrl *Controller) DeleteOAuthToken(ctx context.Context, sub string) error {
	return ctrl.deleteOAuthFor(ctx, ctrl.db, sub)
}

func (ctrl *Controller) revokeOAuthToken(token *tcwh.OAuth) error {
	ctrl.log.Debug("revoking token", "twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin)

	query := url.Values{}
	query.Set("client_id", ctrl.cfg.Twitch.ClientID)
	query.Set("token", token.AccessToken)

	resp, err := http.PostForm(defaultTwitchRevokeURL, query)
	if err != nil {
		return fmt.Errorf("posting to revoke: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		ctrl.log.Info("revoked token", "twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin)
		return nil
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			ctrl.log.Warn("reading revocation response body",
				"twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin,
				"status_code", resp.StatusCode, "status", resp.Status,
				"error", err.Error(),
			)
			return nil
		}
		var v struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			ctrl.log.Warn("unmarshalling revocation response body",
				"twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin,
				"status_code", resp.StatusCode, "status", resp.Status,
				"error", err.Error(),
			)
			return nil
		}
		ctrl.log.Warn("revocation response body",
			"twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin,
			"status_code", resp.StatusCode, "status", resp.Status,
			"message", v.Message,
		)
		return nil
	}
	return nil
}

func getOAuthTokens(ctx context.Context, db dbGetter) ([]*tcwh.OAuth, error) {
	return selectT[oauth](ctx, db, `
SELECT
	sub,
	twitch_id,
	twitch_login,
	last_validated,
	refresh_token,
	access_token,
	expires_at,
	scopes
FROM oauth_tokens
`)
}
