# Implementation Plan: Multi-Tenant Library

**Branch**: `012-multi-tenant-library` | **Date**: 2026-05-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/012-multi-tenant-library/spec.md`

## Summary

Make the data-api library reusable across multiple host applications by: (1) adding a `Config`
struct to the handler with configurable `Schema` and `RequiredScope` fields, replacing the six
hardcoded `'wd'` references in the domain layer and four hardcoded `"data_engineer"` literals
in the handler layer; (2) deleting the `domain/tier.go` file and all `IsDataAPIEnabled` call
sites — tier gating moves entirely to host app middleware; (3) exporting migration SQL as
`embed.FS` (`dataapi.MigrationsFS`) so host apps control migration execution and schema
placement.

All changes are backwards-compatible: zero-value `Schema` defaults to `"wd"` and zero-value
`RequiredScope` defaults to `"data_engineer"`.

## Technical Context

**Language/Version**: Go 1.26+
**Primary Dependencies**: chi router (github.com/go-chi/chi/v5), pgx/v5
**Storage**: PostgreSQL — schema name is now runtime-configurable
**Testing**: `go test -race ./...`, table-driven tests in `handler/extract_test.go`
**Target Platform**: Library (imported by host applications)
**Project Type**: Go library
**Performance Goals**: No additional latency — schema name is stored on the handler struct, not looked up per-request
**Constraints**: No new external dependencies; backwards-compatible `NewHandler` signature change

## Constitution Check

| Rule | Status | Notes |
|------|--------|-------|
| No hardcoded schema | PASS after | Removing all `'wd'` references — that is the point of this spec |
| No hardcoded scope | PASS after | Removing all `"data_engineer"` literals |
| Tier check removed | PASS after | `domain/tier.go` deleted; `IsDataAPIEnabled` removed from all call sites |
| Migrations exported | PASS after | `embed.FS` variable exposed; host app controls execution |
| Backwards compatibility | PASS | Zero-value defaults preserve all existing behaviour |
| No new dependencies | PASS | Pure refactor within existing packages |

## Project Structure

### Documentation (this feature)

```text
specs/012-multi-tenant-library/
├── plan.md              ← This file
├── spec.md
└── tasks.md
```

### Source Code Changes

```text
handler/
├── auth.go              ← No change (AuthContext, DataConfig interfaces unchanged)
├── extract.go           ← Config struct added; schema + requiredScope fields on handler;
│                           "data_engineer" literals → h.requiredScope;
│                           IsDataAPIEnabled calls removed;
│                           DiscoverTables/ListExecutions wd.user_org → h.schema+".user_org"
└── extract_test.go      ← Tests updated for Config param; tier-check tests removed

domain/
├── service.go           ← All 'wd' schema refs parameterised via schema string arg
├── deny.go              ← IsDenied receives schema arg
├── execution.go         ← Execution queries receive schema arg
├── tier.go              ← DELETED
└── ...

migrations/             ← EXISTING goose SQL stays; NEW schema-agnostic export added
│
migrations.go           ← NEW: package-level embed.FS export (MigrationsFS)
migrations_sql/         ← NEW directory: plain SQL (no Goose directives), search_path-relative
├── 001_data_extraction.sql   ← data_extraction_execution, _log, _deny, _deny_log tables
└── 002_deny_defaults.sql     ← optional seed (noop on no admin user)
```

## Design Decisions

### Config struct vs individual parameters

Adding a `Config` struct to `NewHandler` is preferred over individual parameters because:
- It preserves the existing positional `pool, auth, cfg` signature (Config appended)
- Future additions (e.g. `MaxRowCount int`) don't break the call site again
- Zero-value struct gives safe defaults

```go
// Config holds library-level configuration for the handler.
type Config struct {
    // Schema is the PostgreSQL schema name for all extraction queries.
    // Defaults to "wd" if empty.
    Schema string
    // RequiredScope is the API key scope checked on every extraction request.
    // Defaults to "data_engineer" if empty.
    RequiredScope string
}
```

**Schema validation**: An empty `Schema` defaults to `"wd"` (backwards compat) rather than
being rejected. Host apps that intentionally pass an empty value get `"wd"` behaviour. A blank
non-empty schema (e.g. whitespace-only) is not validated in this spec — host apps are
responsible for passing a valid PostgreSQL identifier.

### Schema propagation in domain layer

Domain functions (ValidateTable, ValidateBaseTable, ExtractCurrent, DiscoverTables, CursorFor,
IsDenied, and all execution functions) receive a `schema string` parameter. The handler stores
`h.schema` and passes it on every domain call. This is the most surgical approach — no new
types introduced in the domain layer.

### Migration export format

The existing Goose-formatted migrations stay in `migrations/` for WoodenDollars' own CI
tooling. A new `migrations_sql/` directory contains plain SQL (no Goose directives) that uses
`search_path`-relative names (no `wd.` prefix). These are exported via `MigrationsFS`. Host
apps set their own `search_path` before executing.

```go
//go:embed migrations_sql/*.sql
var MigrationsFS embed.FS
```

`migrations.go` lives at the **repo root** with `package dataapi` — the same package exposed
to importers as `github.com/nickwhiteley/data-api`. This makes the variable accessible as
`dataapi.MigrationsFS` from host applications without any sub-package import.

### DiscoverTables session-auth path

`DiscoverTables` has a secondary path for session-auth users (tenant_admin, platform_admin)
that queries `wd.user_org`. This query must also use `h.schema`. Updated to
`h.schema + ".user_org"`. Same for `ListExecutions`.

## Affected Call Sites Summary

### handler/extract.go — scope check (4 instances)

| Handler | Before | After |
|---------|--------|-------|
| WindowExtract | `!= "data_engineer"` | `!= h.requiredScope` |
| CurrentExtract | `!= "data_engineer"` | `!= h.requiredScope` |
| ResetExtraction | `!= "data_engineer"` | `!= h.requiredScope` |
| DiscoverTables | `== "data_engineer"` | `== h.requiredScope` |

### handler/extract.go — tier check (3 call sites, all removed)

`WindowExtract`, `CurrentExtract`, `ResetExtraction` — `IsDataAPIEnabled` calls and their
error-handling blocks removed entirely.

### handler/extract.go — schema-qualified queries (2 instances)

`DiscoverTables` and `ListExecutions` have inline `wd.user_org` references — replaced with
`h.schema + ".user_org"`.

### domain/service.go — schema in SQL (6 instances)

`ValidateTable`, `ValidateBaseTable`, `ExtractCurrent`, `DiscoverTables`, and the cursor
query in `CursorFor` — all `'wd'` schema references in SQL strings become `'` + schema + `'`
(for `information_schema` queries) or `schema + "."` (for table-qualified references).

### domain/execution.go and domain/deny.go

All `wd.data_extraction_execution`, `wd.data_extraction_deny` references become
`schema + ".data_extraction_execution"` etc. Schema passed as argument.
