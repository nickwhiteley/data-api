# Feature Specification: Data API

**Feature Branch**: `010-data-api`  
**Created**: 2026-05-14  
**Status**: Draft  

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Window Data Extraction (Priority: P1)

A data engineer builds a pipeline that incrementally extracts all changes to a given table since
the last run. They call the window extraction endpoint with their API key, receive a DJSON
payload of all new or changed rows, and the platform records the extraction window so the next
call automatically continues from where this one left off.

**Why this priority**: This is the core value proposition of the feature. All other stories
depend on or complement this one.

**Independent Test**: A data engineer with a valid API key calls
`GET v1/data/extract/account` with no parameters, receives all `account_log` rows within the
computed window, and a subsequent call returns only rows changed after the first call's
`end_at`.

**Acceptance Scenarios**:

1. **Given** a data engineer with a valid API key and the data engineer role, and no prior
   extraction exists for this table, **When** they call `GET v1/data/extract/account`,
   **Then** they receive all `account_log` rows with `modified_at >= 2000-01-01` and
   `modified_at < (now − safety lag)`, in DJSON format, with `data_extraction_execution_id`
   in the response.

2. **Given** a prior completed extraction exists for this user and table, **When** they call
   `GET v1/data/extract/account` again, **Then** only rows changed since the previous
   `end_at` are returned.

3. **Given** a data engineer supplies `start_at` and `end_at` query parameters, **When** they
   call the endpoint, **Then** exactly the rows in that window are returned with no safety lag
   applied.

4. **Given** a data engineer requests page 1 with `row_count=100&page_number=1`, **When** the
   response is received, **Then** `data_extraction_execution_id` is included at the top level
   alongside `columns` and `rows`.

5. **Given** a data engineer requests page 2 with a valid `data_extraction_execution_id`,
   **When** the response is received, **Then** the same time window as page 1 is used.

6. **Given** a data engineer requests the final page and receives fewer rows than `row_count`
   (including zero), **When** the response is received, **Then** the extraction status is set
   to `completed` and the cursor advances to this `end_at`.

7. **Given** a client disconnects during page 1 transmission, **When** the data engineer
   retries page 1, **Then** the same `data_extraction_execution_id` is returned and the
   extraction restarts from the beginning of the window.

8. **Given** `row_count` exceeds 10,000, **When** the request is made, **Then** a 400 error
   is returned.

---

### User Story 2 — Current State Extraction (Priority: P2)

A data engineer needs a point-in-time snapshot of a table's current state rather than the
change history. They call the current extraction endpoint and receive all live rows for the
table. This is limited to two calls per user per calendar day per table.

**Why this priority**: Complements incremental extraction — useful for initial loads and
reconciliation. The rate limit makes it a deliberate, bounded operation.

**Independent Test**: A data engineer calls `GET v1/data/extract/account/current` and receives
all current `account` rows. A third call on the same calendar day returns a 429 error.

**Acceptance Scenarios**:

1. **Given** a data engineer has made fewer than 2 current extractions today for this table,
   **When** they call `GET v1/data/extract/account/current`, **Then** they receive all
   non-deleted rows from the `account` table in DJSON format.

2. **Given** a data engineer has already made 2 current extractions today for this table
   (with status `started` or `completed`), **When** they make a third call, **Then** a 429
   is returned.

3. **Given** a client disconnects before page 1 is fully sent (status stays `pending`),
   **When** the data engineer retries, **Then** the attempt does not count toward the daily
   limit.

4. **Given** a data engineer paginates a current extraction with `row_count` and
   `page_number`, **When** the last page is received, **Then** the extraction is marked
   `completed` and counted toward the daily limit.

---

### User Story 3 — Discovery (Priority: P3)

A data engineer or tenant admin wants to know which tables are available to extract. They call
the discovery endpoint and receive a list of table names with descriptions.

**Why this priority**: Enables self-service onboarding without needing to contact support or
consult internal documentation.

**Independent Test**: A data engineer calls `GET v1/data/extract` and receives a list of
available tables excluding denied and internal tables.

**Acceptance Scenarios**:

1. **Given** a data engineer calls `GET v1/data/extract`, **When** the response is received,
   **Then** the list contains only DB-2 shadow tables whose base table exists, excludes denied
   tables, and excludes `data_extraction_execution`.

2. **Given** a table has no database comment, **When** it appears in discovery, **Then** the
   `description` field is an empty string.

