package store

import (
	"context"
	"strings"
	"time"

	"github.com/selfdrivingcarp/tcwh"
)

type oauth struct {
	Sub           string    `db:"sub"`
	TwitchID      string    `db:"twitch_id"`
	TwitchLogin   string    `db:"twitch_login"`
	LastValidated time.Time `db:"last_validated"`
	RefreshToken  string    `db:"refresh_token"`
	AccessToken   string    `db:"access_token"`
	ExpiresAt     time.Time `db:"expires_at"`
	Scopes        string    `db:"scopes"`
}

func (dbOA oauth) Promote() *tcwh.OAuth {
	return &tcwh.OAuth{
		Sub:           dbOA.Sub,
		TwitchID:      dbOA.TwitchID,
		LastValidated: dbOA.LastValidated,
		RefreshToken:  dbOA.RefreshToken,
		AccessToken:   dbOA.AccessToken,
		ExpiresAt:     dbOA.ExpiresAt,
		Scopes:        strings.Split(dbOA.Scopes, " "),
	}
}

func dbOAuth(oa *tcwh.OAuth) *oauth {
	dbOA := &oauth{
		Sub:           oa.Sub,
		TwitchID:      oa.TwitchID,
		LastValidated: oa.LastValidated,
		RefreshToken:  oa.RefreshToken,
		AccessToken:   oa.AccessToken,
		ExpiresAt:     oa.ExpiresAt,
		Scopes:        strings.Join(oa.Scopes, " "),
	}
	return dbOA
}

func (s *Store) GetOAuth(ctx context.Context, user string) (*tcwh.OAuth, error) {
	return getT[oauth](ctx, s.DB, `
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
		user)
}

func (s *Store) DeleteOAuth(ctx context.Context, user string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE sub = ?`, user)
	return err
}

func (s *Store) SetOAuth(ctx context.Context, user string, oauth *tcwh.OAuth) error {
	_, err := s.DB.NamedExecContext(ctx, `
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
`, dbOAuth(oauth))
	return err
}
