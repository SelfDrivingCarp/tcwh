package controller

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/selfdrivingcarp/tcwh"
)

//go:embed schema1.sql
var schema1 string

var schemas = []string{schema1}

type Controller struct {
	cfg       *tcwh.Config
	log       *slog.Logger
	db        *sqlx.DB
	lock      sync.Mutex
	eventSubs map[string]*eventSub
	token     *AppToken
	senders   map[string]*webhookSender
	sendersWG sync.WaitGroup
}

func New(cfg *tcwh.Config, log *slog.Logger) (*Controller, error) {
	db, err := sqlx.Open("sqlite", cfg.DB.Path+"?_pragma=busy_timeout")
	if err != nil {
		return nil, err
	}
	ctrl := &Controller{
		cfg:       cfg,
		db:        db,
		log:       log.With("comp", "ctrl"),
		eventSubs: map[string]*eventSub{},
		senders:   map[string]*webhookSender{},
	}
	return ctrl, ctrl.init()
}

func (ctrl *Controller) init() error {
	var version int
	if err := ctrl.db.Get(&version, `PRAGMA user_version`); err != nil {
		return fmt.Errorf("getting database version: %w", err)
	}
	ctrl.log.Debug("got PRAGMA", "user_version", version)

	for i, schema := range schemas {
		if i+1 <= version {
			continue
		}
		ctrl.log.Info("applying schema", "version", i+1)
		if _, err := ctrl.db.Exec(schema); err != nil {
			return fmt.Errorf("applying schema %d: %w", i+1, err)
		}
	}
	if err := ctrl.getAppToken(); err != nil {
		return fmt.Errorf("getting app token: %w", err)
	}
	go ctrl.manageSenders()
	return nil
}

func (ctrl *Controller) Close() {
	if err := ctrl.db.Close(); err != nil {
		ctrl.log.Error("closing database", "error", err.Error())
	}
	ctrl.unsubscribeAll()
	if err := ctrl.revokeAppToken(); err != nil {
		ctrl.log.Error("revoking app token", "error", err.Error())
	}
	for _, sender := range ctrl.senders {
		sender.Close()
	}
	ctrl.log.Debug("waiting for senders to finish")
	ctrl.sendersWG.Wait()
}

func (ctrl *Controller) TokenIsRevoked(ctx context.Context, sub, iat string) (bool, error) {
	rows, err := ctrl.db.QueryContext(ctx,
		`SELECT * FROM tokens_issued
	WHERE revoked AND sub = ? AND iat = ?`, sub, iat)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	defer rows.Close()
	return rows.Next(), err
}

func (ctrl *Controller) RecordToken(ctx context.Context, sub, iat string) error {
	_, err := ctrl.db.ExecContext(ctx, "INSERT INTO tokens_issued (sub, iat) VALUES (?,?)", sub, iat)
	return err
}

type promotable[O any] interface {
	Promote() O
}

type dbGetter interface {
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	GetContext(ctx context.Context, dest any, query string, args ...any) error
}

type dbSetter interface {
	dbGetter
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
}

func selectT[I promotable[*O], O any](ctx context.Context, db dbGetter, query string, args ...any) ([]*O, error) {
	var results []I
	err := db.SelectContext(ctx, &results, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tcwh.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	o := make([]*O, len(results))
	for i := range results {
		o[i] = results[i].Promote()
	}
	return o, nil
}

func getT[I promotable[*O], O any](ctx context.Context, db dbGetter, query string, args ...any) (*O, error) {
	var result I
	err := db.GetContext(ctx, &result, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tcwh.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	return result.Promote(), nil
}

func (ctrl *Controller) inRWTx(ctx context.Context, fn func(ctx context.Context, tx dbSetter) error) error {
	tx, err := ctrl.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func (ctrl *Controller) inROTx(ctx context.Context, fn func(ctx context.Context, tx dbGetter) error) error {
	tx, err := ctrl.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Commit()
	return fn(ctx, tx)
}

func (ctrl *Controller) GetState(ctx context.Context, sub string) (*tcwh.UserState, error) {
	state := tcwh.UserState{User: sub}
	err := ctrl.inROTx(ctx, func(ctx context.Context, tx dbGetter) error {
		var err error
		if state.OAuth, err = getOAuth(ctx, tx, sub); err != nil && !errors.Is(err, tcwh.ErrNotFound) {
			return fmt.Errorf("getting oauth: %w", err)
		}
		if state.Webhooks, err = getWebhooks(ctx, tx, sub); err != nil {
			return fmt.Errorf("getting webhooks: %w", err)
		}
		return nil
	})
	return &state, err
}

func (ctrl *Controller) Start(ctx context.Context) {
	// ctrl.log.Warn("updateSubs disabled")
	ctrl.updateSubs(ctx)
}
