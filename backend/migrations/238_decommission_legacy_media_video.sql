-- Decommission the database-backed media-video implementation removed in favor
-- of the Seedance delivery package. Only holds proven by durable billing events
-- are refunded; unrelated batch-image and current media holds share the same
-- users.frozen_balance column and must remain untouched.

DO $$
DECLARE
    insufficient_user_id BIGINT;
BEGIN
    IF to_regclass('media_video_tasks') IS NOT NULL THEN
        -- Serialize cleanup with any old application instance that is still
        -- polling or settling a legacy task during a rolling deployment.
        LOCK TABLE media_video_tasks IN ACCESS EXCLUSIVE MODE;

        CREATE TEMP TABLE legacy_media_video_pending_refunds (
            task_id TEXT PRIMARY KEY,
            user_id BIGINT NOT NULL,
            amount NUMERIC(20,8) NOT NULL
        ) ON COMMIT DROP;

        INSERT INTO legacy_media_video_pending_refunds(task_id, user_id, amount)
        SELECT task.task_id, task.user_id, task.hold_amount
        FROM media_video_tasks task
        WHERE task.hold_amount > 0
          AND EXISTS (
              SELECT 1
              FROM (
                  SELECT request_id, api_key_id FROM usage_billing_dedup
                  UNION ALL
                  SELECT request_id, api_key_id FROM usage_billing_dedup_archive
              ) event
              WHERE event.request_id = 'batch_image_hold:' || task.task_id
                AND event.api_key_id = task.api_key_id
          )
          AND NOT EXISTS (
              SELECT 1
              FROM (
                  SELECT request_id, api_key_id FROM usage_billing_dedup
                  UNION ALL
                  SELECT request_id, api_key_id FROM usage_billing_dedup_archive
              ) event
              WHERE event.request_id IN (
                        'batch_image_capture:' || task.task_id,
                        'batch_image_release:' || task.task_id
                    )
                AND event.api_key_id = task.api_key_id
          );

        -- A deficit means another process or a previous manual repair changed
        -- the shared pool. Stop for operator review instead of refunding money
        -- backed by another feature's hold.
        SELECT refund.user_id
          INTO insufficient_user_id
          FROM (
              SELECT user_id, SUM(amount) AS amount
              FROM legacy_media_video_pending_refunds
              GROUP BY user_id
          ) refund
          LEFT JOIN users ON users.id = refund.user_id
         WHERE users.id IS NULL
            OR COALESCE(users.frozen_balance, 0) < refund.amount
         LIMIT 1;

        IF insufficient_user_id IS NOT NULL THEN
            RAISE EXCEPTION
                'legacy media-video holds exceed frozen balance for user %',
                insufficient_user_id;
        END IF;

        UPDATE users
           SET balance = users.balance + refund.amount,
               frozen_balance = users.frozen_balance - refund.amount,
               updated_at = NOW()
          FROM (
              SELECT user_id, SUM(amount) AS amount
              FROM legacy_media_video_pending_refunds
              GROUP BY user_id
          ) refund
         WHERE users.id = refund.user_id;
    END IF;
END $$;

-- Remove parent guards first so account and user deletion no longer depends on
-- the retired task table.
DROP TRIGGER IF EXISTS media_video_guard_account_soft_delete ON accounts;
DROP TRIGGER IF EXISTS media_video_guard_account_delete ON accounts;
DROP TRIGGER IF EXISTS media_video_guard_user_soft_delete ON users;
DROP TRIGGER IF EXISTS media_video_guard_user_delete ON users;

-- Dropping the task table removes its admission/archive triggers before their
-- trigger functions are removed below.
DROP TABLE IF EXISTS media_video_tasks CASCADE;

DROP FUNCTION IF EXISTS guard_media_video_parent_deletion();
DROP FUNCTION IF EXISTS guard_media_video_parent_admission();
DROP FUNCTION IF EXISTS reconcile_media_video_submission(TEXT, BIGINT, TEXT, BOOLEAN, TEXT);
DROP FUNCTION IF EXISTS refund_media_video_dispute(TEXT, BIGINT, TEXT);
DROP FUNCTION IF EXISTS archive_media_video_order();

DROP TABLE IF EXISTS media_video_ledger;
DROP TABLE IF EXISTS media_video_order_archive;

ALTER TABLE groups DROP COLUMN IF EXISTS video_price_per_request;
