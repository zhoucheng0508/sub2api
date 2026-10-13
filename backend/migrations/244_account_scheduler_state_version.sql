-- Independent from monitoring configuration generation: establishing or
-- extending a pause must fence snapshots read before that pause as well.
-- Version every account row update since the cache publishes the entire row.
CREATE OR REPLACE FUNCTION account_scheduler_state_version() RETURNS trigger AS $$
DECLARE
    previous_version bigint := 0;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        previous_version := COALESCE((OLD.credentials->>'scheduler_state_version')::bigint, 0);
    END IF;
    -- Decimal text preserves bigint precision through JSON and Redis Lua.
    NEW.credentials := jsonb_set(COALESCE(NEW.credentials, '{}'::jsonb),
        '{scheduler_state_version}', to_jsonb((previous_version + 1)::text), true);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS accounts_scheduler_state_version ON accounts;
CREATE TRIGGER accounts_scheduler_state_version
BEFORE INSERT OR UPDATE ON accounts
FOR EACH ROW EXECUTE FUNCTION account_scheduler_state_version();
