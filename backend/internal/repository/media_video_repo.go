package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type mediaVideoRepository struct{ db *sql.DB }

const mediaVideoSelectColumns = `task_id,COALESCE(upstream_task_id,''),user_id,api_key_id,COALESCE(group_id,0),COALESCE(upstream_account_id,0),model,prompt_hash,duration,ratio,resolution,has_images,idempotency_key_hash,request_hash,status,progress,downloadable,COALESCE(error::text,''),created_at,updated_at,completed_at,expires_at,last_polled_at,next_poll_at,billing_status,price_snapshot,hold_amount,actual_amount,currency,held_at,settled_at,released_at,settlement_attempts,next_settlement_at,COALESCE(last_billing_error,''),submission_state,poll_token,poll_failures`

func NewMediaVideoRepository(db *sql.DB) service.MediaVideoRepository {
	return &mediaVideoRepository{db: db}
}

func (r *mediaVideoRepository) Balance(ctx context.Context, userID int64) (float64, float64, error) {
	var available, frozen float64
	err := r.db.QueryRowContext(ctx, `SELECT balance,frozen_balance FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&available, &frozen)
	return available, frozen, err
}
func (r *mediaVideoRepository) Insert(ctx context.Context, t *service.MediaVideoTask) (bool, error) {
	q := `INSERT INTO media_video_tasks(task_id,upstream_task_id,user_id,api_key_id,group_id,upstream_account_id,model,prompt_hash,duration,ratio,resolution,has_images,idempotency_key_hash,request_hash,status,progress,downloadable,error,created_at,updated_at,completed_at,expires_at,last_polled_at,next_poll_at,billing_status,price_snapshot,hold_amount,actual_amount,currency,held_at,settled_at,released_at,settlement_attempts,next_settlement_at,last_billing_error) VALUES($1,NULL,$2,$3,NULLIF($4,0),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NULL,$20,NULL,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32) ON CONFLICT(user_id,api_key_id,idempotency_key_hash) DO UPDATE SET task_id=EXCLUDED.task_id,upstream_task_id=NULL,group_id=EXCLUDED.group_id,upstream_account_id=EXCLUDED.upstream_account_id,model=EXCLUDED.model,prompt_hash=EXCLUDED.prompt_hash,duration=EXCLUDED.duration,ratio=EXCLUDED.ratio,resolution=EXCLUDED.resolution,has_images=EXCLUDED.has_images,request_hash=EXCLUDED.request_hash,status=EXCLUDED.status,progress=NULL,downloadable=FALSE,error=NULL,created_at=EXCLUDED.created_at,updated_at=EXCLUDED.updated_at,completed_at=NULL,expires_at=EXCLUDED.expires_at,last_polled_at=NULL,next_poll_at=EXCLUDED.next_poll_at,billing_status=EXCLUDED.billing_status,price_snapshot=EXCLUDED.price_snapshot,hold_amount=EXCLUDED.hold_amount,actual_amount=NULL,currency=EXCLUDED.currency,held_at=NULL,settled_at=NULL,released_at=NULL,settlement_attempts=0,next_settlement_at=NULL,last_billing_error=NULL,submission_state='prepared',submission_started_at=NULL,poll_token='',poll_lease_until=NULL,poll_failures=0 WHERE media_video_tasks.expires_at<=NOW() AND media_video_tasks.billing_status IN ('not_billed','released','settled')`
	res, e := r.db.ExecContext(ctx, q, t.TaskID, t.UserID, t.APIKeyID, t.GroupID, t.UpstreamAccountID, t.Model, t.PromptHash, t.Duration, t.Ratio, t.Resolution, t.HasImages, t.IdempotencyKeyHash, t.RequestHash, t.Status, t.Progress, t.Downloadable, nullJSON(t.Error), t.CreatedAt, t.UpdatedAt, t.ExpiresAt, t.NextPollAt, t.BillingStatus, t.PriceSnapshot, t.HoldAmount, t.ActualAmount, t.Currency, t.HeldAt, t.SettledAt, t.ReleasedAt, t.SettlementAttempts, t.NextSettlementAt, mediaVideoNullString(t.LastBillingError))
	if e != nil {
		return false, translatePersistenceError(e, nil, nil)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
func mediaVideoNullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func (r *mediaVideoRepository) GetByIdempotency(ctx context.Context, u, k int64, h string) (*service.MediaVideoTask, error) {
	return r.get(ctx, `WHERE user_id=$1 AND api_key_id=$2 AND idempotency_key_hash=$3`, u, k, h)
}
func (r *mediaVideoRepository) Get(ctx context.Context, id string, u, k int64) (*service.MediaVideoTask, error) {
	return r.get(ctx, `WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, id, u, k)
}
func (r *mediaVideoRepository) get(ctx context.Context, where string, args ...any) (*service.MediaVideoTask, error) {
	q := `SELECT ` + mediaVideoSelectColumns + ` FROM media_video_tasks ` + where
	t := &service.MediaVideoTask{}
	var errText string
	var lp, np, held, settled, released, nextSettlement sql.NullTime
	var comp sql.NullTime
	var prog sql.NullFloat64
	var actual sql.NullFloat64
	err := r.db.QueryRowContext(ctx, q, args...).Scan(&t.TaskID, &t.UpstreamTaskID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.UpstreamAccountID, &t.Model, &t.PromptHash, &t.Duration, &t.Ratio, &t.Resolution, &t.HasImages, &t.IdempotencyKeyHash, &t.RequestHash, &t.Status, &prog, &t.Downloadable, &errText, &t.CreatedAt, &t.UpdatedAt, &comp, &t.ExpiresAt, &lp, &np, &t.BillingStatus, &t.PriceSnapshot, &t.HoldAmount, &actual, &t.Currency, &held, &settled, &released, &t.SettlementAttempts, &nextSettlement, &t.LastBillingError, &t.SubmissionState, &t.PollToken, &t.PollFailures)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if prog.Valid {
		t.Progress = &prog.Float64
	}
	if errText != "" {
		t.Error = json.RawMessage(errText)
	}
	if comp.Valid {
		t.CompletedAt = &comp.Time
	}
	if lp.Valid {
		t.LastPolledAt = &lp.Time
	}
	if np.Valid {
		t.NextPollAt = &np.Time
	}
	if actual.Valid {
		t.ActualAmount = &actual.Float64
	}
	if held.Valid {
		t.HeldAt = &held.Time
	}
	if settled.Valid {
		t.SettledAt = &settled.Time
	}
	if released.Valid {
		t.ReleasedAt = &released.Time
	}
	if nextSettlement.Valid {
		t.NextSettlementAt = &nextSettlement.Time
	}
	return t, nil
}
func (r *mediaVideoRepository) List(ctx context.Context, u, k int64, limit int, status, cursor string) ([]*service.MediaVideoTask, bool, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	allowed := make([]string, 0, 2)
	for _, candidate := range strings.Split(status, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "queued" || candidate == "running" {
			allowed = append(allowed, candidate)
		}
	}
	if len(allowed) == 0 {
		allowed = []string{"queued", "running"}
	}
	statusCSV := strings.Join(allowed, ",")
	var q string
	var args []any
	if cursor == "" {
		q = `SELECT ` + mediaVideoSelectColumns + ` FROM media_video_tasks WHERE user_id=$1 AND api_key_id=$2 AND status = ANY(string_to_array($3,',')) AND expires_at>NOW() ORDER BY created_at DESC, task_id DESC LIMIT $4`
		args = []any{u, k, statusCSV, limit + 1}
	} else {
		cursorTime, cursorTaskID, err := decodeMediaVideoCursor(cursor)
		if err != nil {
			return nil, false, "", err
		}
		q = `SELECT ` + mediaVideoSelectColumns + ` FROM media_video_tasks WHERE user_id=$1 AND api_key_id=$2 AND status = ANY(string_to_array($3,',')) AND expires_at>NOW() AND (created_at,task_id)<($4,$5) ORDER BY created_at DESC, task_id DESC LIMIT $6`
		args = []any{u, k, statusCSV, cursorTime, cursorTaskID, limit + 1}
	}
	items, err := r.list(ctx, q, args...)
	if err != nil {
		return nil, false, "", err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		next = encodeMediaVideoCursor(items[len(items)-1])
	}
	return items, hasMore, next, nil
}

