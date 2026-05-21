-- +goose Up
-- +goose StatementBegin

-- Add data_api_enabled to platform_usage_tier
ALTER TABLE wd.platform_usage_tier
    ADD COLUMN data_api_enabled BOOLEAN NOT NULL DEFAULT false;

-- Enable for Enterprise tier (highest sort_order)
UPDATE wd.platform_usage_tier
    SET data_api_enabled = true
    WHERE sort_order = (SELECT MAX(sort_order) FROM wd.platform_usage_tier WHERE deleted_at IS NULL);

-- data_extraction_execution
CREATE TABLE wd.data_extraction_execution (
    data_extraction_execution_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    tenant_id                     UUID        NOT NULL REFERENCES wd.tenant(tenant_id),
    user_id                       UUID        NOT NULL REFERENCES wd.user(user_id),
    table_name                    TEXT        NOT NULL,
    extract_type                  TEXT        NOT NULL CHECK (extract_type IN ('window', 'current')),
    status                        TEXT        NOT NULL CHECK (status IN ('pending', 'started', 'completed', 'reset')),
    start_at                      TIMESTAMPTZ NOT NULL,
    end_at                        TIMESTAMPTZ NOT NULL,
    row_count                     INTEGER     NOT NULL DEFAULT 0,
    execution_time_taken          INTEGER     NOT NULL DEFAULT 0,
    inserted_at                   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at                    TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_data_extraction_execution_pending
    ON wd.data_extraction_execution (user_id, table_name)
    WHERE status = 'pending';

CREATE INDEX idx_data_extraction_execution_cursor
    ON wd.data_extraction_execution (user_id, table_name, inserted_at DESC)
    WHERE status IN ('completed', 'reset') AND deleted_at IS NULL;

CREATE INDEX idx_data_extraction_execution_stats
    ON wd.data_extraction_execution (tenant_id, user_id, start_at DESC)
    WHERE deleted_at IS NULL;

-- Audit trigger for data_extraction_execution (has tenant_id)
CREATE TRIGGER data_extraction_execution_audit
    AFTER INSERT OR UPDATE OR DELETE ON wd.data_extraction_execution
    FOR EACH ROW EXECUTE FUNCTION wd.audit_trigger();

-- data_extraction_execution_log
CREATE TABLE wd.data_extraction_execution_log (
    data_extraction_execution_log_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    data_extraction_execution_id      UUID        NOT NULL,
    tenant_id                         UUID        NOT NULL,
    user_id                           UUID        NOT NULL,
    table_name                        TEXT        NOT NULL,
    extract_type                      TEXT        NOT NULL,
    status                            TEXT        NOT NULL,
    start_at                          TIMESTAMPTZ NOT NULL,
    end_at                            TIMESTAMPTZ NOT NULL,
    row_count                         INTEGER     NOT NULL,
    execution_time_taken              INTEGER     NOT NULL,
    inserted_at                       TIMESTAMPTZ NOT NULL,
    updated_at                        TIMESTAMPTZ NOT NULL,
    deleted_at                        TIMESTAMPTZ,
    modified_at                       TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    modified_by                       UUID
);

-- data_extraction_deny (platform-wide, no tenant_id, no audit trigger)
CREATE TABLE wd.data_extraction_deny (
    data_extraction_deny_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    table_name               TEXT        NOT NULL,
    inserted_by              UUID        NOT NULL REFERENCES wd.user(user_id),
    inserted_at              TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at               TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_data_extraction_deny_active
    ON wd.data_extraction_deny (table_name)
    WHERE deleted_at IS NULL;

-- data_extraction_deny_log
CREATE TABLE wd.data_extraction_deny_log (
    data_extraction_deny_log_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    data_extraction_deny_id      UUID        NOT NULL,
    table_name                   TEXT        NOT NULL,
    inserted_by                  UUID        NOT NULL,
    inserted_at                  TIMESTAMPTZ NOT NULL,
    deleted_at                   TIMESTAMPTZ,
    modified_at                  TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    modified_by                  UUID
);

-- Seed default deny entries using the platform admin user if one exists.
-- Uses a DO block so migration never fails on fresh DBs without a platform admin.
DO $$
DECLARE v_user_id UUID;
BEGIN
    SELECT uo.user_id INTO v_user_id
    FROM wd.user_org uo
    WHERE uo.role = 'platform_admin' AND uo.deleted_at IS NULL
    LIMIT 1;

    IF v_user_id IS NOT NULL THEN
        INSERT INTO wd.data_extraction_deny (table_name, inserted_by)
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

-- Platform config seed
INSERT INTO wd.platform_config (key, label, value, is_sensitive)
VALUES ('WD_DATA_EXTRACT_SAFETY_LAG_SECONDS', 'Data Extract Safety Lag (seconds)', '5', false)
ON CONFLICT DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM wd.platform_config WHERE key = 'WD_DATA_EXTRACT_SAFETY_LAG_SECONDS';

DROP TABLE IF EXISTS wd.data_extraction_deny_log;
DROP TABLE IF EXISTS wd.data_extraction_deny;
DROP TABLE IF EXISTS wd.data_extraction_execution_log;
DROP TABLE IF EXISTS wd.data_extraction_execution;

ALTER TABLE wd.platform_usage_tier DROP COLUMN IF EXISTS data_api_enabled;

-- +goose StatementEnd
