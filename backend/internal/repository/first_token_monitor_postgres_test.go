package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// Uses a private schema on the explicitly configured local test database.
// Never connects to a database implicitly or modifies existing account rows.
func TestFirstTokenMonitorPostgresFencingAndRecovery(t *testing.T) {
	dsn := os.Getenv("FIRST_TOKEN_MONITOR_TEST_DSN")
	if dsn == "" {
		t.Skip("set FIRST_TOKEN_MONITOR_TEST_DSN for the local PostgreSQL test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = base.Close() }()
	schema := fmt.Sprintf("ftm_review_%d", time.Now().UnixNano())
	_, err = base.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer func() {
		_, err := base.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		require.NoError(t, err)
	}()
	db, err := sql.Open("postgres", dsn+" search_path="+schema+",public")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1) // Scoped operations must not consume a second pooled connection.
	_, err = db.ExecContext(ctx, `CREATE TABLE accounts (LIKE public.accounts INCLUDING DEFAULTS);
        CREATE TABLE scheduler_outbox(id bigint GENERATED ALWAYS AS IDENTITY,event_type text,account_id bigint,group_id bigint,payload jsonb,dedup_key text);
        CREATE UNIQUE INDEX monitor_test_outbox_dedup ON scheduler_outbox(dedup_key) WHERE dedup_key IS NOT NULL;`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("243_first_token_monitor_generation.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	migration, err = migrations.FS.ReadFile("244_account_scheduler_state_version.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	id := time.Now().UnixNano() / 1000
	_, err = db.ExecContext(ctx, `INSERT INTO accounts(id,name,platform,type,credentials) VALUES($1,'monitor-test','openai','apikey','{"first_token_monitor_enabled":true}')`, id)
	require.NoError(t, err)
	getGeneration := func() int64 {
		var gen int64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT (credentials->>'first_token_monitor_generation')::bigint FROM accounts WHERE id=$1`, id).Scan(&gen))
		return gen
	}
	require.EqualValues(t, 1, getGeneration())
	// Exercise partial/bulk-style JSON merges, without a client-provided revision.
	_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials=credentials||'{"first_token_monitor_enabled":false}'::jsonb WHERE id=$1`, id)
	require.NoError(t, err)
	require.EqualValues(t, 2, getGeneration())
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	applied, err := repo.SetFirstTokenMonitorPause(ctx, id, 1, time.Now().Add(time.Minute), "stale")
	require.NoError(t, err)
	require.False(t, applied)
	_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials=credentials||'{"first_token_monitor_enabled":true,"first_token_monitor_generation":999}'::jsonb WHERE id=$1`, id)
	require.NoError(t, err)
	require.EqualValues(t, 3, getGeneration())
	observed, release := make(chan struct{}), make(chan struct{})
	monitorDone, recoveryDone := make(chan error, 1), make(chan error, 1)
	go func() {
		monitorDone <- repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error {
			current, err := scoped.GetByID(ctx, id)
			if err != nil {
				return err
			}
			if current.Credentials["first_token_monitor_generation"] != float64(3) {
				return fmt.Errorf("unexpected generation")
			}
			close(observed)
			<-release
			guard, valid := scoped.(service.FirstTokenMonitorGuard)
			if !valid {
				return fmt.Errorf("repository does not implement monitor guard")
			}
			ok, err := guard.SetFirstTokenMonitorPause(ctx, id, 3, time.Now().Add(time.Minute), "test pause")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("current pause rejected")
			}
			return nil
		})
	}()
	select {
	case <-observed:
	case <-ctx.Done():
		t.Fatal("guarded read timed out")
	}
	go func() {
		recoveryDone <- repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error { return scoped.ClearTempUnschedulable(ctx, id) })
	}()
	close(release)
	require.NoError(t, <-monitorDone)
	require.NoError(t, <-recoveryDone)
	var until sql.NullTime
	require.NoError(t, db.QueryRowContext(ctx, `SELECT temp_unschedulable_until FROM accounts WHERE id=$1`, id).Scan(&until))
	require.False(t, until.Valid)
	require.EqualValues(t, 4, getGeneration())
	applied, err = repo.SetFirstTokenMonitorPause(ctx, id, 3, time.Now().Add(time.Minute), "late old pause")
	require.NoError(t, err)
	require.False(t, applied)
	// Confirm the connection-bound ent driver can run a transaction (used by
	// account edit code) without trying to cast sql.Conn to sql.DB.
	require.NoError(t, repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error {
		r, valid := scoped.(*accountRepository)
		if !valid {
			return fmt.Errorf("unexpected repository type")
		}
		tx, err := r.client.Tx(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Client().ExecContext(ctx, "SELECT 1")
		if err != nil {
			return err
		}
		return tx.Commit()
	}))
	if addr := os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_ADDR"); addr != "" {
		client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_PASSWORD")})
		defer func() { _ = client.Close() }()
		cache := NewTempUnschedCache(client)
		key := fmt.Sprintf("first_token_monitor:{%d}", id)
		keys := []string{key, key + ":samples", fmt.Sprintf("temp_unsched:account:%d", id)}
		for _, request := range []string{"slow-a", "slow-b", "slow-c", "normal-after-recovery"} {
			keys = append(keys, key+":request:"+request)
		}
		defer func() { require.NoError(t, client.Del(context.Background(), keys...).Err()) }()
		svc := service.NewRateLimitService(repo, nil, nil, nil, cache)
		ms := 31000
		for _, request := range []string{"slow-a", "slow-b", "slow-c"} {
			// Supply the dispatched account snapshot; monitor re-reads it using
			// the bound database session and calls the real Redis Lua script.
			var credentialsRaw []byte
			require.NoError(t, db.QueryRowContext(ctx, "SELECT credentials FROM accounts WHERE id=$1", id).Scan(&credentialsRaw))
			credentials := map[string]any{}
			require.NoError(t, json.Unmarshal(credentialsRaw, &credentials))
			svc.MonitorFirstToken(ctx, &service.Account{ID: id, Platform: service.PlatformOpenAI, Credentials: credentials}, &service.UsageLog{RequestID: request, FirstTokenMs: &ms})
		}
		require.NoError(t, db.QueryRowContext(ctx, "SELECT temp_unschedulable_until FROM accounts WHERE id=$1", id).Scan(&until))
		require.True(t, until.Valid)
		require.NoError(t, svc.ClearTempUnschedulable(ctx, id))
		require.NoError(t, db.QueryRowContext(ctx, "SELECT temp_unschedulable_until FROM accounts WHERE id=$1", id).Scan(&until))
		require.False(t, until.Valid)
		state, err := cache.GetTempUnsched(ctx, id)
		require.NoError(t, err)
		require.Nil(t, state)
		require.EqualValues(t, 5, getGeneration())
	}

	t.Run("terminated_session_recovery", func(t *testing.T) {
		db.SetMaxOpenConns(2)
		defer db.SetMaxOpenConns(1)
		generation := getGeneration()
		committed := make(chan int, 1)
		resume := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error {
				r, valid := scoped.(*accountRepository)
				if !valid {
					return fmt.Errorf("unexpected repository type")
				}
				var pid int
				conn, valid := r.sql.(*sql.Conn)
				if !valid {
					return fmt.Errorf("repository is not session bound")
				}
				if err := conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
					return err
				}
				applied, err := r.SetFirstTokenMonitorPause(ctx, id, generation, time.Now().Add(time.Minute), "First token monitor: terminated session")
				if err != nil {
					return err
				}
				if !applied {
					return fmt.Errorf("pause not committed")
				}
				committed <- pid
				<-resume
				// No external publication is part of this committed operation.
				return nil
			})
		}()
		var pid int
		select {
		case pid = <-committed:
		case err := <-done:
			t.Fatalf("pause failed: %v", err)
		case <-ctx.Done():
			t.Fatal("pause timed out")
		}
		var terminated bool
		require.NoError(t, base.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&terminated))
		require.True(t, terminated)
		recoveryErr := repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error {
			return scoped.ClearTempUnschedulable(ctx, id)
		})
		close(resume)
		require.NoError(t, recoveryErr)
		require.NoError(t, <-done)
		require.NoError(t, db.QueryRowContext(ctx, "SELECT temp_unschedulable_until FROM accounts WHERE id=$1", id).Scan(&until))
		require.False(t, until.Valid)
		require.Equal(t, generation+1, getGeneration())
	})

	t.Run("credential_pause_compare_and_clear", func(t *testing.T) {
		deadline := time.Now().Add(time.Minute).Truncate(time.Microsecond)
		require.NoError(t, repo.SetTempUnschedulable(ctx, id, deadline, "OAuth 401: expired"))
		// Another writer replaced the credential pause after the refresh read.
		require.NoError(t, repo.SetTempUnschedulable(ctx, id, deadline.Add(time.Minute), "First token monitor: slow"))
		applied, err := repo.ClearCredentialRefreshPause(ctx, id, deadline, "OAuth 401: expired")
		require.NoError(t, err)
		require.False(t, applied)
		applied, err = repo.ClearCredentialRefreshPause(ctx, id, deadline.Add(time.Minute), "First token monitor: slow")
		require.NoError(t, err)
		require.False(t, applied)
		require.NoError(t, repo.ClearTempUnschedulable(ctx, id))
		require.NoError(t, repo.SetTempUnschedulable(ctx, id, deadline, "OAuth 401: expired"))
		applied, err = repo.ClearCredentialRefreshPause(ctx, id, deadline, "OAuth 401: expired")
		require.NoError(t, err)
		require.True(t, applied)
	})

	t.Run("scheduler_version_pause_extend_recover_and_spoof", func(t *testing.T) {
		readAccount := func() *service.Account {
			var account *service.Account
			require.NoError(t, repo.WithFirstTokenMonitorGuard(ctx, id, func(scoped service.AccountRepository) error {
				var err error
				account, err = scoped.GetByID(ctx, id)
				return err
			}))
			return account
		}
		getVersion := func() int64 {
			var version int64
			require.NoError(t, db.QueryRowContext(ctx, "SELECT (credentials->>'scheduler_state_version')::bigint FROM accounts WHERE id=$1", id).Scan(&version))
			return version
		}
		generation := getGeneration()
		version := getVersion()
		beforePause := readAccount()
		deadline := time.Now().Add(time.Minute)
		applied, err := repo.SetFirstTokenMonitorPause(ctx, id, generation, deadline, "First token monitor: version test")
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, version+1, getVersion())
		require.Equal(t, generation, getGeneration(), "monitor generation does not change when pausing")
		if addr := os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_ADDR"); addr != "" {
			client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("FIRST_TOKEN_MONITOR_TEST_REDIS_PASSWORD")})
			defer func() { _ = client.Close() }()
			defer func() {
				require.NoError(t, client.Del(context.Background(), fmt.Sprintf("sched:acc:%d", id), fmt.Sprintf("sched:meta:%d", id), fmt.Sprintf("sched:monitor_generation:%d", id), fmt.Sprintf("sched:state_version:%d", id)).Err())
			}()
			cache := NewSchedulerCache(client)
			require.NoError(t, cache.SetAccount(ctx, readAccount()))
			require.NoError(t, cache.SetAccount(ctx, beforePause))
			cached, err := cache.GetAccount(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, cached.TempUnschedulableUntil, "late pre-pause database snapshot must be rejected")
		}
		applied, err = repo.SetFirstTokenMonitorPause(ctx, id, generation, deadline.Add(time.Minute), "First token monitor: extended")
		require.NoError(t, err)
		require.True(t, applied)
		require.Equal(t, version+2, getVersion())
		// Full credentials replacement cannot spoof or drop the row version.
		_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials=credentials||'{"scheduler_state_version":"999999"}'::jsonb WHERE id=$1`, id)
		require.NoError(t, err)
		require.Equal(t, version+3, getVersion())
		_, err = db.ExecContext(ctx, `UPDATE accounts SET credentials=credentials-'scheduler_state_version',name='updated during cooldown' WHERE id=$1`, id)
		require.NoError(t, err)
		require.Equal(t, version+4, getVersion())
		require.NoError(t, repo.ClearTempUnschedulable(ctx, id))
		require.Equal(t, version+5, getVersion())
		// Reapplying the migration must not reset established versions.
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		require.Equal(t, version+5, getVersion())
	})
}
