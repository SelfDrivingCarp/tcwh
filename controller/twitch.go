package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nicklaw5/helix/v2"

	"github.com/selfdrivingcarp/tcwh"
)

const (
	twitchSubsURL = "https://api.twitch.tv/helix/eventsub/subscriptions"
)

type eventSub struct {
	secret []byte
	id     string
}

type eventSubCondition struct {
	BroadcasterID string `json:"broadcaster_user_id"`
	UserID        string `json:"user_id,omitempty"`
}

type eventSubTransport struct {
	Method   string `json:"method"`
	Callback string `json:"callback"`
	Secret   string `json:"secret"`
}

type eventSubRequest struct {
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Condition eventSubCondition `json:"condition"`
	Transport eventSubTransport `json:"transport"`
}

func (ctrl *Controller) subscribeLocked(ctx context.Context, sub string) error {
	es := ctrl.eventSubs[sub]
	if es != nil {
		return fmt.Errorf("existing sub")
	}

	oauth, err := getOAuth(ctx, ctrl.db, sub)
	if err != nil {
		return fmt.Errorf("getting oauth token: %w", err)
	}

	var b []byte
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("creating secret: %w", err)
	}

	es = &eventSub{
		secret: []byte(hex.EncodeToString(secret)),
	}

	esReq := eventSubRequest{
		Type:    "channel.chat.message",
		Version: "1",
		Condition: eventSubCondition{
			BroadcasterID: oauth.TwitchID,
			UserID:        "1011929489",
		},
		Transport: eventSubTransport{
			Method:   "webhook",
			Callback: ctrl.cfg.Twitch.WebhookCBURL + sub,
			Secret:   string(es.secret),
		},
	}
	b, err = json.Marshal(esReq)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}

	ctrl.log.Debug("requesting subscription",
		"broadcaster_id", esReq.Condition.BroadcasterID,
		"user_id", esReq.Condition.UserID,
		"callback", esReq.Transport.Callback,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, twitchSubsURL, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("creating subscribe request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ctrl.token.AccessToken)
	req.Header.Set("Client-Id", ctrl.cfg.Twitch.ClientID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending subscription request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			ctrl.log.Error("reading subscription response body", "error", err.Error())
		}
		return fmt.Errorf("requesting subscription %s: %s", resp.Status, string(b))
	}
	var v struct {
		Data []struct {
			SubscriptionID string `json:"id"`
		} `json:"data"`
	}
	b, err = io.ReadAll(io.LimitReader(resp.Body, 1024*10))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("unmarshalling response: %w", err)
	}
	if len(v.Data) != 1 {
		return fmt.Errorf("expected 1 data item, got %d", len(v.Data))
	}
	es.id = v.Data[0].SubscriptionID

	ctrl.eventSubs[sub] = es
	ctrl.eventSubscribed(sub, es)
	return nil
}

func (ctrl *Controller) unsubscribeAll() {
	ctx := context.Background()
	ctrl.lock.Lock()
	defer ctrl.lock.Unlock()
	for sub, es := range ctrl.eventSubs {
		if err := ctrl.unsubscribeLocked(ctx, sub, es); err != nil {
			ctrl.log.Error("unsubscribing", "sub", sub, "subscription_id", es.id, "error", err.Error())
		}
		delete(ctrl.eventSubs, sub)
	}
	// any lingering subs
	subIDs, err := ctrl.getEventSubIDs(ctx)
	if err != nil {
		ctrl.log.Debug("getting sub IDs", "error", err.Error())
		return
	}
	for _, subID := range subIDs {
		if err := ctrl.unsubscribeByID(ctx, subID); err != nil {
			ctrl.log.Error("unsubscribing", "id", subID, "error", err.Error())
		}
	}
}

func (ctrl *Controller) getEventSubIDs(ctx context.Context) ([]string, error) {
	ctrl.log.Debug("getting subscription IDs")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, twitchSubsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+ctrl.token.AccessToken)
	req.Header.Set("Client-Id", ctrl.cfg.Twitch.ClientID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("non-200 response: %s", resp.Status)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024*64))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	var v struct {
		Data []struct {
			SubscriptionID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}
	subIDs := make([]string, len(v.Data))
	for i, data := range v.Data {
		subIDs[i] = data.SubscriptionID
	}
	ctrl.log.Debug("got sub IDs", "count", len(subIDs), "ids", subIDs)
	return subIDs, nil
}

