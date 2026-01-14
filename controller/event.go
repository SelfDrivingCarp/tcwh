package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/selfdrivingcarp/tcwh"
)

func (ctrl *Controller) eventOAuthDeleted(token *tcwh.OAuth) {
	ctrl.log.Debug("oauth deleted", "sub", token.Sub)

	if err := ctrl.revokeOAuthToken(token); err != nil {
		ctrl.log.Error("revoking token",
			"twitch_id", token.TwitchID, "twitch_login", token.TwitchLogin,
			"error", err.Error(),
		)
	}
	ctrl.updateSubs(context.Background())
}

func (ctrl *Controller) eventOAuthAdded(token *tcwh.OAuth) {
	ctrl.log.Debug("oauth added",
		"sub", token.Sub,
		"twitch_id", token.TwitchID,
		"twitch_login", token.TwitchLogin,
		"scopes", token.Scopes,
	)
	ctrl.updateSubs(context.Background())
}

func (ctrl *Controller) eventWebhookAdded(wh *tcwh.Webhook) {
	ctrl.log.Debug("webhook added",
		"sub", wh.Sub,
		"template_type", wh.TemplateType,
	)
	ctrl.updateSubs(context.Background())
}

func (ctrl *Controller) eventWebhookUpdated(wh *tcwh.Webhook) {
	ctrl.log.Debug("webhook updated",
		"sub", wh.Sub,
		"template_type", wh.TemplateType,
		"enabled", wh.Enabled,
	)
	ctrl.updateSubs(context.Background())
}

func (ctrl *Controller) eventSubscribed(sub string, es *eventSub) {
	ctrl.log.Debug("webhook subscribed",
		"sub", sub,
		"subscription_id", es.id,
	)
}

func (ctrl *Controller) eventUnsubscribed(sub string, es *eventSub) {
	ctrl.log.Debug("webhook unsubscribed",
		"sub", sub,
		"subscription_id", es.id,
	)
}

func (ctrl *Controller) eventWebhookCall(sub string, we webhookEvent) {
	ctrl.log.Debug("webhook call",
		"broadcaster_user_name", we.Event.BroadcasterUserName,
	)

	f := fmt.Sprintf("[%s|%s] %s: %s", we.Event.BroadcasterUserName, time.Now().Format(time.TimeOnly), we.Event.ChatterUserName, we.Event.Message.Text)
	b, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: f})
	if err != nil {
		ctrl.log.Error("marshaling webhook body", "error", err.Error())
		return
	}
	wh, err := getWebhooks(context.Background(), ctrl.db, sub)
	if err != nil {
		ctrl.log.Error("getting webhooks", "sub", sub, "error", err.Error())
		return
	}
	for _, wh := range wh {
		if !wh.Enabled {
			continue
		}
		ctrl.lock.Lock()
		sender, present := ctrl.senders[wh.URL]
		if !present {
			sender = newSender(ctrl.log, wh.URL)
			ctrl.sendersWG.Go(sender.Run)
			ctrl.senders[wh.URL] = sender
		}
		sender.Send(&webhookSend{
			body: bytes.NewReader(b),
			wh:   wh,
		})
		ctrl.lock.Unlock()
	}
}
