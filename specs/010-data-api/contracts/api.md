# API Contracts: Data API (010)

All endpoints require `Authorization: Bearer <api-key>` with `scope = data_engineer`, except
admin deny-list endpoints which require a session with `role = platform_admin`.

All endpoints return 403 if the tenant is not on the Enterprise tier.  
All `{table}` path parameters use the **base table name** (e.g. `account`, not `account_log`).  
All denied or non-existent tables return **404** (consistent, does not leak schema).

---

## DJSON Response Format

Used by window and current extraction endpoints.

```json
{
  "data_extraction_execution_id": "01927c3a-...",
  "columns": ["account_log_id", "account_id", "tenant_id", "balance", "modified_at", "modified_by"],
  "rows": [
    ["01927c3b-...", "01927c3c-...", "01927c3d-...", "100.00", "2026-05-14T10:00:00Z", "01927c3e-..."],
    ["..."]
  ]
}
```

- `data_extraction_execution_id`: present on every page.
- `columns`: column names from the `_log` or base table; transmitted once per response.
- `rows`: each inner array corresponds to one row; values match `columns` order.
- Timestamps: UTC ISO 8601 strings.
- NULLs: JSON `null`.
- `data_extraction_execution_id` is omitted from the `columns`/`rows` — it is a top-level metadata field.

---

## GET /v1/data/extract

**Discovery** — list available tables.

**Auth**: data engineer API key or session (tenant admin, platform admin).  
**Response**: `200 OK`

```json
{
  "tables": [
    { "table_name": "account", "description": "Financial account records" },
    { "table_name": "transaction", "description": "" },
    { "table_name": "user_profile", "description": "" }
  ]
}
```

- Returns only DB-2 shadow tables whose base table exists in `wd` schema.
- Excluded: denied tables, `data_extraction_execution`.
- `description`: from PostgreSQL table comment on `{table}_log`; empty string if none.

---

## GET /v1/data/extract/{table}

**Window extraction** — incremental changes from `{table}_log`.

**Auth**: data engineer API key.

**Query parameters**:

| Parameter | Type | Description |
|---|---|---|
| `start_at` | string | Override window start. UTC ISO 8601 or Unix epoch seconds. No safety lag applied. |
| `end_at` | string | Override window end. UTC ISO 8601 or Unix epoch seconds. No safety lag applied. |
| `row_count` | integer | Page size. Max 10,000. Must be provided with `page_number`. |
| `page_number` | integer | 1-based page number. Must be provided with `row_count`. |
| `data_extraction_execution_id` | UUID | Required for `page_number > 1`. |

**Computed window** (when no overrides):
- `start_at` = `end_at` of most recent `completed`/`reset` row for this user+table, or `2000-01-01T00:00:00Z` if none.
- `end_at` = `clock_timestamp() - WD_DATA_EXTRACT_SAFETY_LAG_SECONDS`.
- Extract filter: `modified_at >= start_at AND modified_at < end_at`.

**Response**: `200 OK` — DJSON format (see above).

**Errors**:

| Code | Condition |
|---|---|
| 400 | `row_count > 10,000` or `start_at > end_at` |
| 403 | Missing data_engineer scope or not Enterprise tier |
| 404 | Table denied or does not exist |
| 404 | `data_extraction_execution_id` not found or belongs to different user |

**Status transitions**:
- Row inserted as `pending` before first byte sent.
- Updated to `started` after page 1 fully written.
- Updated to `completed` when `result_count < row_count` (last page).
- Non-paginated: `pending` → `started` → `completed` in one response cycle.
- Page 1 retry: existing `pending` row reused (unique constraint); same `data_extraction_execution_id` returned.

---

## GET /v1/data/extract/{table}/current

**Current state extraction** — all live rows from `{table}`.

**Auth**: data engineer API key.

**Rate limit**: 2 per user per calendar day per table (counts `started` + `completed` only).

**Query parameters**:

| Parameter | Type | Description |
|---|---|---|
| `row_count` | integer | Page size. Max 10,000. |
| `page_number` | integer | 1-based. |
| `data_extraction_execution_id` | UUID | Required for `page_number > 1`. |

