# Data Model: Data API (010)

## New Tables

### `wd.data_extraction_execution`

Tracks every extraction job — window, current, or reset dummy.

```sql
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
    execution_time_taken          INTEGER     NOT NULL DEFAULT 0,  -- milliseconds
    inserted_at                   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at                    TIMESTAMPTZ
);

-- Only one pending row allowed per user+table (idempotent page 1 retry)
CREATE UNIQUE INDEX uq_data_extraction_execution_pending
    ON wd.data_extraction_execution (user_id, table_name)
    WHERE status = 'pending';

-- Cursor query: most recent completed/reset row for user+table
CREATE INDEX idx_data_extraction_execution_cursor
    ON wd.data_extraction_execution (user_id, table_name, inserted_at DESC)
    WHERE status IN ('completed', 'reset') AND deleted_at IS NULL;

-- Stats page query: filter by tenant, engineer, date range
CREATE INDEX idx_data_extraction_execution_stats
    ON wd.data_extraction_execution (tenant_id, user_id, start_at DESC)
    WHERE deleted_at IS NULL;
```

**Status lifecycle**:

```
pending  ──► started ──► completed
   │
   └──► (disconnect: stays pending; retry reuses row via unique index)

started ──► completed  (last page: result_count < row_count)
         └──► (disconnect on page 2+: stays started; cursor not advanced)

(reset row inserted directly with status='reset', extract_type='window')
```

**Cursor logic**: `SELECT end_at FROM wd.data_extraction_execution WHERE user_id=$1 AND table_name=$2 AND status IN ('completed','reset') AND deleted_at IS NULL ORDER BY inserted_at DESC LIMIT 1`. Returns `2000-01-01 00:00:00+00` if no row.

**Daily limit query** (`/current`): `SELECT COUNT(*) FROM wd.data_extraction_execution WHERE user_id=$1 AND table_name=$2 AND extract_type='current' AND status IN ('started','completed') AND start_at::date = CURRENT_DATE AND deleted_at IS NULL`.

---

### `wd.data_extraction_execution_log`

Shadow log per DB-2. Full row copy on every UPDATE.

```sql
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
```

---

### `wd.data_extraction_deny`

Platform-wide deny list. No `tenant_id` — applies to all tenants.

```sql
CREATE TABLE wd.data_extraction_deny (
    data_extraction_deny_id  UUID        PRIMARY KEY DEFAULT uuidv7(),
    table_name               TEXT        NOT NULL,
    inserted_by              UUID        NOT NULL REFERENCES wd.user(user_id),
    inserted_at              TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at               TIMESTAMPTZ
);

-- Only one active deny per table
CREATE UNIQUE INDEX uq_data_extraction_deny_active
    ON wd.data_extraction_deny (table_name)
    WHERE deleted_at IS NULL;
```

**Default seeded entries** (inserted by migration, `inserted_by` = system/platform admin user):
- `platform_config_log`
- `user_credential_log`
- `session_log`
- `data_extraction_execution_log`
- `data_extraction_deny_log`

---

### `wd.data_extraction_deny_log`

Shadow log per DB-2.

```sql
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
```

---

## Altered Tables

### `wd.platform_usage_tier`

Add feature flag for Data API access.

```sql
ALTER TABLE wd.platform_usage_tier
    ADD COLUMN data_api_enabled BOOLEAN NOT NULL DEFAULT false;

-- Set true for the Enterprise tier
UPDATE wd.platform_usage_tier
    SET data_api_enabled = true
    WHERE sort_order = (SELECT MAX(sort_order) FROM wd.platform_usage_tier WHERE deleted_at IS NULL);
```

---

## Modified Defaults: All `_log` Tables

All 33 existing `_log` tables have `modified_at` changed from `DEFAULT NOW()` to `DEFAULT clock_timestamp()`. Full list:

```sql
ALTER TABLE wd.platform_usage_tier_log          ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.tenant_log                        ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.tenant_auth_method_log            ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.tenant_registration_log           ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.organisation_log                  ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.user_log                          ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.user_profile_log                  ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.user_credential_log               ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.user_org_log                      ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.account_log                       ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.transaction_log                   ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.escrow_confirmation_log           ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.api_key_log                       ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.notification_log                  ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.help_content_log                  ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.organisation_join_request_log     ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.escrow_log                        ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.escrow_action_log                 ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.account_shortlist_log             ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.session_log                       ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.staff_cost_log                    ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.non_person_cost_log               ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.cost_schedule_log                 ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.cost_accrual_log                  ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.cost_pool_log                     ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.ongoing_transfer_request_log      ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.ongoing_transfer_deduction_log    ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.cost_schedule_run_log             ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.platform_config_log               ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.feedback_log                      ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.error_event_log                   ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.performance_metric_log            ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
ALTER TABLE wd.platform_stats_bucket_log         ALTER COLUMN modified_at SET DEFAULT clock_timestamp();
```

---

## Platform Config Seed

```sql
INSERT INTO wd.platform_config (key, label, value, is_sensitive)
VALUES ('WD_DATA_EXTRACT_SAFETY_LAG_SECONDS', 'Data Extract Safety Lag (seconds)', '5', false)
ON CONFLICT (key) DO NOTHING;
```

---

## Migration Files

| File | Purpose |
|---|---|
| `044_data_api.sql` | New tables, indexes, `data_api_enabled` column, default deny rows, platform config seed |
| `045_log_clock_timestamp.sql` | `ALTER` all 33 `_log` tables to `clock_timestamp()` |
