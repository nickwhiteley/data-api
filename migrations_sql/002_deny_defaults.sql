-- Optional seed: populate default deny entries using the platform admin user.
-- This is a no-op when no platform admin user exists in the host application.
-- Host application must set search_path to the target schema before executing.

DO $$
DECLARE v_user_id UUID;
BEGIN
    -- Attempt to find a platform admin user via the host app's user_org table.
    -- If the table does not exist or no admin is found, the block exits cleanly.
    BEGIN
        EXECUTE format(
            'SELECT user_id FROM %I.user_org WHERE role = $1 AND deleted_at IS NULL LIMIT 1',
            current_schema()
        ) INTO v_user_id USING 'platform_admin';
    EXCEPTION WHEN undefined_table THEN
        -- user_org table does not exist in this schema; skip seed.
        RETURN;
    END;

    IF v_user_id IS NOT NULL THEN
        INSERT INTO data_extraction_deny (table_name, inserted_by)
        SELECT t.table_name, v_user_id
        FROM (VALUES
            ('platform_config_log'),
            ('user_credential_log'),
            ('session_log'),
            ('data_extraction_execution_log'),
            ('data_extraction_deny_log')
        ) AS t(table_name)
        ON CONFLICT DO NOTHING;
    END IF;
END $$;
