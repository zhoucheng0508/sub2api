-- Authentication revocation must not remove the active financial order.
ALTER TABLE media_video_tasks DROP CONSTRAINT IF EXISTS media_video_tasks_api_key_id_fkey;
ALTER TABLE media_video_tasks ADD CONSTRAINT media_video_tasks_api_key_id_fkey
    FOREIGN KEY(api_key_id) REFERENCES api_keys(id) ON DELETE RESTRICT;

-- Access expiry is not a deadline for recording an honest supplier result.
CREATE OR REPLACE FUNCTION reconcile_media_video_submission(
    local_task TEXT, bound_account BIGINT, provider_task TEXT, rejected BOOLEAN, evidence TEXT
) RETURNS void AS $$
DECLARE order_row media_video_tasks%ROWTYPE; previous JSONB;
BEGIN
    IF rejected IS NULL OR evidence IS NULL OR length(trim(evidence))<10 THEN
        RAISE EXCEPTION 'Supplier reconciliation evidence and result are required';
    END IF;
    SELECT * INTO STRICT order_row FROM media_video_tasks WHERE task_id=local_task FOR UPDATE;
    IF order_row.upstream_account_id IS DISTINCT FROM bound_account THEN
        RAISE EXCEPTION 'Bound supplier account does not match';
    END IF;
    SELECT order_snapshot INTO previous FROM media_video_ledger WHERE task_id=local_task AND operation='reconciliation';
    IF FOUND THEN
        IF previous->>'evidence'=evidence
           AND (previous->>'confirmed_rejected')::BOOLEAN=rejected
           AND previous->>'confirmed_provider_task' IS NOT DISTINCT FROM provider_task THEN RETURN; END IF;
        RAISE EXCEPTION 'Conflicting reconciliation decision';
    END IF;
    IF order_row.submission_state NOT IN ('submitting','uncertain')
        OR order_row.billing_status<>'held' OR order_row.status<>'creating' THEN
        RAISE EXCEPTION 'Order is not eligible for submission reconciliation';
    END IF;
    IF NOT rejected AND (provider_task IS NULL OR provider_task !~ '^[A-Za-z0-9_-]+$') THEN
        RAISE EXCEPTION 'A valid provider task is required';
    END IF;
    INSERT INTO media_video_ledger(task_id,operation,user_id,api_key_id,amount,order_snapshot)
    VALUES(local_task,'reconciliation',order_row.user_id,order_row.api_key_id,0,
        to_jsonb(order_row)||jsonb_build_object('evidence',evidence,'operator',session_user,
            'confirmed_provider_task',provider_task,'confirmed_rejected',rejected));
    UPDATE media_video_tasks SET
        upstream_task_id=CASE WHEN rejected THEN NULL ELSE provider_task END,
        submission_state=CASE WHEN rejected THEN 'rejected' ELSE 'reconciling' END,
        status=CASE WHEN rejected THEN 'failed' ELSE 'queued' END,
        billing_status=CASE WHEN rejected THEN 'release_pending' ELSE billing_status END,
        completed_at=CASE WHEN rejected THEN NOW() ELSE completed_at END,
        next_poll_at=NOW(),next_settlement_at=NOW(),poll_token='',poll_lease_until=NULL,updated_at=NOW()
    WHERE task_id=local_task;
END;
$$ LANGUAGE plpgsql;
REVOKE ALL ON FUNCTION reconcile_media_video_submission(TEXT,BIGINT,TEXT,BOOLEAN,TEXT) FROM PUBLIC;

-- A dispute refund records an operator decision, never a false supplier failure.
-- It schedules the same idempotent release transaction used by normal failures.
CREATE OR REPLACE FUNCTION refund_media_video_dispute(
    local_task TEXT, bound_account BIGINT, evidence TEXT
) RETURNS void AS $$
DECLARE order_row media_video_tasks%ROWTYPE; previous JSONB;
BEGIN
    IF evidence IS NULL OR length(trim(evidence))<10 THEN RAISE EXCEPTION 'Dispute evidence is required'; END IF;
    SELECT * INTO STRICT order_row FROM media_video_tasks WHERE task_id=local_task FOR UPDATE;
    IF order_row.upstream_account_id IS DISTINCT FROM bound_account THEN RAISE EXCEPTION 'Bound supplier account does not match'; END IF;
    SELECT order_snapshot INTO previous FROM media_video_ledger WHERE task_id=local_task AND operation='dispute_refund_decision';
    IF FOUND THEN
        IF previous->>'evidence'=evidence THEN RETURN; END IF;
        RAISE EXCEPTION 'Conflicting dispute refund decision';
    END IF;
    IF order_row.billing_status NOT IN ('held','settlement_pending')
        OR order_row.submission_state NOT IN ('uncertain','reconciling','accepted') THEN
        RAISE EXCEPTION 'Order is not eligible for dispute refund';
    END IF;
    INSERT INTO media_video_ledger(task_id,operation,user_id,api_key_id,amount,order_snapshot)
    VALUES(local_task,'dispute_refund_decision',order_row.user_id,order_row.api_key_id,0,
        to_jsonb(order_row)||jsonb_build_object('evidence',evidence,'operator',session_user,'decision','refund_without_supplier_failure_claim'));
    UPDATE media_video_tasks SET submission_state='dispute_refund',status='failed',billing_status='release_pending',
        error=jsonb_build_object('type','dispute_refund','message','Operator approved refund; supplier outcome is retained in the audit ledger'),
        downloadable=FALSE,completed_at=NOW(),next_settlement_at=NOW(),poll_token='',poll_lease_until=NULL,updated_at=NOW()
    WHERE task_id=local_task;
END;
$$ LANGUAGE plpgsql;
REVOKE ALL ON FUNCTION refund_media_video_dispute(TEXT,BIGINT,TEXT) FROM PUBLIC;
