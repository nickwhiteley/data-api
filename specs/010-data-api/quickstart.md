# Developer Quickstart: Data API (010)

## Prerequisites

- Go 1.26+, running PostgreSQL 18+, `golangci-lint`
- Existing WoodenDollars API compiles and tests pass

## Source Code Layout

```text
api/
├── internal/
│   ├── handler/
│   │   ├── dataexport/
│   │   │   └── extract.go          ← REPLACE (window, current, reset, discovery)
│   │   └── admin/
│   │       └── platform.go         ← EXTEND (deny list endpoints)
│   ├── domain/
│   │   └── dataextract/
│   │       ├── service.go          ← NEW (extraction orchestration)
│   │       ├── deny.go             ← NEW (deny list queries)
│   │       └── execution.go        ← NEW (execution record management)
│   └── db/
│       ├── migrations/
│       │   ├── 044_data_api.sql    ← NEW
│       │   └── 045_log_clock_timestamp.sql ← NEW
│       └── schema/
│           └── current.sql         ← UPDATE (new tables + clock_timestamp alters)
├── tests/
│   └── integration/
│       ├── data_extract_test.go    ← REPLACE
│       └── data_deny_test.go       ← NEW
└── cmd/server/
    └── main.go                     ← EXTEND (register dataexport routes)

app/
└── src/
    └── routes/
        └── (app)/
            ├── +layout.svelte      ← EXTEND (data nav item)
            ├── data/
            │   ├── +page.svelte    ← NEW (stats page)
            │   └── +page.ts        ← NEW (data loader)
            └── admin/
                ├── +page.svelte    ← EXTEND (deny list link)
                └── data-deny/
                    ├── +page.svelte ← NEW
                    └── +page.ts     ← NEW
```

## Implementation Order

1. **Migrations** (`044`, `045`) — unblocks all DB work
2. **Domain layer** (`dataextract/execution.go`, `deny.go`) — DB queries + business logic
3. **Handler rewrite** (`handler/dataexport/extract.go`) — all 4 endpoints
4. **Admin handler** (`handler/admin/platform.go`) — deny list CRUD
5. **Route registration** (`cmd/server/main.go`) — wire everything up
6. **API integration tests** — cover happy path + all error branches
7. **Frontend: stats page** (`app/src/routes/(app)/data/`)
8. **Frontend: deny list page** (`app/src/routes/(app)/admin/data-deny/`)
9. **Frontend: nav + user management edit button**
10. **E2E tests**

## Key Implementation Notes

### Window extraction SQL pattern

```sql
-- Determine window
SELECT COALESCE(
    (SELECT end_at FROM wd.data_extraction_execution
     WHERE user_id = $1 AND table_name = $2
       AND status IN ('completed', 'reset') AND deleted_at IS NULL
     ORDER BY inserted_at DESC LIMIT 1),
    '2000-01-01 00:00:00+00'::timestamptz
) AS start_at,
(clock_timestamp() - ($3 || ' seconds')::interval) AS end_at;

-- Extract (example for account_log)
SELECT * FROM wd.account_log
WHERE tenant_id = $1
  AND modified_at >= $start_at
  AND modified_at < $end_at
ORDER BY modified_at ASC, account_log_id ASC
LIMIT $row_count OFFSET (($page_number - 1) * $row_count);
```

### Idempotent page 1 insert

```go
// Insert or reuse pending row (unique index enforces at most one pending per user+table)
_, err = tx.Exec(ctx, `
    INSERT INTO wd.data_extraction_execution
        (tenant_id, user_id, table_name, extract_type, status, start_at, end_at)
    VALUES ($1, $2, $3, $4, 'pending', $5, $6)
    ON CONFLICT DO NOTHING`,
    tenantID, userID, tableName, extractType, startAt, endAt)
// Then SELECT the pending row (whether just inserted or pre-existing)
```

### Completed detection

```go
if len(rows) < rowCount { // last page (includes len==0)
    _, err = tx.Exec(ctx, `UPDATE wd.data_extraction_execution
        SET status = 'completed', updated_at = clock_timestamp()
        WHERE data_extraction_execution_id = $1`, execID)
}
```

### DJSON serialisation

```go
type DJSONResponse struct {
    ExecutionID string          `json:"data_extraction_execution_id"`
    Columns     []string        `json:"columns"`
    Rows        [][]interface{} `json:"rows"`
}
```

Use `pgx` row descriptions to populate `Columns` dynamically — no hardcoded column lists.

### Tier gate

```go
func (h *ExtractHandler) requireEnterpriseTier(ctx context.Context, pool *pgxpool.Pool, w http.ResponseWriter) bool {
    var enabled bool
    err := pool.QueryRow(ctx, `
        SELECT put.data_api_enabled
        FROM wd.tenant t
        JOIN wd.platform_usage_tier put USING (platform_usage_tier_id)
        WHERE t.tenant_id = $1 AND t.deleted_at IS NULL`,
        middleware.TenantID(ctx),
    ).Scan(&enabled)
    if err != nil || !enabled {
        handler.Error(w, http.StatusForbidden, apierror.New(apierror.CodeForbidden, "data API requires Enterprise tier"))
        return false
    }
    return true
}
```

### Safety lag

```go
lagSecs := config.Current().DataExtractSafetyLagSeconds // loaded from platform_config
endAt := time.Now().UTC().Add(-time.Duration(lagSecs) * time.Second)
```

Add `DataExtractSafetyLagSeconds int` to the `Config` struct and populate it in `ApplyPlatformConfig`.

## Running Tests

```bash
# All tests with race detector
go test -race ./...

# Integration tests only
go test -race -run TestDataExtract ./api/tests/integration/...
go test -race -run TestDataDeny ./api/tests/integration/...
```

## Frontend Nav Item

In `app/src/routes/(app)/+layout.svelte`, add after the existing nav items (both desktop and mobile sections):

```svelte
{#if ['data_engineer', 'tenant_admin', 'platform_admin'].includes($session.user?.role ?? '')}
  <a href="/data">{$_('nav.data')}</a>
{/if}
```

Add translation key `nav.data` to all locale files (`en-GB`, `en-US`, `fr`).
