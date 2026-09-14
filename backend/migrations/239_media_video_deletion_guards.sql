-- Serialize video admission with parent deletion, including soft deletes and
-- bulk/direct SQL paths. No existing balance or pricing data is rewritten.
CREATE INDEX IF NOT EXISTS idx_media_video_tasks_account_lifecycle
    ON media_video_tasks(upstream_account_id, billing_status, expires_at);
CREATE INDEX IF NOT EXISTS idx_media_video_tasks_key_billing
    ON media_video_tasks(api_key_id, billing_status);

CREATE OR REPLACE FUNCTION guard_media_video_parent_deletion() RETURNS trigger AS $$
DECLARE pending_count BIGINT;
BEGIN
    IF TG_OP='UPDATE' AND (OLD.deleted_at IS NOT NULL OR NEW.deleted_at IS NULL) THEN
        RETURN NEW;
    END IF;
    IF TG_TABLE_NAME='accounts' THEN
        SELECT count(*) INTO pending_count FROM media_video_tasks
        WHERE upstream_account_id=OLD.id AND (
            status IN ('creating','queued','running')
            OR billing_status NOT IN ('not_billed','settled','released')
            OR (downloadable AND expires_at>NOW()));
        IF pending_count>0 THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='MEDIA_VIDEO_ACCOUNT_IN_USE', DETAIL=pending_count::TEXT;
        END IF;
    ELSE
        SELECT count(*) INTO pending_count FROM media_video_tasks
        WHERE user_id=OLD.id AND (status IN ('creating','queued','running')
            OR billing_status NOT IN ('not_billed','settled','released'));
        IF pending_count>0 THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='MEDIA_VIDEO_USER_IN_USE', DETAIL=pending_count::TEXT;
        END IF;
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER media_video_guard_account_soft_delete BEFORE UPDATE OF deleted_at ON accounts
    FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();
CREATE TRIGGER media_video_guard_account_delete BEFORE DELETE ON accounts
    FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();
CREATE TRIGGER media_video_guard_user_soft_delete BEFORE UPDATE OF deleted_at ON users
    FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();
CREATE TRIGGER media_video_guard_user_delete BEFORE DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_deletion();

CREATE OR REPLACE FUNCTION guard_media_video_parent_admission() RETURNS trigger AS $$
BEGIN
    IF TG_OP='UPDATE' AND NEW.task_id IS NOT DISTINCT FROM OLD.task_id
       AND NEW.user_id IS NOT DISTINCT FROM OLD.user_id
       AND NEW.api_key_id IS NOT DISTINCT FROM OLD.api_key_id
       AND (NEW.upstream_account_id IS NULL OR NEW.upstream_account_id IS NOT DISTINCT FROM OLD.upstream_account_id) THEN
        RETURN NEW;
    END IF;
    -- A lock alone does not invalidate an older REPEATABLE READ snapshot.
    -- Write a new parent tuple version without changing business values:
    -- stale-snapshot deletion then fails with serialization_failure (40001),
    -- while READ COMMITTED deletion waits and checks the committed video.
    -- Keep the same parent-before-key order used by admission/deletion/billing.
    UPDATE users SET updated_at=updated_at WHERE id=NEW.user_id AND deleted_at IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='MEDIA_VIDEO_PARENT_UNAVAILABLE';
    END IF;
    IF NEW.upstream_account_id IS NOT NULL THEN
        UPDATE accounts SET updated_at=updated_at WHERE id=NEW.upstream_account_id AND deleted_at IS NULL;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='MEDIA_VIDEO_PARENT_UNAVAILABLE';
        END IF;
    END IF;
    -- Existing terminal tasks can be updated when an expired account is purged;
    -- a newly inserted/replaced order must still have a usable owner/key.
    IF TG_OP='INSERT' OR NEW.task_id IS DISTINCT FROM OLD.task_id THEN
        PERFORM id FROM api_keys WHERE id=NEW.api_key_id AND user_id=NEW.user_id AND deleted_at IS NULL FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='MEDIA_VIDEO_PARENT_UNAVAILABLE';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER media_video_guard_parent_admission
    BEFORE INSERT OR UPDATE OF task_id,user_id,api_key_id,upstream_account_id ON media_video_tasks
    FOR EACH ROW EXECUTE FUNCTION guard_media_video_parent_admission();