func encodeMediaVideoCursor(task *service.MediaVideoTask) string {
	if task == nil || task.CreatedAt == nil {
		return ""
	}
	raw := task.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + task.TaskID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMediaVideoCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", service.ErrLaogouInvalidCursor
	}
	tm, err := time.Parse(time.RFC3339Nano, parts[0])
	return tm, parts[1], err
}
func (r *mediaVideoRepository) ListPollable(ctx context.Context, now time.Time, limit int) ([]*service.MediaVideoTask, error) {
	return r.claimPollable(ctx, now, limit, "")
}

func (r *mediaVideoRepository) ClaimPollable(ctx context.Context, id string, now time.Time) (*service.MediaVideoTask, error) {
	tasks, err := r.claimPollable(ctx, now, 1, id)
	if err != nil || len(tasks) == 0 {
		return nil, err
	}
	return tasks[0], nil
}

func (r *mediaVideoRepository) claimPollable(ctx context.Context, now time.Time, limit int, id string) ([]*service.MediaVideoTask, error) {
	q := `WITH claimed AS (
		SELECT task_id FROM media_video_tasks
		WHERE status IN ('queued','running') AND (expires_at>$1 OR submission_state='reconciling')
		  AND ($4='' OR task_id=$4)
		  AND (next_poll_at IS NULL OR next_poll_at<=$1)
		  AND upstream_task_id IS NOT NULL AND (poll_lease_until IS NULL OR poll_lease_until<=$1)
		ORDER BY next_poll_at NULLS FIRST LIMIT $2 FOR UPDATE SKIP LOCKED
	), updated AS (
		UPDATE media_video_tasks t SET last_polled_at=$1, poll_lease_until=$1 + INTERVAL '90 seconds',poll_token=$3
		FROM claimed c WHERE t.task_id=c.task_id RETURNING t.*
	)
	SELECT ` + mediaVideoSelectColumns + ` FROM updated`
	return r.list(ctx, q, now, limit, uuid.NewString(), id)
}

