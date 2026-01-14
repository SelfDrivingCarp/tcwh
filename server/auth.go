package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/selfdrivingcarp/tcwh"
)

const (
	tokenLifetime  = time.Hour * 24 * 365 * 10
	cookieLifetime = time.Hour * 24 * 365
	cookieNameAuth = "auth"
)

type jwtAuth struct {
	cfg *tcwh.AuthConfig
	key []byte
}

func processKey(input string) []byte {
	key := sha256.Sum256([]byte(input))
	return key[:]
}

func newJWTAuth(cfg *tcwh.AuthConfig) *jwtAuth {
	return &jwtAuth{
		cfg: cfg,
		key: processKey(cfg.Secret),
	}
}

func (ja *jwtAuth) keyFunc(_ *jwt.Token) (any, error) {
	return ja.key, nil
}

func (ja *jwtAuth) mintToken(userID, keyInput string) (string, string, error) {
	gotKey := processKey(keyInput)
	if !bytes.Equal(gotKey, ja.key) {
		return "", "", fmt.Errorf("%w: invalid key", tcwh.ErrForbidden)
	}
	now := time.Now()
	issuedAt := jwt.NewNumericDate(now)
	expires := now.Add(tokenLifetime)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Issuer:    ja.cfg.Issuer,
		Subject:   userID,
		Audience:  []string{ja.cfg.Audience},
		IssuedAt:  issuedAt,
		ExpiresAt: jwt.NewNumericDate(expires),
	})
	tokenStr, err := token.SignedString(ja.key)
	if err != nil {
		return "", "", fmt.Errorf("signing token: %w", err)
	}

	b, err := json.Marshal(issuedAt)
	if err != nil {
		return "", "", fmt.Errorf("marshalling iat: %w", err)
	}

	return tokenStr, string(b), nil
}

func (ja *jwtAuth) parse(tokenString string) (string, string, error) {
	t, err := jwt.Parse(tokenString, ja.keyFunc,
		jwt.WithAudience(ja.cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(ja.cfg.Issuer),
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		return "", "", err
	}
	sub, err := t.Claims.GetSubject()
	if err != nil {
		return "", "", fmt.Errorf("getting subject: %w", err)
	}
	if sub == "" {
		return "", "", errors.New("no subject")
	}
	iat, err := t.Claims.GetIssuedAt()
	if err != nil {
		return "", "", fmt.Errorf("getting iat: %w", err)
	}
	b, err := json.Marshal(iat)
	if err != nil {
		return "", "", fmt.Errorf("marshalling iat: %w", err)
	}
	return sub, string(b), nil
}

type userCtxKey struct{}

func (s *Server) authWrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		tokenStr := r.PostFormValue(cookieNameAuth)
		if tokenStr != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     cookieNameAuth,
				Value:    tokenStr,
				Expires:  now.Add(cookieLifetime),
				Secure:   true,
				HttpOnly: true,
			})
		} else {
			// try to get the auth token from cookies
			for _, cookie := range r.CookiesNamed(cookieNameAuth) {
				if cookie != nil && cookie.Valid() == nil {
					tokenStr = cookie.Value
					break
				}
			}
		}
		if tokenStr == "" {
			http.ServeFile(w, r, filepath.Join(s.cfg.HTMLPath, "login.html"))
			return
		}

		sub, iat, err := s.auth.parse(tokenStr)
		if err != nil {
			s.log.Debug("bad token", "sub", sub, "iat", iat, "error", err.Error())
			http.ServeFile(w, r, filepath.Join(s.cfg.HTMLPath, "login.html"))
			return
		}
		isRevoked, err := s.ctrl.TokenIsRevoked(r.Context(), sub, iat)
		if err != nil {
			s.log.Error("checking token revocation", "sub", sub, "iat", iat, "error", err.Error())
			quickResponse(w, http.StatusInternalServerError)
			return
		}
		if isRevoked {
			s.log.Warn("use of revoked token", "sub", sub, "iat", iat)
			quickResponse(w, http.StatusForbidden)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userCtxKey{}, sub))
		h.ServeHTTP(w, r)
	}
}

func getSub(ctx context.Context) string {
	user, _ := ctx.Value(userCtxKey{}).(string)
	return user
}

func (s *Server) handleIssue(w http.ResponseWriter, r *http.Request) {
	sub := r.FormValue("subject")
	if sub == "" {
		http.Error(w, "required: subject", http.StatusBadRequest)
		return
	}
	keyInput := r.Header.Get("Authorization")

	tokenStr, iat, err := s.auth.mintToken(sub, keyInput)
	if errors.Is(err, tcwh.ErrForbidden) {
		s.log.Warn("token issuance denied", "subject", sub, "error", err.Error())
		quickResponse(w, http.StatusForbidden)
		return
	} else if err != nil {
		s.log.Error("issuing token", "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}

	s.log.Info("issued token", "sub", sub, "iat", iat)
	s.ctrl.RecordToken(r.Context(), sub, iat)

	w.Header().Set("Content-Type", "application/jwt")
	fmt.Fprintln(w, tokenStr)
}
