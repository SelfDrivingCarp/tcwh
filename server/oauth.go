package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
)

var (
	oauthChallenge = rand.Int64() // invalidate token requests not from this process

	oauthScopes = []string{
		"channel:bot",
		"user:bot",
		"user:read:chat",
	}
)

func (s *Server) handleOAuthChallenge(w http.ResponseWriter, r *http.Request) {
	user := getSub(r.Context())
	query := url.Values{}
	query.Set("client_id", s.cfg.Twitch.ClientID)
	query.Set("force_verify", "true")
	query.Set("redirect_uri", s.cfg.Twitch.RedirURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(oauthScopes, " "))
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.BigEndian, rand.Int64())
	state := oauthStateToken(hex.EncodeToString(buf.Bytes()), user)
	query.Set("state", state)

	authURL := s.twitchAuthURL
	authURL.RawQuery = query.Encode()

	s.log.Debug("creating oauth challenge",
		"user", user,
		"twitch_auth_url", s.twitchAuthURL.String(),
		"query", query)

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func (s *Server) handleOAuthResponse(w http.ResponseWriter, r *http.Request) {
	user := getSub(r.Context())
	q := r.URL.Query()
	s.log.Debug("oauth response", "query", q)
	code := q.Get("code")
	if code == "" {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Error doing oauth %s: %s", q.Get("error"), q.Get("error_description"))
		return
	}

	if err := s.ctrl.NewOAuthToken(r.Context(), user, code); err != nil {
		s.log.Error("getting oauth token", "user", user, "code", code, "error", err.Error())
		http.Error(w, "error getting access token, sorry", http.StatusBadGateway)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// generate an oauthStateToken using a given nonce
func oauthStateToken(nonce, uid string) string {
	h := sha256.New()
	prefix := nonce + ":" + uid
	fmt.Fprint(h, prefix)
	binary.Write(h, binary.BigEndian, oauthChallenge)
	hash := h.Sum(nil)
	return fmt.Sprintf("%s:%s", prefix, hex.EncodeToString(hash[0:8]))
}

func (s *Server) handleOAuthDelete(w http.ResponseWriter, r *http.Request) {
	user := getSub(r.Context())
	if err := s.ctrl.DeleteOAuthToken(r.Context(), user); err != nil {
		s.log.Error("deleting oauth token", "sub", user, "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
