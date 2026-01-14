package controller

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/selfdrivingcarp/tcwh"
)

type webhookSend struct {
	body io.Reader
	wh   *tcwh.Webhook
}

type webhookSender struct {
	log      *slog.Logger
	url      string
	in       chan *webhookSend
	lastSend time.Time
}

func newSender(log *slog.Logger, url string) *webhookSender {
	sender := &webhookSender{
		log:      log.With("sender_url", url),
		in:       make(chan *webhookSend, 64),
		url:      url,
		lastSend: time.Now(),
	}
	return sender
}

func (sender *webhookSender) Run() {
	defer sender.log.Debug("sender exiting")
	for whs := range sender.in {
		sender.sendWebhookEvent(whs)
	}
}

func (sender *webhookSender) Send(whs *webhookSend) {
	select {
	case sender.in <- whs:
		// cool
	default:
		sender.log.Warn("dropped event hook message")
	}
}

func (sender *webhookSender) Close() {
	sender.log.Debug("closing")
	close(sender.in)
}

func (sender *webhookSender) sendWebhookEvent(whs *webhookSend) {
	sender.lastSend = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, whs.wh.URL, whs.body)
	if err != nil {
		sender.log.Error("creating request", "sub", whs.wh.Sub, "error", err.Error())
		return
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		sender.log.Error("sending request")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		sender.log.Error("non-200 status")
		return
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
