package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// All monitor and recovery side effects run on one database session while a
// per-account advisory lock is held. Session loss releases the lock, so monitor
// processing must not publish external state after its database commit.
type monitorConnDriver struct {
	*entsql.Driver
	conn *sql.Conn
}

func (d *monitorConnDriver) BeginTx(ctx context.Context, opts *sql.TxOptions) (dialect.Tx, error) {
	tx, err := d.conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &entsql.Tx{Conn: entsql.Conn{ExecQuerier: tx}, Tx: tx}, nil
}
func (d *monitorConnDriver) Tx(ctx context.Context) (dialect.Tx, error) { return d.BeginTx(ctx, nil) }
func (d *monitorConnDriver) Close() error                               { return nil }

func (r *accountRepository) WithFirstTokenMonitorGuard(ctx context.Context, id int64, fn func(service.AccountRepository) error) error {
	db, ok := r.sql.(*sql.DB)
	if !ok {
		return errors.New("first token monitor requires a database session")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", -id); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock($1)", -id); err != nil {
			// Never return a session with a held lock to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	scoped := *r
	scoped.sql = conn
	scoped.client = dbent.NewClient(dbent.Driver(&monitorConnDriver{Driver: entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: conn}), conn: conn}))
	return fn(&scoped)
}

func (r *accountRepository) SetFirstTokenMonitorPause(ctx context.Context, id, generation int64, until time.Time, reason string) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
        WITH updated AS (
            UPDATE accounts SET temp_unschedulable_until=$1, temp_unschedulable_reason=$2, updated_at=NOW()
            WHERE id=$3 AND deleted_at IS NULL
              AND credentials->>'first_token_monitor_enabled'='true'
              AND COALESCE((credentials->>'first_token_monitor_generation')::bigint,0)=$4
              AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until<$1)
            RETURNING id
        )
        INSERT INTO scheduler_outbox(event_type,account_id)
        SELECT $5,id FROM updated
    `, until, reason, id, generation, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	// Do not publish a snapshot after commit: the session can be lost before
	// publication. The atomic outbox consumer reads current database state.
	return true, nil
}

// Compare the current pause as well as serializing against monitor recovery:
// unrelated pause writers need not hold the monitor advisory lock.
func (r *accountRepository) ClearCredentialRefreshPause(ctx context.Context, id int64, until time.Time, reason string) (bool, error) {
	result, err := r.sql.ExecContext(ctx, `
        WITH updated AS (
            UPDATE accounts SET temp_unschedulable_until=NULL, temp_unschedulable_reason=NULL,
                updated_at=NOW()
            WHERE id=$1 AND deleted_at IS NULL
                AND temp_unschedulable_until=$2 AND temp_unschedulable_reason=$3
                AND (temp_unschedulable_reason LIKE 'OAuth 401:%'
                    OR temp_unschedulable_reason='Authentication failed (401): invalid or expired credentials'
                    OR temp_unschedulable_reason='token refresh retry exhausted'
                    OR temp_unschedulable_reason LIKE 'token refresh retry exhausted:%')
            RETURNING id
        )
        INSERT INTO scheduler_outbox(event_type,account_id)
        SELECT $4,id FROM updated
    `, id, until, reason, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshot(ctx, id)
	return true, nil
}
