-- data_extraction_execution
-- Host application must set search_path to the target schema before executing,
-- or schema-qualify all references. No schema prefix is used here.

CREATE TABLE data_extraction_execution (
    data_extraction_execution_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    tenant_id                     UUID        NOT NULL,
    user_id                       UUID        NOT NULL,
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
    ON data_extraction_execution (user_id, table_name)
    WHERE status = 'pending';

CREATE INDEX idx_data_extraction_execution_cursor
    ON data_extraction_execution (user_id, table_name, inserted_at DESC)
    WHERE status IN ('completed', 'reset') AND deleted_at IS NULL;

CREATE INDEX idx_data_extraction_execution_stats
    ON data_extraction_execution (tenant_id, user_id, start_at DESC)
    WHERE deleted_at IS NULL;

-- data_extraction_execution_log
CREATE TABLE data_extraction_execution_log (
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

-- data_extraction_deny (platform-wide, no tenant_id)
CREATE TABLE data_extraction_deny (
    data_extraction_deny_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    table_name               TEXT        NOT NULL,
    inserted_by              UUID        NOT NULL,
    inserted_at              TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at               TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_data_extraction_deny_active
    ON data_extraction_deny (table_name)
    WHERE deleted_at IS NULL;

-- data_extraction_deny_log
CREATE TABLE data_extraction_deny_log (
    data_extraction_deny_log_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    data_extraction_deny_id      UUID        NOT NULL,
    table_name                   TEXT        NOT NULL,
    inserted_by                  UUID        NOT NULL,
    inserted_at                  TIMESTAMPTZ NOT NULL,
    deleted_at                   TIMESTAMPTZ,
    modified_at                  TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    modified_by                  UUID
);