3. **Given** a table is on the platform deny list, **When** discovery is called, **Then** that
   table does not appear in the results.

---

### User Story 4 — Extraction Reset (Priority: P3)

A data engineer needs to re-extract data from an earlier point in time, for example after a
pipeline failure. They call the reset endpoint to rewind their cursor to a specific timestamp
or to the beginning of time.

**Why this priority**: Recovery path — not needed for the happy path but essential when
pipelines go wrong.

**Independent Test**: A data engineer calls `POST v1/data/extract/account/reset` with a
timestamp, then calls the window endpoint and receives rows starting from that timestamp.

**Acceptance Scenarios**:

1. **Given** a data engineer calls `POST v1/data/extract/account/reset` with `timestamp=2026-01-01T00:00:00Z`,
   **When** they then call `GET v1/data/extract/account`, **Then** rows from `2026-01-01`
   onwards are returned.

2. **Given** a data engineer calls reset with no `timestamp` parameter, **When** they then
   call the window endpoint, **Then** all rows from `2000-01-01` onwards are returned.

3. **Given** a table is on the platform deny list, **When** reset is called, **Then** a 404
   is returned.

---

### User Story 5 — Platform Admin: Deny List Management (Priority: P2)

A platform admin needs to prevent data engineers from extracting certain sensitive tables.
They manage a platform-wide deny list via a dedicated admin UI page.

**Why this priority**: A security gate — without it, sensitive data (e.g. session tokens,
config values) would be accessible to all data engineers across all tenants.

**Independent Test**: A platform admin adds `transaction_log` to the deny list; a data
engineer immediately receives 404 when attempting to extract that table.

**Acceptance Scenarios**:

1. **Given** a platform admin adds a table to the deny list, **When** a data engineer attempts
   to extract that table, **Then** a 404 is returned.

2. **Given** a table is being actively extracted (status `started`) when it is added to the
   deny list, **When** the in-progress extraction continues, **Then** it completes normally;
   only subsequent requests are blocked.

3. **Given** a platform admin removes a table from the deny list, **When** a data engineer
   extracts it, **Then** the extraction succeeds.

4. **Given** a platform admin attempts to add a table already on the deny list, **When** the
   request is submitted, **Then** a validation error is returned (duplicate active deny).

---

### User Story 6 — Extraction Stats (Priority: P3)

Tenant admins, data engineers, and platform admins can view extraction history to monitor
pipeline health and diagnose failures.

**Why this priority**: Operational visibility — useful but the feature is functional without it.

**Independent Test**: A tenant admin views the data extract management page, selects a data
engineer from the dropdown, and sees a paginated table of extractions with date range filter.

**Acceptance Scenarios**:

1. **Given** a tenant admin views the stats page, **When** they select a data engineer from
   the list, **Then** they see that engineer's extraction history within their tenant only.

2. **Given** a platform admin views the stats page, **When** they select a tenant and a data
   engineer, **Then** they see that engineer's extraction history.

3. **Given** a data engineer views the stats page, **When** they select another engineer in
   their tenant, **Then** they see that engineer's history (cross-visibility within tenant is
   permitted).

4. **Given** a user applies a date range filter, **When** results are shown, **Then** only
   extractions with `start_at` within the range are displayed.

---

### User Story 7 — Tenant Admin: API Key Management (Priority: P1)

A tenant admin provisions a data engineer by creating a user (or selecting an existing system
user), assigning the data engineer role, and creating an API key for that user.

**Why this priority**: Without this, there is no way to provision data engineers — it is a
prerequisite for all other stories.

**Independent Test**: A tenant admin opens the user management page, clicks edit on a user,
creates an API key with an expiry date, and the key can authenticate data API requests.

**Acceptance Scenarios**:

1. **Given** a tenant admin is on the user management page, **When** they click the edit
   button on a user row, **Then** they can create, view, and revoke API keys for that user.

2. **Given** a tenant admin sets an expiry date on an API key, **When** the expiry date
   passes, **Then** the key no longer authenticates.

3. **Given** an API key is associated with a user who has the data engineer role, **When**
   the key is used to call a data API endpoint, **Then** the request is authenticated and
   authorised.

4. **Given** an API key is associated with a user without the data engineer role, **When**
   the key is used to call a data API endpoint, **Then** a 403 is returned.

---

### Edge Cases

- What happens when `{table}` in the path does not exist or has no `_log` counterpart? → 404.
- What happens when an invalid or wrong-user `data_extraction_execution_id` is passed on
  page > 1? → 404.
