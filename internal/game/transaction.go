package game

import (
	"context"
	"database/sql"
)

type queries interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type transaction interface {
	queries
	Commit() error
	Rollback() error
}

// NewTransactionalWorldService includes all command effects in the core's
// checkpoint/outbox transaction. Individual game operations use savepoints so
// their existing rollback behaviour remains intact.
func NewTransactionalWorldService(tx *sql.Tx) *WorldService {
	return &WorldService{db: tx, outer: tx}
}

func (ws *WorldService) beginTx(ctx context.Context) (transaction, error) {
	if ws.outer == nil {
		return ws.pool.BeginTx(ctx, nil)
	}
	if _, err := ws.outer.ExecContext(ctx, "SAVEPOINT game_operation"); err != nil {
		return nil, err
	}
	return &savepoint{Tx: ws.outer, ctx: ctx}, nil
}

type savepoint struct {
	*sql.Tx
	ctx  context.Context
	done bool
}

func (s *savepoint) Commit() error {
	if s.done {
		return sql.ErrTxDone
	}
	_, err := s.Tx.ExecContext(s.ctx, "RELEASE SAVEPOINT game_operation")
	s.done = true
	return err
}

func (s *savepoint) Rollback() error {
	if s.done {
		return sql.ErrTxDone
	}
	s.done = true
	_, err := s.Tx.ExecContext(s.ctx, "ROLLBACK TO SAVEPOINT game_operation")
	if err != nil {
		return err
	}
	_, err = s.Tx.ExecContext(s.ctx, "RELEASE SAVEPOINT game_operation")
	return err
}
