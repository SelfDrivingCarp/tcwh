package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/selfdrivingcarp/tcwh"
)

func (s *Server) handleWebhookAdd(w http.ResponseWriter, r *http.Request) {
	sub := getSub(r.Context())

	label := r.PostFormValue("label")
	url := r.PostFormValue("url")
	templateType := r.PostFormValue("template_type")

	if label == "" || url == "" || templateType == "" {
		http.Error(w, "required: label, url, template_type", http.StatusBadRequest)
		return
	}

	err := s.ctrl.AddWebhook(r.Context(), &tcwh.Webhook{
		Sub:          sub,
		Label:        label,
		TemplateType: templateType,
		URL:          url,
		Enabled:      true,
	})
	if err != nil {
		s.log.Error("adding webhook", "sub", sub, "label", label, "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	sub := getSub(r.Context())

	label := r.PathValue("label")
	if label == "" {
		http.Error(w, "required: label", http.StatusBadRequest)
		return
	}

	err := s.ctrl.DeleteWebhook(r.Context(), sub, label)
	if err != nil {
		s.log.Error("deleting webhook", "sub", sub, "label", label, "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleWebhookEdit(w http.ResponseWriter, r *http.Request) {
	sub := getSub(r.Context())

	label := r.PathValue("label")
	if label == "" {
		http.Error(w, "required: label", http.StatusBadRequest)
		return
	}

	err := s.ctrl.ToggleWebhookEnabled(r.Context(), sub, label)
	if err != nil {
		s.log.Error("toggling webhook", "sub", sub, "label", label, "error", err.Error())
		quickResponse(w, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleWebhookCall(w http.ResponseWriter, r *http.Request) {
	sub := r.PathValue("sub")

	var wc tcwh.WebhookCall

	for header, field := range map[string]*string{
		"Twitch-Eventsub-Message-Id":           &wc.MessageID,
		"Twitch-Eventsub-Message-Timestamp":    &wc.Timestamp,
		"Twitch-Eventsub-Message-Retry":        &wc.Retry,
		"Twitch-Eventsub-Message-Type":         &wc.Type,
		"Twitch-Eventsub-Message-Signature":    &wc.Signature,
		"Twitch-Eventsub-Subscription-Type":    &wc.SubscriptionType,
		"Twitch-Eventsub-Subscription-Version": &wc.SubscriptionVersion,
	} {
		*field = r.Header.Get(header)
	}

	b, err := limitedBodyRead(r, 256*1024)
	if err != nil {
		var httpErr httpError
		if errors.As(err, &httpErr) {
			quickResponse(w, httpErr.statusCode)
		} else {
			quickResponse(w, http.StatusBadRequest)
		}
		return
	}
	wc.Body = b

	if wc.Type == "webhook_callback_verification" {
		var v struct {
			Challenge    string `json:"challenge"`
			Subscription struct {
				ID string `json:"id"`
			} `json:"subscription"`
		}
		if err := json.Unmarshal(wc.Body, &v); err != nil {
			s.log.Warn("unmarshalling webhook challenge", "error", err.Error())
			quickResponse(w, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(v.Challenge)))
		w.Write([]byte(v.Challenge))
		s.log.Debug("webhook callback verification", "subcription_id", v.Subscription.ID)
		return
	}

	if err := s.ctrl.HandleWebhookCall(r.Context(), sub, &wc); err != nil {
		s.log.Error("handling webhook call",
			"sub", sub,
			"path", r.URL.Path,
			"body_length", len(b),
			"error", err.Error(),
		)
		switch {
		case errors.Is(err, tcwh.ErrBadValue):
			quickResponse(w, http.StatusBadRequest)
		case errors.Is(err, tcwh.ErrNotFound):
			quickResponse(w, http.StatusNotFound)
		case errors.Is(err, tcwh.ErrForbidden):
			quickResponse(w, http.StatusForbidden)
		default:
			quickResponse(w, http.StatusInternalServerError)
		}
		return
	}

	quickResponse(w, http.StatusAccepted)
}
