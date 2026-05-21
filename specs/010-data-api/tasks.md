# Tasks: Data API (010)

**Input**: Design documents from `specs/010-data-api/`  
**Branch**: `010-data-api`  
**Tests**: Mandatory per constitution Principle V — integration tests for every API path, e2e for every UI path, happy and unhappy paths.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Parallelisable (different files, no shared state dependency)
- **[Story]**: User story label from spec.md

---

## Phase 1: Setup

**Purpose**: Confirm baseline, create new package skeleton.

- [x] T001 Verify existing tests pass and linter is clean: `go test -race ./... && golangci-lint run`
- [x] T002 Create package skeleton: `api/internal/domain/dataextract/` with empty `service.go`, `execution.go`, `deny.go`, `tier.go`, `djson.go` stub files (package declaration only)

**Checkpoint**: Codebase compiles with new empty package.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Database schema, domain layer foundations, and cross-cutting helpers that every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 [P] Write `api/internal/db/migrations/044_data_api.sql`: create `wd.data_extraction_execution`, `wd.data_extraction_execution_log`, `wd.data_extraction_deny`, `wd.data_extraction_deny_log` tables with all indexes and constraints per `data-model.md`; add `data_api_enabled BOOLEAN NOT NULL DEFAULT false` to `wd.platform_usage_tier`; set `data_api_enabled = true` on Enterprise tier; seed default deny rows (`platform_config_log`, `user_credential_log`, `session_log`, `data_extraction_execution_log`, `data_extraction_deny_log`); seed `WD_DATA_EXTRACT_SAFETY_LAG_SECONDS` platform config entry; add INSERT/UPDATE/DELETE audit triggers on `wd.data_extraction_execution` and `wd.data_extraction_deny` using the existing `wd.audit_trigger()` function (DB-3)
- [x] T004 [P] Write `api/internal/db/migrations/045_log_clock_timestamp.sql`: `ALTER TABLE ... ALTER COLUMN modified_at SET DEFAULT clock_timestamp()` for all 33 existing `_log` tables as listed in `data-model.md`
- [x] T005 Update `api/internal/db/schema/current.sql`: add new tables, indexes, `data_api_enabled` column, and `clock_timestamp()` defaults for all 33 `_log` tables to match migrations T003 and T004
- [x] T006 Implement execution record management in `api/internal/domain/dataextract/execution.go`: insert pending row (with `ON CONFLICT DO NOTHING` for idempotent page 1 retry), transition to `started`, transition to `completed`, insert reset row, cursor lookup (most recent `completed`/`reset` `end_at` for user+table, defaulting to `2000-01-01`), `/current` daily count query
- [x] T007 [P] Implement deny list queries in `api/internal/domain/dataextract/deny.go`: `IsDenied(ctx, tableName)`, `ListDenied(ctx)`, `AddDeny(ctx, tableName, insertedBy)`, `RemoveDeny(ctx, tableName)` — all with soft-delete semantics
- [x] T008 [P] Implement tier gate in `api/internal/domain/dataextract/tier.go`: `IsDataAPIEnabled(ctx, pool, tenantID)` — joins `wd.tenant` → `wd.platform_usage_tier` on `data_api_enabled`
- [x] T009 Add `DataExtractSafetyLagSeconds int` to `Config` struct in `api/internal/config/config.go` and populate it in `ApplyPlatformConfig` from `WD_DATA_EXTRACT_SAFETY_LAG_SECONDS` with default `5`
- [x] T010 [P] Implement DJSON serialisation in `api/internal/domain/dataextract/djson.go`: `Serialise(rows pgx.Rows, execID string) (DJSONResponse, error)` — uses pgx `FieldDescriptions()` for dynamic column names; NULLs as JSON null; timestamps as UTC ISO 8601
- [x] T011 Ensure `data_engineer` is a fully valid user role: verify it is accepted wherever roles are validated (role check constants, `getMe()` response, any role enum in the user/user_org domain); update relevant files in `api/internal/`
- [x] T012 Remove prototype `ExtractService` and `Cursor` types from `api/internal/domain/audit/audit.go` (no longer used; existing audit module tests must still pass)
- [x] T013 [P] Write integration test for migrations in `api/tests/integration/db_migration_test.go`: verify `data_extraction_execution`, `data_extraction_deny` tables exist; verify partial unique index on `(user_id, table_name) WHERE status='pending'`; verify default deny rows are present; verify `data_api_enabled` column exists on `platform_usage_tier`; verify `clock_timestamp()` default on a sample `_log` table `modified_at`