func (ctrl *Controller) unsubscribeLocked(ctx context.Context, sub string, es *eventSub) error {
	if err := ctrl.unsubscribeByID(ctx, es.id); err != nil {
		return err
	}
	ctrl.eventUnsubscribed(sub, es)
	return nil
}

func (ctrl *Controller) unsubscribeByID(ctx context.Context, subID string) error {
	u, _ := url.Parse(twitchSubsURL)
	q := url.Values{}
	q.Set("id", subID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Client-Id", ctrl.cfg.Twitch.ClientID)
	req.Header.Set("Authorization", "Bearer "+ctrl.token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("non-2XX status: %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}

type webhookEvent struct {
	Event *helix.EventSubChannelChatMessageEvent
}

func (ctrl *Controller) HandleWebhookCall(ctx context.Context, sub string, whc *tcwh.WebhookCall) error {
	// TODO: drop dupes

	ctrl.lock.Lock()
	es := ctrl.eventSubs[sub]
	ctrl.lock.Unlock()

	if es == nil {
		return tcwh.ErrNotFound
	}

	mac := hmac.New(sha256.New, es.secret)
	mac.Write([]byte(whc.MessageID))
	mac.Write([]byte(whc.Timestamp))
	mac.Write(whc.Body)
	expectedMac := mac.Sum(nil)
	signature, err := hex.DecodeString(strings.TrimPrefix(whc.Signature, "sha256="))
	if err != nil {
		return fmt.Errorf("%w: decoding signature", tcwh.ErrBadValue)
	}
	if !hmac.Equal(signature, expectedMac) {
		return tcwh.ErrForbidden
	}

	var event webhookEvent
	if err := json.Unmarshal(whc.Body, &event); err != nil {
		ctrl.log.Error("unmarshaling webhook event", "sub", sub, "error", err.Error())
		return nil
	}

	ctrl.eventWebhookCall(sub, event)

	return nil
}

func (ctrl *Controller) updateSubs(ctx context.Context) {
	ctrl.lock.Lock()
	defer ctrl.lock.Unlock()
	ctrl.log.Debug("updating subs")
	defer ctrl.log.Debug("updating subs finished")
	var tokens []*tcwh.OAuth
	var activeWebhooks []*tcwh.Webhook
	err := ctrl.inROTx(ctx, func(ctx context.Context, tx dbGetter) error {
		var err error
		if tokens, err = getOAuthTokens(ctx, tx); err != nil {
			return fmt.Errorf("getting oauth tokens: %w", err)
		}
		if activeWebhooks, err = getWebhooksActive(ctx, tx); err != nil {
			return fmt.Errorf("getting active webhooks: %w", err)
		}
		return nil
	})
	if err != nil {
		ctrl.log.Error("getting data for updating subs", "error", err.Error())
		return
	}

	activeWHSubs := map[string]struct{}{}
	for _, wh := range activeWebhooks {
		activeWHSubs[wh.Sub] = struct{}{}
	}

	for _, token := range tokens {
		_, hasWebhooks := activeWHSubs[token.Sub]
		es, hasEventsub := ctrl.eventSubs[token.Sub]
		switch {
		case hasWebhooks && hasEventsub:
			// cool
		case !hasWebhooks && !hasEventsub:
			// cool
		case hasWebhooks && !hasEventsub:
			if err := ctrl.subscribeLocked(ctx, token.Sub); err != nil {
				ctrl.log.Error("subscribing", "sub", token.Sub, "error", err.Error())
			}
		case !hasWebhooks && hasEventsub:
			if err := ctrl.unsubscribeLocked(ctx, token.Sub, es); err != nil {
				ctrl.log.Error("unsubscribing", "sub", token.Sub, "error", err.Error())
			}
		}
	}
}
