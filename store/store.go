// Package store provides storage
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"

	_ "github.com/glebarez/go-sqlite"
	"github.com/jmoiron/sqlx"

	"github.com/selfdrivingcarp/tcwh"
)

//go:embed schema1.sql
var schema1 string

type Store struct {
	*sqlx.DB
	log *slog.Logger
}

func New(cfg *tcwh.DBConfig, log *slog.Logger) (*Store, error) {
	db, err := sqlx.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, err
	}
	s := &Store{
		DB:  db,
		log: log.With("comp", "store"),
	}
	return s, s.init()
}

func (s *Store) init() error {
	var version int
	if err := s.Get(&version, `PRAGMA user_version`); err != nil {
		return fmt.Errorf("getting database version: %w", err)
	}
	s.log.Debug("got PRAGMA", "user_version", version)
	if version != 1 {
		s.log.Info("initializing database")
		if _, err := s.Exec(schema1); err != nil {
			return fmt.Errorf("initializing database: %w", err)
		}
	}
	return nil
}

func (s *Store) RecordToken(ctx context.Context, sub, iat string) error {
	_, err := s.ExecContext(ctx, "INSERT INTO tokens_issued (sub, iat) VALUES (?,?)", sub, iat)
	return err
}

func (s *Store) TokenIsRevoked(ctx context.Context, sub, iat string) (bool, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT * FROM tokens_issued
	WHERE revoked AND sub = ? AND iat = ?`, sub, iat)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	defer rows.Close()
	return rows.Next(), err
}

type promotable[O any] interface {
	Promote() O
}

func selectT[I promotable[*O], O any](ctx context.Context, db *sqlx.DB, query string, args ...any) ([]*O, error) {
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

func getT[I promotable[*O], O any](ctx context.Context, db *sqlx.DB, query string, args ...any) (*O, error) {
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