**Checkpoint**: Migrations run, schema correct, domain helpers compile, `go test -race ./...` passes.

---

## Phase 3: User Story 7 — Tenant Admin API Key Management (Priority: P1)

**Goal**: Tenant admin can provision a data engineer by creating a user and an API key with `data_engineer` scope via the UI.

**Independent Test**: Tenant admin opens user management, clicks edit on a user, creates an API key with expiry date and `data_engineer` scope; that key successfully authenticates a data API request.

### Tests

- [x] T014 [P] [US7] Write integration tests in `api/tests/integration/api_key_management_test.go`: happy path (create key with data_engineer scope, key authenticates), revoke (deleted_at set), expired key rejected, key without data_engineer scope returns 403 on data API endpoint
- [x] T015 [P] [US7] Write e2e test in `app/tests/e2e/api-key-management.spec.ts`: tenant admin opens user row edit modal, creates API key with expiry, verifies key visible in list, revokes key

### Implementation

- [x] T016 [P] [US7] Extend API key create endpoint to accept `scope` field: find existing endpoint in `api/internal/handler/` (search for api_key INSERT), add `scope` to request body and DB insert; validate scope is a known value
- [x] T017 [P] [US7] Add `GET /v1/admin/user/{id}/api-keys` endpoint in `api/internal/handler/admin/`: list API keys for a user (key ID, name, scope, created_at, expires_at, masked key); scoped to caller's tenant
- [x] T018 [US7] Add `DELETE /v1/admin/api-key/{id}` endpoint in `api/internal/handler/admin/`: soft-delete the key; 404 if not found or not in caller's tenant
- [x] T019 [US7] Register new API key routes in `api/cmd/server/main.go`
- [x] T020 [P] [US7] Frontend: add Edit button per user row in the user management table at `app/src/routes/(app)/admin/+page.svelte` (or `team/+page.svelte` — find the correct user list page); button opens an edit panel/modal
- [x] T021 [US7] Frontend: build API key management panel in `app/src/routes/(app)/admin/` (new component `UserEditPanel.svelte` or inline modal): list existing keys, create new key form (name, scope dropdown, expiry date), revoke button per key
- [x] T022 [US7] Add i18n keys for API key management UI in all locale files (`app/src/lib/i18n/locales/en-GB.json`, `en-US.json`, `fr.json`)

**Checkpoint**: Tenant admin can provision a data engineer end-to-end without accessing the database.

---

## Phase 4: User Story 1 — Window Data Extraction (Priority: P1) 🎯 MVP

**Goal**: Data engineer can incrementally extract changes from any `_log` table. Cursor advances automatically. Exactly-once delivery enforced.

**Independent Test**: `GET /v1/data/extract/account` with a valid data-engineer API key returns account_log rows in DJSON format; second call returns only rows changed since the first call's `end_at`.

### Tests

- [x] T023 [P] [US1] Write integration tests in `api/tests/integration/data_extract_test.go` (replaces prototype): seed tenant + data_engineer API key + account_log rows; happy path (cursor advances on completed); pagination (pages 1, 2, last); exact-multiple edge case (cursor advances only on zero-row final page); idempotent page 1 retry (same execution ID returned); two users extract same table independently; manual `start_at`/`end_at` overrides; `row_count > 10000` returns 400; non-existent table returns 404; denied table returns 404; non-Enterprise tenant returns 403; missing data_engineer scope returns 403; invalid `data_extraction_execution_id` on page 2 returns 404; disconnect simulation (status stays `pending` when response not completed)

