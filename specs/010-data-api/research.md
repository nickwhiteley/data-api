# Research: Data API (010)

## Existing Prototype

**Decision**: The existing `api/internal/handler/dataexport/extract.go` and `audit.ExtractService` are a prototype reading from `audit_log` with cursor-based pagination. They will be replaced entirely.  
**Rationale**: The spec requires per-table `_log` extraction with window-based cursors tracked in `data_extraction_execution`. The prototype's approach (cursor on `audit_log`) is fundamentally different.  
**Alternatives considered**: Extending the prototype — rejected because the data model (window vs cursor, `_log` tables vs `audit_log`) is incompatible.

---

## Data Engineer Role

**Decision**: `data_engineer` is both an API key scope (controls data API access) and a user role (controls UI nav visibility).  
**Rationale**: Session users access the stats UI via browser (role on user); pipelines call the data API via API key (scope on api_key). Both surfaces need the same identity concept.  
**How to apply**: The existing `api_key.scope` column already carries `data_engineer`. The user's session role must also be `data_engineer` for the nav item check (`$session.user?.role`). These are set independently — tenant admin assigns role when creating the user and scope when creating the key.

---

## Enterprise Tier Gating

**Decision**: Add `data_api_enabled BOOLEAN NOT NULL DEFAULT false` to `wd.platform_usage_tier`. Check at request time by joining tenant → tier.  
**Rationale**: Clean per-tier feature flag. No magic sort_order comparisons. Extensible to future tier-based features.  
**Query**:
```sql
SELECT put.data_api_enabled
FROM wd.tenant t
JOIN wd.platform_usage_tier put USING (platform_usage_tier_id)
WHERE t.tenant_id = $1 AND t.deleted_at IS NULL
```
The migration seeds `data_api_enabled = true` on the Enterprise tier row.  
**Middleware**: `requireEnterpriseTier(ctx, pool, w)` helper in the `dataexport` handler, called after `requireDataEngineer`.

---

## Safety Lag Configuration

**Decision**: Read from platform config via `platformconfig.Service.Get(ctx, "WD_DATA_EXTRACT_SAFETY_LAG_SECONDS")`, default 5.  
**Rationale**: Follows the existing `ApplyPlatformConfig` pattern. Config refreshes every 2 minutes; 5s default is conservative.  
**Seeded by migration**: `INSERT INTO wd.platform_config (key, label, value, is_sensitive) VALUES ('WD_DATA_EXTRACT_SAFETY_LAG_SECONDS', 'Data Extract Safety Lag (seconds)', '5', false)`.

---

## DJSON Output Format

**Decision**: Columnar JSON — `columns` array and `rows` array of arrays, with top-level metadata fields.  
**Rationale**: Efficient encoding for tabular data; column names transmitted once not per-row.  
**Shape**:
```json
{
  "data_extraction_execution_id": "01927...",
  "columns": ["account_log_id", "account_id", "tenant_id", "modified_at", "modified_by"],
  "rows": [
    ["01927...", "01928...", "01929...", "2026-05-14T10:00:00Z", "01930..."],
    ["..."]
  ]
}
```
Timestamps serialised as UTC ISO 8601 strings. NULLs serialised as `null`.

---

## Pagination Strategy

**Decision**: Offset-based pagination (`page_number` + `row_count`) with the extraction window (`start_at`/`end_at`) fixed at page 1 and stored in `data_extraction_execution`. Pages 2+ look up the window from the execution record.  
**Rationale**: Agreed in spec. Simple for callers to implement. Window anchoring prevents drift between pages.  
**Exact-multiple edge case**: When `result_count == row_count`, whether it is the last page is unknown; caller must request the next page (which returns 0 rows) to trigger `completed`.

---

## `clock_timestamp()` Migration

**Decision**: New migration (`045_log_clock_timestamp.sql`) issues `ALTER TABLE ... ALTER COLUMN modified_at SET DEFAULT clock_timestamp()` for all 33 existing `_log` tables.  
**Rationale**: `NOW()` = transaction start time; `clock_timestamp()` ≈ write time, shrinking the gap to milliseconds and making a 5-second safety lag reliable.  
**Impact**: Non-destructive `ALTER`; no data rewrite; no downtime required.

---

## Discovery Implementation

**Decision**: Query `information_schema.tables` for tables in the `wd` schema ending in `_log`, then cross-join with `information_schema.columns` to verify the base table exists (strip `_log` suffix, confirm table present). Exclude `data_extraction_execution` and any tables on the deny list.  
**Description source**: `pg_catalog.obj_description(to_regclass('wd.' || table_name))` — empty string if no comment.  
**Rationale**: Schema-driven; automatically includes new `_log` tables as they are added without code changes.

---

## Partial Unique Constraint for Idempotent Retry

**Decision**: `CREATE UNIQUE INDEX ON wd.data_extraction_execution (user_id, table_name) WHERE status = 'pending'`.  
**Rationale**: Enforces at DB level that at most one `pending` row exists per user+table, making page 1 retry idempotent without application-level locking.

---

## Deny List: Active Constraint

**Decision**: `CREATE UNIQUE INDEX ON wd.data_extraction_deny (table_name) WHERE deleted_at IS NULL`.  
**Rationale**: Prevents duplicate active denies for the same table while allowing re-deny after removal (soft delete preserves history).

---

## Frontend Route for Data Engineers

**Decision**: New route `app/src/routes/(app)/data/` for the stats page. Nav item added to `+layout.svelte` with `['data_engineer', 'tenant_admin', 'platform_admin']` role check.  
**Rationale**: Matches existing pattern in `+layout.svelte`. Data engineers who log in via session (not API key) see the stats page via their `role` field.

---

## Existing Handler Disposition

| File | Action |
|---|---|
| `api/internal/handler/dataexport/extract.go` | Replace entirely |
| `api/internal/domain/audit/audit.go` | Retain (used by audit module); extract service removed |
| `api/tests/integration/data_extract_test.go` | Replace with new integration tests |
