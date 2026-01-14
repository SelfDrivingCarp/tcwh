package controller

import (
	"context"
	"fmt"

	"github.com/selfdrivingcarp/tcwh"
)

type webhook struct {
	Sub          string  `db:"sub"`
	Label        string  `db:"label"`
	TemplateType string  `db:"template_type"`
	Template     *string `db:"template"`
	URL          string  `db:"url"`
	Enabled      bool    `db:"enabled"`
}

func (dbwh webhook) Promote() *tcwh.Webhook {
	wh := &tcwh.Webhook{
		Sub:          dbwh.Sub,
		Label:        dbwh.Label,
		TemplateType: dbwh.TemplateType,
		URL:          dbwh.URL,
		Enabled:      dbwh.Enabled,
	}
	if dbwh.Template != nil {
		wh.Template = *dbwh.Template
	}
	return wh
}

func dbWebhook(wh *tcwh.Webhook) *webhook {
	dbwh := &webhook{
		Sub:          wh.Sub,
		Label:        wh.Label,
		TemplateType: wh.TemplateType,
		URL:          wh.URL,
		Enabled:      wh.Enabled,
	}
	if wh.Template != "" {
		dbwh.Template = &wh.Template
	}
	return dbwh
}

func (ctrl *Controller) AddWebhook(ctx context.Context, wh *tcwh.Webhook) error {
	_, err := ctrl.db.NamedExecContext(ctx, `
INSERT INTO webhooks (
	sub,
	label,
	template_type,
	template,
	url,
	enabled
) VALUES (
	:sub,
	:label,
	:template_type,
	:template,
	:url,
	:enabled
);
`, dbWebhook(wh))
	if err != nil {
		return err
	}
	ctrl.eventWebhookAdded(wh)
	return nil
}

func (ctrl *Controller) ToggleWebhookEnabled(ctx context.Context, sub, label string) error {
	return ctrl.inRWTx(ctx, func(ctx context.Context, tx dbSetter) error {
		wh, err := getWebhook(ctx, tx, sub, label)
		if err != nil {
			return fmt.Errorf("getting webhook: %w", err)
		}
		wh.Enabled = !wh.Enabled

		if _, err := tx.NamedExecContext(ctx, `
UPDATE webhooks
	SET enabled = :enabled
	WHERE sub = :sub AND label = :label
`,
			wh); err != nil {
			return fmt.Errorf("updating webhook: %w", err)
		}

		ctrl.eventWebhookUpdated(wh)
		return nil
	})
}

func getWebhook(ctx context.Context, db dbGetter, sub, label string) (*tcwh.Webhook, error) {
	return getT[webhook](ctx, db, `
SELECT
	sub,
	label,
	template_type,
	template,
	url,
	enabled
FROM webhooks
	WHERE sub = ? AND label = ?
`, sub, label)
}

func getWebhooks(ctx context.Context, db dbGetter, sub string) ([]*tcwh.Webhook, error) {
	return selectT[*webhook](ctx, db, `
SELECT
	sub,
	label,
	template_type,
	template,
	url,
	enabled
FROM webhooks
	WHERE sub = ? 
	ORDER BY label
`, sub)
}

func getWebhooksActive(ctx context.Context, db dbGetter) ([]*tcwh.Webhook, error) {
	return selectT[*webhook](ctx, db, `
SELECT
	sub,
	label,
	template_type,
	template,
	url,
	enabled	
FROM webhooks
	WHERE
		enabled = 1
`)
}

func (ctrl *Controller) DeleteWebhook(ctx context.Context, sub, label string) error {
	return ctrl.inRWTx(ctx, func(ctx context.Context, tx dbSetter) error {
		ctrl.log.Debug("getting webhook to delete", "sub", sub, "label", label)
		wh, err := getWebhook(ctx, tx, sub, label)
		if err != nil {
			return fmt.Errorf("retrieving webhook: %w", err)
		}
		ctrl.log.Debug("got webhook to delete", "sub", sub, "label", label, "url", wh.URL)
		_, err = tx.ExecContext(ctx, `
DELETE FROM webhooks
	WHERE sub = ? AND label = ?
`, sub, label)
		if err != nil {
			return err
		}
		ctrl.log.Debug("deleted webhook", "sub", sub, "label", label, "url", wh.URL)
		ctrl.eventWebhookUpdated(wh)
		return nil
	})
}