### Implementation

- [x] T024 [P] [US1] Implement window extraction SQL in `api/internal/domain/dataextract/service.go`: `ExtractWindow(ctx, input ExtractWindowInput) (ExtractResult, error)` — builds dynamic query against `wd.{table}_log`, applies `modified_at >= start_at AND modified_at < end_at` and `tenant_id = $n`, orders by `modified_at ASC, {table}_log_id ASC`, offsets by `(page_number-1) * row_count`; validates table exists via `information_schema` before querying
- [x] T025 [US1] Rewrite `api/internal/handler/dataexport/extract.go`: `GET /v1/data/extract/{table}` — call `requireDataEngineer`, `requireEnterpriseTier`, `isDenied`; validate `row_count ≤ 10000`; on page 1 resolve window and insert/reuse pending execution row; on page 2+ look up execution record by `data_extraction_execution_id`; call `service.ExtractWindow`; serialise to DJSON; transition status (`started` after page 1 sent, `completed` on last page); write `data_extraction_execution_log` row on every status change
- [x] T026 [US1] Register `GET /v1/data/extract/{table}` route under `/v1/data` group in `api/cmd/server/main.go`; ensure `AuthMiddleware` applied
- [x] T027 [US1] Confirm all integration tests in `api/tests/integration/data_extract_test.go` pass: `go test -race -run TestDataExtract ./api/tests/integration/...`

**Checkpoint**: Window extraction is fully functional. Cursor advances. Pagination correct. All error branches return correct codes.

---

## Phase 5: User Story 5 — Platform Admin Deny List Management (Priority: P2)

**Goal**: Platform admins can add and remove tables from a platform-wide deny list. Denied tables return 404 on extract, reset, and discovery.

**Independent Test**: Platform admin adds `transaction` to deny list via UI; data engineer immediately receives 404 on `GET /v1/data/extract/transaction`.

### Tests

- [x] T028 [P] [US5] Write integration tests in `api/tests/integration/data_deny_test.go`: list deny entries; add deny; duplicate active deny returns 409; extract denied table returns 404; in-progress extraction completes after table is denied; remove deny; extract succeeds after removal; platform admin auth required (non-admin returns 403); `audit_log` not in default deny list but `session_log` is
- [x] T029 [P] [US5] Write e2e test in `app/tests/e2e/data-deny.spec.ts`: platform admin navigates to Admin → Data Deny, adds a table, verifies it appears in list, removes it

### Implementation

- [x] T030 [P] [US5] Add deny list endpoints in `api/internal/handler/admin/platform.go`: `GET /v1/admin/data/denies` (list), `POST /v1/admin/data/denies` (add, 409 on duplicate), `DELETE /v1/admin/data/denies/{table}` (soft-delete, 404 if not active); all require `platform_admin` role check
- [x] T031 [US5] Register deny list routes under `/v1/admin/data/denies` in `api/cmd/server/main.go`
- [x] T032 [P] [US5] Frontend: create `app/src/routes/(app)/admin/data-deny/+page.ts` and `+page.svelte`: table listing active deny entries (table name, added by, added at), add form (table name input + submit), remove button per row with confirmation
- [x] T033 [US5] Frontend: add "Data Deny List" link to the admin page index at `app/src/routes/(app)/admin/+page.svelte`
- [x] T034 [US5] Add i18n keys for deny list UI in `app/src/lib/i18n/locales/en-GB.json`, `en-US.json`, `fr.json`

**Checkpoint**: Platform admins can manage the deny list. Denied tables return 404 across all data API endpoints.

---

## Phase 6: User Story 2 — Current State Extraction (Priority: P2)

**Goal**: Data engineer can extract all live rows from any non-denied table (max 2 per calendar day per table).

**Independent Test**: Two successful `GET /v1/data/extract/account/current` calls succeed; third call in same calendar day returns 429.