func (r *mediaVideoRepository) ListBillingRecovery(ctx context.Context, now time.Time, limit int) ([]*service.MediaVideoTask, error) {
	// Recovery probes share the polling lease so a second worker cannot bypass
	// a content Retry-After via the billing recovery queue.
	q := `WITH claimed AS (
	 SELECT task_id FROM media_video_tasks WHERE billing_status IN ('settlement_pending','release_pending')
	 AND (expires_at>$1 OR submission_state IN ('reconciling','dispute_refund','rejected'))
	 AND (next_settlement_at IS NULL OR next_settlement_at<=$1) AND (poll_lease_until IS NULL OR poll_lease_until<=$1)
	 ORDER BY next_settlement_at NULLS FIRST LIMIT $2 FOR UPDATE SKIP LOCKED
	), updated AS (
	 UPDATE media_video_tasks t SET poll_token=$3,poll_lease_until=$1+INTERVAL '90 seconds'
	 FROM claimed c WHERE t.task_id=c.task_id RETURNING t.*
	) SELECT ` + mediaVideoSelectColumns + ` FROM updated`
	return r.list(ctx, q, now, limit, uuid.NewString())
}

func (r *mediaVideoRepository) ListExpiredForRelease(ctx context.Context, now time.Time, limit int) ([]*service.MediaVideoTask, error) {
	q := `SELECT ` + mediaVideoSelectColumns + ` FROM media_video_tasks WHERE expires_at<=$1 AND submission_state NOT IN ('submitting','uncertain','reconciling') AND billing_status IN ('hold_pending','held','settlement_pending','release_pending') ORDER BY expires_at LIMIT $2`
	return r.list(ctx, q, now, limit)
}
func (r *mediaVideoRepository) list(ctx context.Context, q string, args ...any) ([]*service.MediaVideoTask, error) {
	rows, e := r.db.QueryContext(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []*service.MediaVideoTask{}
	for rows.Next() {
		t := &service.MediaVideoTask{}
		var errText string
		var lp, np, comp, held, settled, released, nextSettlement sql.NullTime
		var prog sql.NullFloat64
		var actual sql.NullFloat64
		if e = rows.Scan(&t.TaskID, &t.UpstreamTaskID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.UpstreamAccountID, &t.Model, &t.PromptHash, &t.Duration, &t.Ratio, &t.Resolution, &t.HasImages, &t.IdempotencyKeyHash, &t.RequestHash, &t.Status, &prog, &t.Downloadable, &errText, &t.CreatedAt, &t.UpdatedAt, &comp, &t.ExpiresAt, &lp, &np, &t.BillingStatus, &t.PriceSnapshot, &t.HoldAmount, &actual, &t.Currency, &held, &settled, &released, &t.SettlementAttempts, &nextSettlement, &t.LastBillingError, &t.SubmissionState, &t.PollToken, &t.PollFailures); e != nil {
			return nil, e
		}
		if prog.Valid {
			t.Progress = &prog.Float64
		}
		if errText != "" {
			t.Error = json.RawMessage(errText)
		}
		if comp.Valid {
			t.CompletedAt = &comp.Time
		}
		if lp.Valid {
			t.LastPolledAt = &lp.Time
		}
		if np.Valid {
			t.NextPollAt = &np.Time
		}
		if actual.Valid {
			t.ActualAmount = &actual.Float64
		}
		if held.Valid {
			t.HeldAt = &held.Time
		}
		if settled.Valid {
			t.SettledAt = &settled.Time
		}
		if released.Valid {
			t.ReleasedAt = &released.Time
		}
		if nextSettlement.Valid {
			t.NextSettlementAt = &nextSettlement.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *mediaVideoRepository) UpdateUpstream(ctx context.Context, id, up, st string, account int64) error {
	res, e := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET upstream_task_id=$2,status=$3,submission_state='accepted',updated_at=NOW(),next_poll_at=NOW() WHERE task_id=$1 AND upstream_account_id=$4 AND (upstream_task_id IS NULL OR upstream_task_id=$2) AND status='creating' AND billing_status IN ('held','settlement_pending')`, id, up, st, account)
	return mediaVideoWriteResult(res, e)
}
func (r *mediaVideoRepository) UpdateStatus(ctx context.Context, id, st string, p *float64, dl bool, er json.RawMessage, now, next time.Time) error {
	res, e := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET status=$2,progress=$3,downloadable=$4,error=$5,updated_at=$6,submission_state=CASE WHEN $2='failed' AND upstream_task_id IS NULL THEN 'rejected' ELSE submission_state END,completed_at=CASE WHEN $2 IN ('succeeded','failed') THEN $6 ELSE completed_at END,next_poll_at=$7,poll_failures=0,poll_lease_until=NULL,poll_token='' WHERE task_id=$1 AND status NOT IN ('succeeded','failed') AND (($8<>'' AND poll_token=$8 AND poll_lease_until>NOW()) OR ($8='' AND status='creating' AND upstream_task_id IS NULL AND $2='failed')) AND (billing_status<>'settled' OR $2='succeeded') AND (billing_status<>'released' OR $2='failed')`, id, st, p, dl, nullJSON(er), now, next, service.MediaVideoPollToken(ctx))
	return mediaVideoWriteResult(res, e)
}
func (r *mediaVideoRepository) MarkPolled(ctx context.Context, id string, now, next time.Time) error {
	res, e := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET last_polled_at=$2,next_poll_at=$3,next_settlement_at=CASE WHEN billing_status='settlement_pending' THEN $3 ELSE next_settlement_at END,updated_at=$2,poll_failures=poll_failures+1,poll_lease_until=NULL,poll_token='' WHERE task_id=$1 AND status IN ('queued','running') AND $4<>'' AND poll_token=$4 AND poll_lease_until>NOW()`, id, now, next, service.MediaVideoPollToken(ctx))
	return mediaVideoWriteResult(res, e)
}

func mediaVideoWriteResult(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrMediaVideoStateConflict
	}
	return nil
}

func (r *mediaVideoRepository) ClaimSubmission(ctx context.Context, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET submission_state='submitting',submission_started_at=NOW(),updated_at=NOW() WHERE task_id=$1 AND submission_state='prepared' AND status='creating' AND billing_status='held' AND expires_at>NOW()`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *mediaVideoRepository) RecoverSubmissions(ctx context.Context) error {
	// Never resubmit an ambiguous POST without a confirmed provider contract.
	_, err := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET submission_state='uncertain',last_billing_error='Provider creation outcome requires reconciliation',updated_at=NOW() WHERE submission_state='submitting' AND upstream_task_id IS NULL AND submission_started_at<NOW()-INTERVAL '3 minutes'`)
	return err
}
func (r *mediaVideoRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM media_video_tasks WHERE task_id IN (SELECT task_id FROM media_video_tasks WHERE expires_at<=$1 AND billing_status IN ('not_billed','released','settled') ORDER BY expires_at LIMIT $2)`, now, limit)
	return e
}

func (r *mediaVideoRepository) Delete(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_video_tasks WHERE task_id=$1 AND billing_status IN ('settled','released','not_billed')`, taskID)
	return err
}

func (r *mediaVideoRepository) UpdateBilling(ctx context.Context, taskID, status string, actual *float64, message string, now time.Time) error {
	res, err := r.db.ExecContext(ctx, `UPDATE media_video_tasks SET billing_status=$2,actual_amount=COALESCE($3,actual_amount),held_at=CASE WHEN $2='held' THEN COALESCE(held_at,$5) ELSE held_at END,settled_at=CASE WHEN $2='settled' THEN COALESCE(settled_at,$5) ELSE settled_at END,released_at=CASE WHEN $2='released' THEN COALESCE(released_at,$5) ELSE released_at END,settlement_attempts=CASE WHEN $2 IN ('settlement_pending','release_pending') THEN settlement_attempts+1 ELSE settlement_attempts END,next_settlement_at=CASE WHEN $2 IN ('settlement_pending','release_pending') THEN $5+INTERVAL '30 seconds' ELSE NULL END,last_billing_error=NULLIF($4,''),updated_at=$5
	 WHERE task_id=$1 AND (
	   ($2 IN ('held','settled','released') AND billing_status=$2 AND ($6='' OR (poll_token=$6 AND poll_lease_until>NOW())))
	   OR ($2='settlement_pending' AND billing_status IN ('held','settlement_pending')
	       AND status IN ('queued','running') AND submission_state IN ('accepted','reconciling')
	       AND $6<>'' AND poll_token=$6 AND poll_lease_until>NOW()
	       AND NOT EXISTS(SELECT 1 FROM media_video_ledger WHERE task_id=$1 AND operation='dispute_refund_decision'))
	   OR ($2='release_pending' AND billing_status IN ('hold_pending','held','settlement_pending','release_pending')
	       AND (($6<>'' AND poll_token=$6 AND poll_lease_until>NOW()) OR ($6='' AND status='failed' AND poll_token='')))
	 )`, taskID, status, actual, message, now, service.MediaVideoPollToken(ctx))
	return mediaVideoWriteResult(res, err)
}
