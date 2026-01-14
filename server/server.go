package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/selfdrivingcarp/tcwh"
	"github.com/selfdrivingcarp/tcwh/controller"
)

const (
	defaultTwitchAuthURL = "https://id.twitch.tv/oauth2/authorize"
)

type Server struct {
	log  *slog.Logger
	cfg  *tcwh.Config
	auth *jwtAuth
	ctrl *controller.Controller
	http.Handler

	twitchAuthURL url.URL
}

func New(cfg *tcwh.Config, log *slog.Logger, ctrl *controller.Controller) (*Server, error) {
	twitchAuthURL, err := url.Parse(tcwh.Default(cfg.Twitch.TwitchAuthURL, defaultTwitchAuthURL))
	if err != nil {
		return nil, fmt.Errorf("parsing cfg oauth.twitch-auth-url: %w", err)
	}

	log = log.With("comp", "server")
	auth := newJWTAuth(&cfg.Auth)
	mux := http.NewServeMux()
	s := &Server{
		log:  log,
		cfg:  cfg,
		auth: auth,
		ctrl: ctrl,
		Handler: requestLogger{
			Logger:  log,
			handler: mux,
		},
		twitchAuthURL: *twitchAuthURL,
	}
	static := http.FileServer(http.Dir(cfg.HTMLPath))
	mux.HandleFunc("GET /", s.authWrap(static.ServeHTTP))
	mux.HandleFunc("POST /", s.authWrap(static.ServeHTTP))
	mux.HandleFunc("GET /state", s.authWrap(s.handleState))

	mux.HandleFunc("GET /oauth/new", s.authWrap(s.handleOAuthChallenge))
	mux.HandleFunc("GET /oauth/cb", s.authWrap(s.handleOAuthResponse))
	mux.HandleFunc("DELETE /oauth", s.authWrap(s.handleOAuthDelete))

	mux.HandleFunc("DELETE /webhooks/{label}", s.authWrap(s.handleWebhookDelete))
	mux.HandleFunc("POST /webhooks", s.authWrap(s.handleWebhookAdd))
	mux.HandleFunc("PUT /webhooks/{label}", s.authWrap(s.handleWebhookEdit))
	mux.HandleFunc("POST /webhook/cb/{sub}", s.handleWebhookCall)

	mux.HandleFunc("POST /auth/issue", s.handleIssue)
	mux.HandleFunc("GET /auth/whoami", s.authWrap(s.whoAMI))
	mux.HandleFunc("POST /auth/whoami", s.authWrap(s.whoAMI))
	return s, nil
}

func quickResponse(w http.ResponseWriter, statusCode int) {
	http.Error(w, http.StatusText(statusCode), statusCode)
}

type requestLogger struct {
	*slog.Logger
	handler http.Handler
}

func (rl requestLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rl.Logger.Debug(r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
	)
	rl.handler.ServeHTTP(w, r)
}

func (s *Server) whoAMI(w http.ResponseWriter, r *http.Request) {
	user := getSub(r.Context())
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, user)
}

func (s *Server) sendJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.log.Error("marshalling response",
			"type", fmt.Sprintf("%T", v),
			"error", err.Error(),
		)
		quickResponse(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	io.Copy(w, bytes.NewReader(b))
}

type httpError struct {
	statusCode int
}

func (err httpError) Error() string {
	return fmt.Sprintf("%d: %s", err.statusCode, http.StatusText(err.statusCode))
}

func limitedBodyRead(r *http.Request, max int64) ([]byte, error) {
	if r.ContentLength < 0 {
		return nil, httpError{statusCode: http.StatusLengthRequired}
	}
	if r.ContentLength > 0 {
		if r.ContentLength > int64(max) {
			return nil, httpError{statusCode: http.StatusRequestEntityTooLarge}
		}
		max = r.ContentLength
	}
	return io.ReadAll(io.LimitReader(r.Body, max))
}
