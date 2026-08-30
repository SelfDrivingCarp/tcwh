package controller

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/nicklaw5/helix/v2"

	"github.com/selfdrivingcarp/tcwh"
)

var testEvent helix.EventSubChannelChatMessageEvent

func init() {
	if err := json.Unmarshal(tcwh.TestEvent, &testEvent); err != nil {
		panic("unmarshaling test event: " + err.Error())
	}
}

type webhookSender struct {
	log      *slog.Logger
	wh       *tcwh.Webhook
	in       chan *helix.EventSubChannelChatMessageEvent
	tmpl     *template.Template
	lastSend time.Time
}

func selectChatterRune(runes, chatterID string) string {
	candidates := []rune(runes)
	if len(runes) == 0 {
		return ""
	}
	id, _ := strconv.Atoi(chatterID)
	return string(candidates[id%len(candidates)])
}

func newSender(log *slog.Logger, wh *tcwh.Webhook) (*webhookSender, error) {
	tmpl, err := template.New("").
		Funcs(template.FuncMap{
			"selectChatterRune": selectChatterRune,
		}).
		Parse(cmp.Or(wh.Template, tcwh.DefaultTemplate))
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	sender := &webhookSender{
		log:      log.With("sender_url", wh.URL),
		in:       make(chan *helix.EventSubChannelChatMessageEvent, 64),
		wh:       wh,
		tmpl:     tmpl,
		lastSend: time.Now(),
	}

	msg, err := sender.format(&testEvent)
	if err != nil {
		return nil, fmt.Errorf("formatting test event: %w", err)
	}
	if msg == "" {
		return nil, fmt.Errorf("no output from test event")
	}

	return sender, nil
}

func (sender *webhookSender) Run() {
	defer sender.log.Debug("sender exiting")
	for whs := range sender.in {
		sender.sendWebhookEvent(whs)
	}
}

func (sender *webhookSender) Send(event *helix.EventSubChannelChatMessageEvent) {
	select {
	case sender.in <- event:
		// cool
	default:
		sender.log.Warn("dropped event hook message",
			"message_id", event.MessageID,
		)
	}
}

func (sender *webhookSender) Close() {
	sender.log.Debug("closing")
	close(sender.in)
}

func (sender *webhookSender) format(event *helix.EventSubChannelChatMessageEvent) (string, error) {
	var buf strings.Builder
	data := struct {
		When  time.Time
		Event *helix.EventSubChannelChatMessageEvent
	}{
		When:  time.Now(),
		Event: event,
	}
	if err := sender.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (sender *webhookSender) sendWebhookEvent(event *helix.EventSubChannelChatMessageEvent) {
	sender.lastSend = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	sender.log.Debug("sending webhook event",
		"message_id", event.MessageID,
		"broadcaster", event.BroadcasterUserName,
		"chatter", event.ChatterUserName,
	)
	msg, err := sender.format(event)
	if err != nil {
		sender.log.Error("formating event message", "message_id", event.MessageID, "error", err.Error())
		return
	}
	if err := sender.wh.Send(ctx, msg); err != nil {
		sender.log.Error("sending webhook event", "message_id", event.MessageID, "error", err.Error())
	}
}

func (ctrl *Controller) manageSenders() {
	for {
		time.Sleep(time.Minute * 5)
		ctrl.lock.Lock()
		if len(ctrl.senders) > 0 {
			ctrl.log.Debug("checking for stale senders")
			now := time.Now()
			for url, sender := range ctrl.senders {
				if now.Sub(sender.lastSend) > time.Minute*5 {
					sender.Close()
					delete(ctrl.senders, url)
				}
			}
		}
		ctrl.lock.Unlock()
	}
}
