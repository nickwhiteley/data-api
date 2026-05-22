# Feature Specification: Multi-Tenant Library

**Feature Branch**: `012-multi-tenant-library`
**Created**: 2026-05-22
**Status**: Draft

## Clarifications

### Session 2026-05-22

- Q: Should the schema name default to `"wd"` to preserve backwards compatibility with WoodenDollars? → A: Yes — `Schema` defaults to `"wd"` so existing WoodenDollars wiring requires no change.
- Q: Should `RequiredScope` default to `"data_engineer"`? → A: Yes — same reasoning; existing callers require no change.
- Q: Should the library run its own migrations? → A: No — the library exports `MigrationsFS embed.FS` so each host application incorporates the SQL into its own numbered migration sequence and controls execution.
- Q: Where should the `Schema` and `RequiredScope` fields live? → A: On the existing `Handler` config struct (or a new `Config` struct passed to `NewHandler`).
- Q: Should the tier check (`platform_usage_tier.data_api_enabled`) remain in the library? → A: No — it is WoodenDollars-specific. It moves to each host application's middleware stack. The library has no knowledge of platform tiers.
- Q: Does removing the tier check break the existing WoodenDollars integration? → A: Only in that WoodenDollars must add its own middleware before the data-api route group. A companion spec (WoodenDollars 017) covers this.

---

## User Scenarios & Testing

### User Story 1 — Schema-Configurable Handler (Priority: P1)

A platform engineer integrating data-api into a new application (e.g. MOP with schema `"core"`)
can configure the handler with their own schema name. All extraction queries, table validation,
and cursor state use that schema instead of the hardcoded `"wd"` default.

**Why this priority**: This is the primary blocker for MOP adoption. Without it, data-api is
a WoodenDollars-only library.

**Independent Test**: Mount the handler with `Schema: "core"`, call
`GET /data/extract/case`, and confirm the generated query targets `core.case_log` not
`wd.case_log`.

**Acceptance Scenarios**:

1. **Given** `NewHandler` is called with `Schema: "core"`, **When** `GET /data/extract/case`
   is called, **Then** `ValidateTable` queries `information_schema.tables` with
   `table_schema = 'core'` and the window extraction queries `core.case_log`.

2. **Given** `NewHandler` is called with no `Schema` value (zero value), **When** any route
   is called, **Then** the handler behaves identically to today — all queries target the `wd`
   schema.

3. **Given** `NewHandler` is called with `Schema: "wd"`, **When** `GET /data/extract/account`
   is called, **Then** behaviour is unchanged from the current implementation.

4. **Given** `Schema: "core"`, **When** `GET /data/extract/account/current` is called,
   **Then** `ExtractCurrent` queries `core.account` and `ValidateBaseTable` checks
   `information_schema.tables` with `table_schema = 'core'`.

5. **Given** `Schema: "core"`, **When** `GET /data/extract` (table discovery) is called,
   **Then** `DiscoverTables` filters `information_schema.tables` on `table_schema = 'core'`
   and the cursor and deny-list queries reference `core.data_extraction_execution` and
   `core.data_extraction_deny`.

---

### User Story 2 — Scope-Configurable Handler (Priority: P1)

A platform engineer can configure the required API key scope for data-api routes. The
hardcoded `"data_engineer"` string is replaced by a configurable `RequiredScope` field, so
different platforms can use their own role/scope taxonomy.

**Why this priority**: MOP's API key system will use its own scope names. Hardcoding
`"data_engineer"` couples the library to WoodenDollars' role model.

**Independent Test**: Mount the handler with `RequiredScope: "mop_data_engineer"`, call
`GET /data/extract/case` with an API key carrying scope `"data_engineer"`, and confirm a
403 is returned. Repeat with scope `"mop_data_engineer"` and confirm 200.

**Acceptance Scenarios**:

1. **Given** `RequiredScope: "mop_data_engineer"`, **When** a request arrives with scope
   `"data_engineer"`, **Then** the handler returns 403 with code `forbidden`.

2. **Given** `RequiredScope: "mop_data_engineer"`, **When** a request arrives with scope
   `"mop_data_engineer"`, **Then** the handler proceeds normally.

3. **Given** no `RequiredScope` value (zero value), **When** the handler is initialised,
   **Then** `RequiredScope` defaults to `"data_engineer"` and all existing behaviour is
   preserved.

4. **Given** `RequiredScope: "data_engineer"` (explicit), **When** a request arrives with
   scope `"data_engineer"`, **Then** the handler proceeds normally — no regression.

---

### User Story 3 — Migrations Exported (Priority: P1)

A platform engineer can import the data-api Go module and access its migration SQL via a
package-level `embed.FS` variable. They incorporate it into their own numbered migration
sequence and execute it via their own runner. The library never runs migrations itself.

**Why this priority**: MOP uses a custom sequential migration runner. WoodenDollars does too.
Neither can accept a library that runs migrations independently — ordering, idempotency, and
schema ownership must remain with the host application.

**Independent Test**: Write a Go test that reads `dataapi.MigrationsFS`, confirms all
expected SQL files are present and non-empty, and that none reference `'wd'` directly
(they must use a placeholder or rely on `search_path`).

**Acceptance Scenarios**:

1. **Given** a Go file imports `github.com/nickwhiteley/data-api`, **When** it references
   `dataapi.MigrationsFS`, **Then** the variable compiles and returns an `fs.FS` containing
   the migration SQL files.

2. **Given** the exported `MigrationsFS`, **When** a host app reads its files and executes
   them against a PostgreSQL database with `search_path` set to their schema, **Then** the
   `data_extraction_execution`, `data_extraction_execution_log`, `data_extraction_deny`, and
   `data_extraction_deny_log` tables are created in that schema.

3. **Given** the library is updated to a new version with an additional migration file,
   **When** a host app upgrades the module, **Then** the new file is available in
   `MigrationsFS` and the host app's migration runner applies it in its own sequence.

4. **Given** the library is imported, **When** the host application starts, **Then** no
   migration is run automatically — execution is entirely the host app's responsibility.

---

### User Story 4 — Tier Check Removed (Priority: P1)

The data-api library no longer references `platform_usage_tier` or any tier/feature-flag
table. Tier gating is the host application's concern, implemented as middleware before the
data-api route group.

**Why this priority**: `platform_usage_tier` is a WoodenDollars-specific table. MOP has a
different tier model. A shared library must not encode one product's business logic.

**Independent Test**: Grep the `data-api` codebase for `platform_usage_tier` and `tier` —
zero matches expected after this change (outside of tests that explicitly verify absence).

**Acceptance Scenarios**:

1. **Given** the updated library, **When** `grep -r "platform_usage_tier" ./` is run in the
   data-api repo, **Then** no matches are found.

2. **Given** the updated library, **When** `NewHandler` is called, **Then** no database
   queries against any tier table are issued by the library at any point in the request
   lifecycle.

3. **Given** a host app that does not gate data-api routes, **When** a request arrives,
   **Then** the library processes it normally — it is the host app's responsibility to gate
   access upstream.

---

## Non-Functional Requirements

- All changes are backwards-compatible: existing WoodenDollars wiring (`Schema: ""` or
  `Schema: "wd"`, `RequiredScope: ""` or `RequiredScope: "data_engineer"`) must continue
  to work without modification.
- No new external dependencies may be introduced.
- All existing tests must continue to pass. New tests cover schema parameterisation and
  scope parameterisation.
- `MigrationsFS` migration files must not hardcode a schema name; they must rely on
  `search_path` or a parameter so the host app controls schema placement.