- What happens when the tenant is not on the Enterprise tier? → 403 with a clear tier message.
- What happens when the exact row count is a multiple of `row_count`? → The caller must make
  one additional request returning zero rows to trigger `completed`; documented for pipeline
  authors.
- What happens when two simultaneous page 1 requests arrive from the same user for the same
  table? → The second request reuses the existing `pending` row; both return the same
  `data_extraction_execution_id`.
- What happens when `start_at` > `end_at` in a manual override? → 400.
- What happens when a tenant is downgraded from Enterprise mid-extraction? → In-progress
  extractions complete; subsequent requests return 403.

---

## Requirements *(mandatory)*

### Functional Requirements

**Data Engineer Role**

- **FR-001**: The system MUST fully implement the data engineer role so it can be assigned to
  a user and checked during API authorisation.
- **FR-002**: An API key MUST carry the data engineer role to access any data API endpoint.

**Window Extraction**

- **FR-003**: The system MUST expose `GET v1/data/extract/{table}` returning all rows from the
  corresponding `_log` table within the computed time window in DJSON format.
- **FR-004**: The extraction window MUST be `modified_at >= start_at AND modified_at < end_at`
  where `start_at` is the `end_at` of the most recent `completed` or `reset` execution for
  this user and table (or `2000-01-01` if none), and `end_at` is
  `clock_timestamp() − WD_DATA_EXTRACT_SAFETY_LAG_SECONDS`.
- **FR-005**: The safety lag default MUST be 5 seconds and MUST be configurable as
  `WD_DATA_EXTRACT_SAFETY_LAG_SECONDS` in the platform configuration store.
- **FR-006**: When `start_at` and `end_at` query parameters are supplied, the system MUST use
  those values directly with no safety lag applied.
- **FR-007**: The system MUST return all columns from the `_log` table. Column values MUST be
  serialised as follows: UUIDs as lowercase hyphenated strings
  (e.g. `"550e8400-e29b-41d4-a716-446655440000"`), timestamps as UTC ISO 8601 strings,
  NULLs as JSON `null`, booleans as JSON booleans, and numeric types as JSON numbers.

**Current Extraction**

- **FR-008**: The system MUST expose `GET v1/data/extract/{table}/current` returning all
  non-deleted rows from the base table in DJSON format.
- **FR-009**: The system MUST limit current extractions to a maximum of 2 per user per
  calendar day per table, counting only executions with status `started` or `completed`.
- **FR-010**: When the daily limit is reached, the system MUST return 429.

**Pagination**

- **FR-011**: Both extraction endpoints MUST support optional pagination via `row_count` (max
  10,000, default 1,000) and `page_number` (default 1) integer query parameters. Both
  parameters are optional; omitting them returns the first page of up to 1,000 rows.
- **FR-012**: The system MUST return 400 if `row_count` exceeds 10,000.
- **FR-013**: On page 1, the system MUST include `data_extraction_execution_id` as a top-level
  field in the response alongside `columns` and `rows`.
- **FR-014**: For pages > 1, the caller MUST supply `data_extraction_execution_id` as a query
  parameter; an invalid or wrong-user ID MUST return 404.
- **FR-015**: The extraction time window MUST be fixed at the start of page 1 and held
  constant across all subsequent pages of the same job.
- **FR-016**: When a page returns fewer rows than `row_count` (including zero), the system
  MUST mark the execution `completed` and advance the cursor to `end_at`.
- **FR-017**: If a client disconnects during page 1 and retries, the system MUST reuse the
  existing `pending` row, returning the same `data_extraction_execution_id`.
- **FR-018**: When the total rows are an exact multiple of `row_count`, the caller MUST make
  one additional request (returning zero rows) to trigger `completed`; this behaviour MUST
  be documented in the API reference.

**Reset**

- **FR-019**: The system MUST expose `POST v1/data/extract/{table}/reset` which records a
  dummy execution row with `status = reset`, `extract_type = window`, `row_count = 0`,
  `execution_time_taken = 0`, and `end_at` equal to the supplied `timestamp` parameter or
  `2000-01-01` if omitted.
- **FR-020**: The reset endpoint MUST return 404 for denied tables.

**Deny List**

- **FR-021**: Platform admins MUST be able to add and remove tables from a platform-wide deny
  list via a dedicated admin UI page.