### Tests

- [x] T035 [P] [US2] Write integration tests in `api/tests/integration/data_extract_current_test.go`: happy path returns all non-deleted rows; pagination works (same as window); daily limit (2nd call succeeds, 3rd returns 429); `pending` rows do not count toward limit; denied table returns 404; non-Enterprise returns 403; `row_count > 10000` returns 400

### Implementation

- [x] T036 [P] [US2] Implement current extraction SQL in `api/internal/domain/dataextract/service.go`: `ExtractCurrent(ctx, input ExtractCurrentInput) (ExtractResult, error)` — queries base table `wd.{table}` with `deleted_at IS NULL AND tenant_id = $n`; offset pagination
- [x] T037 [P] [US2] Implement daily limit check in `api/internal/domain/dataextract/execution.go`: `CurrentExtractionCount(ctx, userID, tableName string) (int, error)` — counts `status IN ('started','completed') AND extract_type='current' AND start_at::date = CURRENT_DATE AND deleted_at IS NULL`
- [x] T038 [US2] Add `GET /v1/data/extract/{table}/current` handler in `api/internal/handler/dataexport/extract.go`: same auth + tier + deny checks; check daily limit (429 if ≥ 2); insert pending execution (start_at = end_at = clock_timestamp()); call `service.ExtractCurrent`; serialise to DJSON; status transitions identical to window endpoint
- [x] T039 [US2] Register `GET /v1/data/extract/{table}/current` route in `api/cmd/server/main.go`

**Checkpoint**: Current extraction works with rate limiting. Both window and current endpoints are live.

---

## Phase 7: User Story 3 — Discovery (Priority: P3)

**Goal**: Data engineers and tenant admins can list available tables via `GET /v1/data/extract`.

**Independent Test**: `GET /v1/data/extract` returns a list of tables excluding denied and `data_extraction_execution`; denied tables do not appear; descriptions are empty strings when no comment exists.

### Tests

- [x] T040 [P] [US3] Write integration tests in `api/tests/integration/data_discovery_test.go`: returns available DB-2 shadow tables; excludes denied tables; excludes `data_extraction_execution`; description is empty string for tables without comments; tenant admin can call it; platform admin can call it; unauthenticated returns 401

### Implementation

- [x] T041 [P] [US3] Implement discovery query in `api/internal/domain/dataextract/service.go`: `DiscoverTables(ctx, pool) ([]TableInfo, error)` — queries `information_schema.tables` for `wd` schema tables ending in `_log` where base table exists; joins with `pg_catalog.obj_description` for comment; excludes denied tables and `data_extraction_execution_log`
- [x] T042 [US3] Add `GET /v1/data/extract` handler in `api/internal/handler/dataexport/extract.go`: auth required (data engineer, tenant admin, or platform admin); no tier check needed for discovery; call `service.DiscoverTables`; return JSON `{"tables": [{"table_name": "...", "description": "..."}]}`
- [x] T043 [US3] Register `GET /v1/data/extract` route in `api/cmd/server/main.go` (ensure it does not conflict with `GET /v1/data/extract/{table}`)

**Checkpoint**: Discovery returns correct table list. Denied tables excluded.

---

## Phase 8: User Story 4 — Extraction Reset (Priority: P3)

**Goal**: Data engineer can rewind their window cursor to a specific timestamp or to `2000-01-01`.

**Independent Test**: `POST /v1/data/extract/account/reset?timestamp=2026-01-01T00:00:00Z` followed by `GET /v1/data/extract/account` returns rows from `2026-01-01` onwards.

### Tests

- [x] T044 [P] [US4] Write integration tests in `api/tests/integration/data_reset_test.go`: reset with timestamp sets cursor to that value; reset without timestamp sets cursor to `2000-01-01`; subsequent window extraction uses reset cursor; denied table returns 404; non-Enterprise returns 403; missing data_engineer scope returns 403; reset row has `status='reset'`, `extract_type='window'`, `row_count=0`

