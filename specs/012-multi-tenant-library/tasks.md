# Tasks: Multi-Tenant Library

**Input**: Design documents from `/specs/012-multi-tenant-library/`
**Prerequisites**: plan.md ✓, spec.md ✓

**Organization**: 4 phases. Phases 1 and 2 are foundational and must complete before Phases 3 and 4.

## Format: `[ID] [P?] Description`

- **[P]**: Can run in parallel with other [P] tasks in the same phase

---

## Phase 1: Config Struct & Handler Wiring

**Purpose**: Introduce the `Config` struct and thread `schema`/`requiredScope` through the handler. This is the load-bearing change everything else depends on.

- [X] T001 Add `Config` struct to `handler/extract.go` with `Schema string` and `RequiredScope string` fields; add `schema` and `requiredScope` string fields to `ExtractHandler` struct
- [X] T002 Update `NewHandler` to accept `config Config` as a fourth parameter; populate `h.schema` (defaulting to `"wd"` when empty) and `h.requiredScope` (defaulting to `"data_engineer"` when empty)
- [X] T003 Replace all four `"data_engineer"` string literals in `WindowExtract`, `CurrentExtract`, `ResetExtraction`, and `DiscoverTables` with `h.requiredScope`
- [X] T004 Replace inline `wd.user_org` schema-qualified references in `DiscoverTables` and `ListExecutions` with `h.schema + ".user_org"`
- [X] T005 Update `extract_test.go` — all `NewHandler(pool, auth, cfg)` call sites gain a fourth `Config{}` argument; existing tests must still pass

**Checkpoint**: `go build ./...` and `go test -race ./...` pass with no behaviour change. ✓

---

## Phase 2: Tier Check Removal

**Purpose**: Delete `IsDataAPIEnabled` and all call sites. The library must not reference `platform_usage_tier`.

- [X] T006 Delete `domain/tier.go` entirely
- [X] T007 [P] Remove the tier check block from `WindowExtract` (Steps 2 in the handler): delete the `IsDataAPIEnabled` call, the `err` handling, and the `!enabled` 403 response
- [X] T008 [P] Remove the tier check block from `CurrentExtract` (same pattern as T007)
- [X] T009 [P] Remove the tier check block from `ResetExtraction` (same pattern as T007)
- [X] T010 Remove any import of the tier function now that all call sites are gone; confirm `go vet ./...` passes
- [X] T011 Update `extract_test.go` — remove any test cases that assert tier-check 403 behaviour; add a comment noting tier gating is the host app's responsibility
- [X] T011a Add table-driven test in `extract_test.go` for scope mismatch: mount handler with `RequiredScope: "mop_data_engineer"`, send request with scope `"data_engineer"`, assert 403; send with `"mop_data_engineer"`, assert 200 — confirms scope check uses `h.requiredScope` not the literal string
- [X] T012 Verify: `grep -r "platform_usage_tier\|IsDataAPIEnabled" .` returns zero matches

**Checkpoint**: `go test -race ./...` passes; no references to `platform_usage_tier` in codebase. ✓

---

## Phase 3: Schema Parameterisation in Domain Layer

**Purpose**: Thread `schema string` through all domain functions so no `'wd'` literal remains in query strings.

- [X] T013 Update `domain/service.go` — `ValidateTable`: add `schema string` param; replace `table_schema = 'wd'` with `table_schema = $2` (or use string concat for the table reference); update call site in handler
- [X] T014 [P] Update `domain/service.go` — `ValidateBaseTable`: same pattern as T013
- [X] T015 [P] Update `domain/service.go` — `ExtractCurrent`: add `schema string` param; replace `"wd." + input.TableName` with `schema + "." + input.TableName`; update call site in handler
- [X] T016 [P] Update `domain/service.go` — `DiscoverTables`: add `schema string` param; replace all `table_schema = 'wd'` filter references with `'` + schema + `'` (information_schema queries use string param); update call site in handler
- [X] T017 Update `domain/execution.go` — add `schema string` param to `InsertOrReusePending`, `GetExecutionByID`, `CursorFor`, `CurrentExtractionCount`, `TransitionStarted`, `TransitionCompleted`, `InsertReset`, `ListExecutions`; replace all `wd.data_extraction_execution` references with `schema + ".data_extraction_execution"`; update all call sites in handler
- [X] T018 [P] Update `domain/deny.go` — add `schema string` param to `IsDenied`, `AddDeny`, `RemoveDeny`, `ListDeny`; replace all `wd.data_extraction_deny` references; update call sites in handler
- [X] T019 Pass `h.schema` from handler to every domain function call updated in T013–T018
- [X] T020 Verify: `grep -r "'wd'\." ./domain` returns zero matches; `grep -r "\"wd\.\"\|'wd'" ./handler` returns zero matches (excluding comments and test fixtures using schema name explicitly)
- [X] T021 Add table-driven test in `extract_test.go` (or a new `schema_test.go`) that mounts the handler with `Schema: "core"` and asserts: (a) `ValidateTable` receives `table_schema = 'core'` in the SQL query (use `pgxmock` or inspect the domain call argument); (b) `ExtractCurrent` targets `core.<table>` not `wd.<table>`; (c) zero-value `Schema` still targets `wd`
- [X] T021a Run `go test -race ./...` — all tests pass including the new schema parameterisation tests

**Checkpoint**: Zero hardcoded `wd` schema references in domain or handler packages. ✓

---

## Phase 4: Migration Export

**Purpose**: Export clean SQL (no Goose directives) as `embed.FS` for host app consumption.

- [X] T022 Create directory `migrations_sql/` at the repo root (depends on T006/T012 confirming which tier-related objects have been removed, so we know what to exclude from the exported SQL)
- [X] T023 Create `migrations_sql/001_data_extraction.sql` — copy DDL from `migrations/047_data_api.sql`, strip Goose directives (`-- +goose Up/Down/StatementBegin/End`), replace all `wd.` schema prefixes with unqualified table names (rely on `search_path`), remove the `platform_usage_tier` ALTER (tier-gating is now host-app concern), remove the `platform_config` INSERT (WoodenDollars-specific seed), keep `data_extraction_execution`, `data_extraction_execution_log`, `data_extraction_deny`, `data_extraction_deny_log` table DDL and indexes
- [X] T024 [P] Create `migrations_sql/002_deny_defaults.sql` — copy the `DO $$` seed block for deny entries; replace `wd.` prefixes with unqualified names; document that this is optional and a no-op when no admin user exists
- [X] T025 Create `migrations.go` at the repo root with:
  ```go
  package dataapi

  import "embed"

  //go:embed migrations_sql/*.sql
  var MigrationsFS embed.FS
  ```
- [X] T026 Write a test in `migrations_test.go` that reads `MigrationsFS`, asserts both SQL files are present and non-empty, and asserts neither file contains the string `'wd'` (confirming no hardcoded schema)
- [X] T027 Run `go test -race ./...` — all tests including T026 pass
- [X] T028 Final verification: `grep -rn "platform_usage_tier\|IsDataAPIEnabled" .` → 0 matches; `grep -rn "'wd'" ./domain ./handler ./migrations_sql` → 0 matches; `go vet ./...` → clean; `golangci-lint run` → clean
