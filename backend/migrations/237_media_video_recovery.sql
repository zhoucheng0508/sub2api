-- Short lived task presentation is independent of permanent financial evidence.
CREATE TABLE IF NOT EXISTS media_video_ledger (
    task_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    amount NUMERIC(20,8) NOT NULL,
    order_snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(task_id, operation)
);
CREATE TABLE IF NOT EXISTS media_video_order_archive (
    task_id TEXT PRIMARY KEY,
    order_snapshot JSONB NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE media_video_tasks
    ADD COLUMN IF NOT EXISTS submission_state TEXT NOT NULL DEFAULT 'prepared',
    ADD COLUMN IF NOT EXISTS submission_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS poll_token TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS poll_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS poll_failures INTEGER NOT NULL DEFAULT 0;
-- A pre-upgrade creating row may already have reached the provider.
UPDATE media_video_tasks SET submission_state=CASE
    WHEN upstream_task_id IS NOT NULL THEN 'accepted'
    WHEN status='creating' AND billing_status <> 'hold_pending' THEN 'uncertain'
    ELSE submission_state END;
UPDATE media_video_tasks SET status='running'
    WHERE upstream_task_id IS NOT NULL AND status NOT IN ('queued','running','succeeded','failed');

-- Archive before both TTL deletion and expired idempotency-key replacement.
CREATE OR REPLACE FUNCTION archive_media_video_order() RETURNS trigger AS $$
BEGIN
    INSERT INTO media_video_order_archive(task_id,order_snapshot)
    VALUES(OLD.task_id,to_jsonb(OLD)) ON CONFLICT(task_id) DO NOTHING;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS media_video_archive_delete ON media_video_tasks;
CREATE TRIGGER media_video_archive_delete BEFORE DELETE ON media_video_tasks
    FOR EACH ROW EXECUTE FUNCTION archive_media_video_order();
DROP TRIGGER IF EXISTS media_video_archive_replace ON media_video_tasks;
CREATE TRIGGER media_video_archive_replace BEFORE UPDATE OF task_id ON media_video_tasks
    FOR EACH ROW WHEN (OLD.task_id IS DISTINCT FROM NEW.task_id)
    EXECUTE FUNCTION archive_media_video_order();

-- Operator-only reconciliation. The caller must verify the task using the
-- original provider account. This function never submits or moves money.
CREATE OR REPLACE FUNCTION reconcile_media_video_submission(
    local_task TEXT, bound_account BIGINT, provider_task TEXT, rejected BOOLEAN, evidence TEXT
) RETURNS void AS $$
DECLARE order_row media_video_tasks%ROWTYPE;
BEGIN
    IF evidence IS NULL OR length(trim(evidence))<10 THEN
        RAISE EXCEPTION 'Supplier reconciliation evidence is required';
    END IF;
    SELECT * INTO STRICT order_row FROM media_video_tasks WHERE task_id=local_task FOR UPDATE;
    IF order_row.upstream_account_id IS DISTINCT FROM bound_account
        OR order_row.submission_state NOT IN ('submitting','uncertain')
        OR order_row.billing_status<>'held' OR order_row.status<>'creating' THEN
        RAISE EXCEPTION 'Order is not eligible for submission reconciliation';
    END IF;
    IF NOT rejected AND (provider_task IS NULL OR provider_task !~ '^[A-Za-z0-9_-]+$' OR order_row.expires_at<=NOW()) THEN
        RAISE EXCEPTION 'A valid, unexpired provider task is required';
    END IF;
    INSERT INTO media_video_ledger(task_id,operation,user_id,api_key_id,amount,order_snapshot)
    VALUES(local_task,'reconciliation',order_row.user_id,order_row.api_key_id,0,
        to_jsonb(order_row)||jsonb_build_object('evidence',evidence,'confirmed_provider_task',provider_task,'confirmed_rejected',rejected));
    UPDATE media_video_tasks SET
        upstream_task_id=CASE WHEN rejected THEN NULL ELSE provider_task END,
        submission_state=CASE WHEN rejected THEN 'rejected' ELSE 'accepted' END,
        status=CASE WHEN rejected THEN 'failed' ELSE 'queued' END,
        billing_status=CASE WHEN rejected THEN 'release_pending' ELSE billing_status END,
        completed_at=CASE WHEN rejected THEN NOW() ELSE completed_at END,
        next_poll_at=NOW(),next_settlement_at=NOW(),updated_at=NOW()
    WHERE task_id=local_task;
END;
$$ LANGUAGE plpgsql;
REVOKE ALL ON FUNCTION reconcile_media_video_submission(TEXT,BIGINT,TEXT,BOOLEAN,TEXT) FROM PUBLIC;