### Implementation

- [x] T045 [US4] Add `POST /v1/data/extract/{table}/reset` handler in `api/internal/handler/dataexport/extract.go`: parse optional `timestamp` query param (ISO 8601 or Unix epoch; default `2000-01-01T00:00:00Z`); call `execution.InsertResetRow`; return `{"data_extraction_execution_id": "...", "table_name": "...", "end_at": "..."}`
- [x] T046 [US4] Register `POST /v1/data/extract/{table}/reset` route in `api/cmd/server/main.go`

**Checkpoint**: Reset correctly rewinds cursor. Subsequent window extraction starts from the reset point.

---

## Phase 9: User Story 6 — Extraction Stats (Priority: P3)

**Goal**: Data engineers, tenant admins, and platform admins can view extraction history with date range filter and engineer selector.

**Independent Test**: Tenant admin opens the data extract management page, selects a data engineer, applies a date filter, and sees a paginated table of extractions.

### Tests

- [x] T047 [P] [US6] Write integration tests in `api/tests/integration/data_executions_test.go`: data engineer sees only own executions; tenant admin sees all engineers in tenant; platform admin can query any tenant via `tenant_id` param; date range filter works; cross-tenant isolation (tenant admin cannot query another tenant's data); unauthenticated returns 401; data engineer cannot query another tenant
- [x] T048 [P] [US6] Write e2e test in `app/tests/e2e/data-stats.spec.ts`: data engineer logs in, navigates to Data page via nav item, selects engineer from dropdown, applies date range filter, sees extraction rows; tenant admin sees engineer selector with multiple engineers

### Implementation

- [x] T049 [P] [US6] Add `GET /v1/data/executions` endpoint in `api/internal/handler/dataexport/extract.go`: accept `tenant_id` (platform admin only), `user_id`, `start_date`, `end_date`, `page`, `per_page` params; enforce tenant scoping; return paginated `data_extraction_execution` rows (excluding reset rows from display or marking them clearly); require session auth (not API key)
- [x] T050 [US6] Register `GET /v1/data/executions` route in `api/cmd/server/main.go`
- [x] T051 [P] [US6] Frontend: create `app/src/routes/(app)/data/+page.ts`: load initial executions + engineer list via `GET /v1/data/executions` and user list endpoint; pass to page component
- [x] T052 [P] [US6] Frontend: create `app/src/routes/(app)/data/+page.svelte`: stats table (table name, extract type, start_at, end_at, row count, execution time ms, status); date range filter inputs; engineer selector (all engineers in tenant); platform admin: tenant selector above engineer selector; pagination controls
- [x] T053 [US6] Frontend: add Data nav item to `app/src/routes/(app)/+layout.svelte` (both desktop and mobile nav sections): `{#if ['data_engineer', 'tenant_admin', 'platform_admin'].includes($session.user?.role ?? '')}`; link to `/data`
- [x] T054 [US6] Add i18n key `nav.data` and all data stats page keys in `app/src/lib/i18n/locales/en-GB.json`, `en-US.json`, `fr.json`

**Checkpoint**: Stats page visible to correct roles. Data engineers see their own history. Tenant admins see all engineers in tenant. Platform admins see across tenants.

---

## Phase 10: Polish & Cross-Cutting

**Purpose**: Final validation, documentation inline with spec, linter pass.

- [x] T055 [P] Document DJSON format inline in `specs/010-data-api/contracts/api.md` (already done) and add a brief comment block at the top of `api/internal/domain/dataextract/djson.go` referencing the format
- [x] T056 [P] Add `WD_DATA_EXTRACT_SAFETY_LAG_SECONDS` entry to any developer setup documentation or `.env.example` if one exists
- [x] T057 Run full lint and vet pass: `golangci-lint run ./...` and `go vet ./...`; fix any issues
- [x] T058 Run full test suite with race detector: `go test -race ./...`; confirm all new and existing tests pass
- [x] T059 Verify `specs/010-data-api/checklists/requirements.md` — mark all items complete; confirm AF-8 round-trip (API registered → frontend calls it → response rendered → stores updated → background jobs wired)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — **BLOCKS all user stories**
- **Phase 3 (US7)**: Depends on Phase 2
- **Phase 4 (US1)**: Depends on Phase 2
- **Phase 5 (US5)**: Depends on Phase 2; deny list check used by US1 (T025) — run after Phase 4 to avoid partial feature
- **Phase 6 (US2)**: Depends on Phase 2 and Phase 5 (deny check must be live)
- **Phase 7 (US3)**: Depends on Phase 2 and Phase 5
- **Phase 8 (US4)**: Depends on Phase 4 (uses same execution record machinery)
- **Phase 9 (US6)**: Depends on Phase 4; frontend nav depends on Phase 3 (data_engineer role in session)
- **Phase 10 (Polish)**: Depends on all user story phases

### User Story Dependencies

- **US7 (P1)**: Independent after Phase 2
- **US1 (P1)**: Independent after Phase 2
- **US5 (P2)**: Independent after Phase 2; strengthens US1 (deny enforcement)
- **US2 (P2)**: Independent after Phase 2 + Phase 5
- **US3 (P3)**: Independent after Phase 2 + Phase 5
- **US4 (P3)**: Independent after Phase 4 (shares execution package)
- **US6 (P3)**: Independent after Phase 4 + Phase 3

### Within Each Phase

- Tests MUST be written before implementation tasks within each story
- Domain layer (execution.go, service.go) before handler layer
- Handler before route registration
- Backend route before frontend consumer

---

## Parallel Opportunities

### Phase 2 (Foundational)

```
T003 (migration 044)       — parallel
T004 (migration 045)       — parallel
T007 (deny.go queries)     — parallel
T008 (tier.go helper)      — parallel
T009 (config struct)       — parallel
T010 (djson.go)            — parallel
T011 (data_engineer role)  — parallel
T012 (remove prototype)    — parallel
T013 (migration test)      — parallel
─── then ───
T005 (current.sql update)  — after T003+T004
T006 (execution.go)        — can start alongside
```

### Phase 4 (US1) — once Phase 2 complete

```
T023 (integration tests)   — parallel with T024
T024 (service.go)          — parallel with T023
─── then ───
T025 (handler rewrite)     — after T023+T024
T026 (route registration)  — after T025
T027 (test confirmation)   — after T026
```

### Phases 3, 4, 5 — all unblock after Phase 2

```
Phase 3 (US7) ─┐
Phase 4 (US1) ─┤─ can run in parallel by different team members
Phase 5 (US5) ─┘
```

---

## Implementation Strategy

### MVP (Phase 1 + 2 + 4 only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 4: Window Extraction
4. **VALIDATE**: `GET /v1/data/extract/{table}` works end-to-end with a seeded API key
5. Demo / ship MVP

### Incremental Delivery

1. MVP (above) — core extraction working
2. + Phase 3 (US7) → tenant admins can provision data engineers via UI
3. + Phase 5 (US5) → platform admins can manage deny list
4. + Phase 6 (US2) → current state snapshots available
5. + Phase 7 (US3) + Phase 8 (US4) → discovery and reset available
6. + Phase 9 (US6) → stats dashboard visible
7. + Phase 10 → Polish and final validation

---

## Notes

- `[P]` tasks touch different files and have no dependency on incomplete sibling tasks
- Each phase checkpoint must pass `go test -race ./...` before the next phase begins
- Commit after each completed task or logical group (prefix: `010:`)
- The existing `data_extract_test.go` prototype is **replaced** by T023 — do not extend it
- Table name in all DB queries is `{tableName}_log` — always append `_log` suffix, never trust caller-supplied suffix
- Deny check and tier check are cheap DB reads — do them on every request, not cached
- DJSON `columns` come from `pgx.FieldDescriptions()` — never hardcode column lists
