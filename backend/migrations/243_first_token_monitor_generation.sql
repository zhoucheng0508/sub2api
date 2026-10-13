-- The database owns this fencing generation. Covers single/bulk/import updates
-- and recovery even when no request observes a disabled configuration.
CREATE OR REPLACE FUNCTION account_first_token_monitor_generation() RETURNS trigger AS $$
DECLARE
    previous_generation bigint := 0;
    changed boolean := false;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        previous_generation := COALESCE((OLD.credentials->>'first_token_monitor_generation')::bigint, 0);
        changed := jsonb_build_array(NEW.credentials->'first_token_monitor_enabled', NEW.credentials->'first_token_monitor_threshold_seconds', NEW.credentials->'first_token_monitor_anomaly_count', NEW.credentials->'first_token_monitor_cooldown_minutes')
            IS DISTINCT FROM jsonb_build_array(OLD.credentials->'first_token_monitor_enabled', OLD.credentials->'first_token_monitor_threshold_seconds', OLD.credentials->'first_token_monitor_anomaly_count', OLD.credentials->'first_token_monitor_cooldown_minutes')
            OR NEW.credentials->>'first_token_monitor_revision' IS DISTINCT FROM OLD.credentials->>'first_token_monitor_revision'
            OR (OLD.temp_unschedulable_until IS NOT NULL AND NEW.temp_unschedulable_until IS NULL);
    ELSE
        changed := COALESCE(NEW.credentials, '{}'::jsonb) ? 'first_token_monitor_enabled';
    END IF;
    IF changed THEN previous_generation := previous_generation + 1; END IF;
    IF previous_generation > 0 THEN
        NEW.credentials := jsonb_set(COALESCE(NEW.credentials, '{}'::jsonb), '{first_token_monitor_generation}', to_jsonb(previous_generation), true);
    ELSE
        NEW.credentials := COALESCE(NEW.credentials, '{}'::jsonb) - 'first_token_monitor_generation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS accounts_first_token_monitor_generation ON accounts;
CREATE TRIGGER accounts_first_token_monitor_generation
BEFORE INSERT OR UPDATE OF credentials, temp_unschedulable_until ON accounts
FOR EACH ROW EXECUTE FUNCTION account_first_token_monitor_generation();