- **FR-022**: Any request to extract or reset a denied table MUST return 404. The discovery endpoint MUST silently exclude denied tables from results without returning 404.
- **FR-023**: In-progress extractions MUST be allowed to complete when a table is added to the
  deny list; only subsequent requests are blocked.
- **FR-024**: The following tables MUST be seeded as default deny entries by the database
  migration: `platform_config_log`, `user_credential_log`, `session_log`,
  `data_extraction_execution_log`, `data_extraction_deny_log`.

**Discovery**

- **FR-025**: The system MUST expose `GET v1/data/extract` returning all available tables:
  only DB-2 shadow tables whose base table exists, excluding denied tables and
  `data_extraction_execution`.
- **FR-026**: Each entry MUST include `table_name` and `description` (from table comment;
  empty string if no comment exists).

**Security & Access**

- **FR-027**: All data extraction endpoints (`GET /v1/data/extract/*`, `POST /v1/data/extract/*/reset`) MUST require authentication via API key. The stats endpoint (`GET /v1/data/executions`) and admin deny-list endpoints MUST require session authentication.
- **FR-028**: The system MUST return 403 for any tenant not on the Enterprise tier; in-progress
  extractions at the time of downgrade MUST be allowed to complete.
- **FR-029**: Tier MUST be checked against `platform_usage_tier` on the tenant record.

**Frontend — Data Extract Management Page**

- **FR-030**: A new page reachable from a top-level navigation item (visible only to data
  engineers, tenant admins, and platform admins) MUST display extraction history with columns:
  table name, extract type, start\_at, end\_at, row count, execution time, status.
- **FR-031**: The page MUST include a date range filter and an engineer selector.
- **FR-032**: Platform admins MUST see a tenant selector above the engineer selector.
- **FR-033**: Tenant admins and data engineers MUST see an engineer selector scoped to their
  own tenant; a data engineer MAY view other data engineers' history within the same tenant.

**Frontend — Tenant Admin User Management**

- **FR-034**: The user management page MUST include an edit button per user row that allows a
  tenant admin to create, view, and revoke API keys for that user, including setting an expiry
  date.

**_log Table Migration**

- **FR-035**: All existing `_log` tables MUST be migrated from `DEFAULT NOW()` to
  `DEFAULT clock_timestamp()` for the `modified_at` column; `current.sql` MUST be kept
  aligned.

### Key Entities

- **Data Extraction Execution**: Records one extraction job — its time window (`start_at`,
  `end_at`), which user ran it, which table, how many rows were returned, how long it took,
  its type (window or current), and its lifecycle status (pending → started → completed, or
  reset for dummy rows). Scoped to a tenant.

- **Data Extraction Deny**: A platform-wide record that blocks all access to a named table
  via the data API. Soft-deleted when removed. Seeded with default sensitive tables.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A data engineer can extract incremental changes from any available table without
  manual coordination — the cursor advances automatically on each successful extraction.
- **SC-002**: Two data engineers in the same tenant can extract the same table simultaneously
  without interfering with each other's cursors.
- **SC-003**: No data change is missed or duplicated between consecutive successful window
  extractions for a given user and table.
- **SC-004**: A current extraction of any available table completes within 60 seconds for
  tables with up to 100,000 rows.
- **SC-005**: Platform admins can add a table to the deny list and have it take effect for new
  requests within one page reload, without a deployment.
- **SC-006**: A tenant admin can provision a new data engineer (user + API key) within
  5 minutes using only the UI, without platform admin involvement.
- **SC-007**: The default deny list is in place from the first deployment with no manual
  configuration required.

---

## Assumptions

- The data engineer role identifier already exists in the codebase and requires full
  implementation; no new role identifier needs to be coined.
- The existing API key middleware resolves a `user_id` from a key — the data API reads from
  this and does not require a separate auth path.
- System users (users created solely to hold API keys) are a valid pattern; the platform does
  not enforce that API keys be attached to human users.
- The DJSON format is as described at https://seethespark.com/blog/json-data; the exact schema
  will be documented inline in the plan.
- `audit_log` is intentionally accessible to data engineers and is not on the default deny
  list; this is a deliberate product decision.
- The tenant admin UI edit button for API key management is designed as an extension point;
  future iterations will add password reset and name change to the same modal/page.
- Table comments for discovery descriptions are deferred work; the migration to add comments
  is tracked separately and is not a blocker for this feature.
- The `WD_DATA_EXTRACT_SAFETY_LAG_SECONDS` config key follows the existing platform config
  pattern (`key`, `label`, `value`, `is_sensitive = false`).