**Response**: `200 OK` — DJSON format. `start_at` and `end_at` both set to `clock_timestamp()` at request time.

**Errors**:

| Code | Condition |
|---|---|
| 400 | `row_count > 10,000` |
| 403 | Missing data_engineer scope or not Enterprise tier |
| 404 | Table denied or does not exist |
| 429 | Daily limit reached (2 per calendar day per table) |

---

## POST /v1/data/extract/{table}/reset

**Reset cursor** — rewinds the window extraction cursor.

**Auth**: data engineer API key.

**Query parameters**:

| Parameter | Type | Description |
|---|---|---|
| `timestamp` | string | New cursor position. UTC ISO 8601 or Unix epoch seconds. Defaults to `2000-01-01T00:00:00Z`. |

**Response**: `200 OK`

```json
{
  "data_extraction_execution_id": "01927c3a-...",
  "table_name": "account",
  "end_at": "2026-01-01T00:00:00Z"
}
```

Inserts a `data_extraction_execution` row with `status=reset`, `extract_type=window`, `row_count=0`, `execution_time_taken=0`.

**Errors**:

| Code | Condition |
|---|---|
| 403 | Missing data_engineer scope or not Enterprise tier |
| 404 | Table denied or does not exist |

---

## GET /v1/data/executions

**Execution history** — for stats page.

**Auth**: session (data engineer, tenant admin, platform admin).

**Query parameters**:

| Parameter | Type | Description |
|---|---|---|
| `tenant_id` | UUID | Platform admin only. Filter by tenant. |
| `user_id` | UUID | Filter by data engineer. Scoped to caller's tenant unless platform admin. |
| `start_date` | string | Date range filter (inclusive). UTC ISO 8601 date. |
| `end_date` | string | Date range filter (inclusive). UTC ISO 8601 date. |
| `page` | integer | Pagination, 1-based. Default 1. |
| `per_page` | integer | Page size. Default 50. Max 200. |

**Response**: `200 OK`

```json
{
  "executions": [
    {
      "data_extraction_execution_id": "01927c3a-...",
      "table_name": "account",
      "extract_type": "window",
      "status": "completed",
      "start_at": "2026-05-13T10:00:00Z",
      "end_at": "2026-05-14T10:00:00Z",
      "row_count": 142,
      "execution_time_taken": 380,
      "user_id": "01927c3b-...",
      "inserted_at": "2026-05-14T10:00:05Z"
    }
  ],
  "total": 84,
  "page": 1,
  "per_page": 50
}
```

**Scoping rules**:
- Data engineer: sees only their own executions within their tenant.
- Tenant admin: sees all engineers' executions within their tenant.
- Platform admin: can query any tenant via `tenant_id` param.

---

## GET /v1/admin/data/denies

**List deny entries** — platform admin only.

**Auth**: session (`role = platform_admin`).

**Response**: `200 OK`

```json
{
  "denies": [
    {
      "data_extraction_deny_id": "01927c3a-...",
      "table_name": "platform_config_log",
      "inserted_by": "01927c3b-...",
      "inserted_at": "2026-05-14T10:00:00Z"
    }
  ]
}
```

---

## POST /v1/admin/data/denies

**Add deny entry** — platform admin only.

**Auth**: session (`role = platform_admin`).

**Request body**:
```json
{ "table_name": "transaction_log" }
```

**Response**: `201 Created`

```json
{
  "data_extraction_deny_id": "01927c3a-...",
  "table_name": "transaction_log",
  "inserted_at": "2026-05-14T10:00:00Z"
}
```

**Errors**:

| Code | Condition |
|---|---|
| 409 | Table already on active deny list |

---

## DELETE /v1/admin/data/denies/{table}

**Remove deny entry** — platform admin only.

**Auth**: session (`role = platform_admin`).

**Response**: `204 No Content`

**Errors**:

| Code | Condition |
|---|---|
| 404 | Table not on active deny list |
