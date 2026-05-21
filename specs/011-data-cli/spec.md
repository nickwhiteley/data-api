# Feature Specification: Data CLI (wd-extract)

**Feature Branch**: `011-data-cli`  
**Created**: 2026-05-18  
**Status**: Draft  
**Input**: User description: "011 Data CLI"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Extract All Changed Data (Priority: P1)

A data engineer wants to extract all incremental changes from every available WoodenDollars table into local files for downstream processing (analytics, data warehouse loading, reporting). They run a single command with their API credentials and receive a set of output files, one per table, containing all rows changed since the last successful extraction.

**Why this priority**: This is the core job the tool exists to do. All other stories build on top of this fundamental extraction loop.

**Independent Test**: Run `wd-extract` with a valid API key against a populated environment. Verify that output files are created for each available table, that each file contains the expected columns and rows in the correct format, and that only changed data since the prior run is present.

**Acceptance Scenarios**:

1. **Given** a valid API key and no previous extraction, **When** `wd-extract` is run, **Then** files are created in the current directory for every available table, each named `table-name-YYYYMMDD-HHMMSS.csv`, containing all rows with a header line.
2. **Given** a prior completed extraction and new data since then, **When** `wd-extract` is run again, **Then** only rows changed since the previous extraction appear in the output files (the extraction window is tracked server-side by the API per API key; the CLI passes no date or cursor parameter).
3. **Given** a valid API key, **When** a table spans multiple pages of results, **Then** all pages are fetched and appended to a single file with the header written once.
4. **Given** no API key is configured in environment or flags, **When** `wd-extract` is run, **Then** a clear, human-readable error message is displayed and the tool exits with a non-zero code.

---

### User Story 2 — Extract a Specific Table or Selection (Priority: P2)

A data engineer wants to re-extract a single table (e.g. after a failed load) or a defined subset of tables, without triggering a full extraction. They use the `-t` or `-tables` flag to target only what they need.

**Why this priority**: Selective extraction is essential for operational recovery workflows and targeted refreshes. Running a full extraction when only one table is needed wastes time and API quota.

**Independent Test**: Run `wd-extract -t account` and verify only the `account` table file is created. Run `wd-extract -tables account,transaction` and verify exactly those two files are created, no others.

**Acceptance Scenarios**:

1. **Given** a valid API key, **When** `wd-extract -t account` is run, **Then** only the `account` output file is created; no other table files are produced.
2. **Given** a valid API key, **When** `wd-extract -tables account,transaction` is run, **Then** exactly two files are created — one for `account` and one for `transaction`.
3. **Given** a table name that does not exist, **When** `wd-extract -t nonexistent` is run, **Then** an informative error is shown and the tool exits with a non-zero code.

---

### User Story 3 — Choose Output Format (Priority: P3)

A data engineer needs output in a specific format to match their downstream pipeline. They use the `-f` flag to select CSV, TSV, or pipe-delimited output. The file extension reflects the format chosen.

**Why this priority**: Different loading tools (Redshift, BigQuery, Snowflake, custom parsers) have different format preferences. This is a configuration option that does not affect core correctness.

**Independent Test**: Run `wd-extract -f TSV` and verify output files have `.tsv` extension and values are tab-separated. Run `wd-extract -f PIPE` and verify `.txt` files with pipe-separated values.

**Acceptance Scenarios**:

1. **Given** `-f CSV` (or no `-f` flag), **When** extraction runs, **Then** output files have `.csv` extension and values are comma-separated with standard quoting.
2. **Given** `-f TSV`, **When** extraction runs, **Then** output files have `.tsv` extension and values are tab-separated.
3. **Given** `-f PIPE`, **When** extraction runs, **Then** output files have `.txt` extension and values are pipe (`|`) delimited.
4. **Given** an unrecognised format value, **When** `-f INVALID` is passed, **Then** the tool exits immediately with a clear usage error.
5. **Given** any format, **When** a field value contains the delimiter character, **Then** the value is correctly quoted so the output remains parseable.
6. **Given** any format, **When** a field value is null/absent, **Then** it is represented as an empty field (not the literal string "NULL").

---

### User Story 4 — Partition Output by Date (Priority: P3)

A data engineer wants output files organised into date-based subdirectories to support Hive-style partition pruning in their data lake or S3 bucket. They use the `-p` flag with either `DATEMOD` (partition by the row's modification date) or `DATERUN` (partition by the date the extraction ran).

**Why this priority**: Partitioning is a loading optimisation, not a correctness requirement. The tool is fully functional without it, but data lake consumers benefit greatly.

**Independent Test**: Run `wd-extract -p DATERUN=yyyy-mm-dd` and verify output files are placed in subdirectories named by today's date (e.g. `account/2026-05-18/account-20260518-103000.csv`). Run `wd-extract -p DATEMOD=yyyy-mm` and verify rows are routed to subdirectories matching their `modified_at` month.

**Acceptance Scenarios**:

1. **Given** `-p DATERUN=yyyy-mm-dd`, **When** extraction runs, **Then** each table's output file is placed under `{output_dir}/{table_name}/{run_date}/filename`.
2. **Given** `-p DATEMOD=yyyy-mm-dd`, **When** extraction runs and rows span multiple modification dates, **Then** rows are written to separate files in their respective date subdirectories under `{output_dir}/{table_name}/{mod_date}/`.
3. **Given** `-p DATEMOD=yyyy-mm`, **When** rows span multiple months, **Then** separate files exist under `account/2026-04/` and `account/2026-05/` respectively.
4. **Given** an invalid partition pattern, **When** `-p INVALID=xyz` is passed, **Then** the tool exits immediately with a clear usage error listing valid options.

---

### User Story 5 — Configure Output Location and Row Batch Size (Priority: P4)

A data engineer wants to write output to a specific directory (not the current working directory) and control how many rows are fetched per API request to tune throughput or avoid timeouts.

**Why this priority**: Operational configuration. Most users will use the defaults; some environments require explicit paths or fine-tuned batch sizes.

**Independent Test**: Run `wd-extract -o /tmp/data` and verify all files are written under `/tmp/data/`. Run `wd-extract -r 500` and verify the tool completes correctly using 500-row pages.

**Acceptance Scenarios**:

1. **Given** `-o /tmp/output`, **When** extraction runs, **Then** all output files are created under `/tmp/output/` not the current directory.
2. **Given** `-r 500`, **When** a table has 1200 rows, **Then** the tool fetches 3 pages (500, 500, 200) and produces a single file with 1200 rows and one header.
3. **Given** no `-r` flag, **When** extraction runs, **Then** the tool fetches 1000 rows per page.

---

### Edge Cases

- What happens when an API request fails transiently (network error, 5xx response)? → Retried automatically up to 3 times with increasing delays; failure after all retries logs the error and (in all-tables mode) continues with the next table.
- What happens when the API returns rate-limit (429) responses? → Same retry policy with backoff applies; treated identically to a 5xx transient error.
- What happens if the process is interrupted mid-extraction (e.g. SIGTERM, crash)? → Any partially written output file for the in-progress table is deleted. The extraction cursor for that table remains at the previous position, so the next run re-extracts from the same point cleanly.
- What happens when one table in an all-tables extraction fails permanently (e.g. access denied)? → The error is logged to stderr, the tool continues with remaining tables, and exits with a non-zero code at the end.
- What happens when a DATEMOD partition references a `modified_at` column that is absent from the table's response? → The tool exits with a clear error identifying the missing column before writing any output.
- What happens when the API base URL is not reachable? → After retries are exhausted, the tool exits with a connection error and non-zero code.
- What happens when `-r` is set above the API's maximum allowed page size? → The API returns a validation error; the tool surfaces it clearly and exits.
- What happens when the output directory does not exist? → The tool creates it (including any needed parent directories) before writing.

## Requirements *(mandatory)*

### Functional Requirements

**Configuration & Authentication**

- **FR-001**: The tool MUST read the API key from the `WD_API_KEY` environment variable or the `-k` CLI flag; if neither is provided it MUST exit with a descriptive error message.
- **FR-002**: The tool MUST use `https://woodendollars.com` as the default API base URL; this MUST be overridable via the `WD_API_URL` environment variable or the `-u` CLI flag.
- **FR-003**: The tool MUST display comprehensive help text when invoked with `-h` or `-help`.

**Table Selection**

- **FR-004**: When run with no table selection flags, the tool MUST discover all available tables from the platform and extract each one in turn.
- **FR-005**: The `-t <table>` flag MUST restrict extraction to the single named table.
- **FR-006**: The `-tables <t1,t2,...>` flag MUST restrict extraction to the comma-separated list of named tables.
- **FR-007**: Tables MUST be extracted sequentially (one at a time), not concurrently.

**Extraction**

- **FR-008**: The tool MUST perform incremental window extraction (changed data since the last successful run) rather than full snapshot extraction.
- **FR-009**: The default number of rows fetched per API request MUST be 1000; this MUST be overridable with the `-r <N>` flag.
- **FR-010**: When a table's data spans multiple pages, the tool MUST fetch all pages and append them to the same output file, writing the column header exactly once.
- **FR-011**: The tool MUST automatically retry failed requests (transient network errors, 5xx responses, and 429 rate-limit responses) using exponential backoff, up to a maximum of 3 attempts per request.
- **FR-012**: If the extraction process is interrupted mid-table, the tool MUST delete any partially written output file for that table on the next invocation before restarting it. *(See Assumptions.)*

**Output Formatting**

- **FR-013**: The tool MUST support three output formats selectable via `-f`: `CSV` (default), `TSV`, and `PIPE` (pipe-delimited).
- **FR-014**: The file extension of each output file MUST reflect the chosen format: `.csv` for CSV, `.tsv` for TSV, `.txt` for PIPE.
- **FR-015**: Null/absent field values MUST be written as empty fields (not as the string "NULL" or any other sentinel).
- **FR-016**: Field values containing the delimiter character MUST be correctly quoted so the file remains parseable.
- **FR-017**: The default output directory MUST be the current working directory; this MUST be overridable with the `-o <dir>` flag. The tool MUST create the directory (and any parents) if it does not exist.

**File Naming & Partitioning**

- **FR-018**: Output files MUST be named `{table-name}-{YYYYMMDD}-{HHMMSS}.{ext}`, where the timestamp is the local clock time at the moment the tool begins extracting that table.
- **FR-019**: When no partition flag is specified, output files MUST be written directly into the output directory (flat layout).
- **FR-020**: The `-p DATERUN={format}` flag MUST place each table's output file under `{output_dir}/{table_name}/{run_date_value}/`.
- **FR-021**: The `-p DATEMOD={format}` flag MUST partition rows by their `modified_at` field value; rows from different date partitions MUST be written to separate files under `{output_dir}/{table_name}/{mod_date_value}/`. When a row's `modified_at` value is null or empty, the row MUST be routed to a subdirectory named `unknown` (i.e. `{output_dir}/{table_name}/unknown/`).
- **FR-022**: Partition date formats MUST be limited to: `yyyy-mm-dd`, `yyyymmdd`, `yyyy-mm-dd-hh`, `yyyy`, `yyyy-mm`. Any other value MUST produce a clear error before any output is written.

**Error Handling & Exit Codes**

- **FR-023**: All diagnostic messages, progress output, and error messages MUST be written to stderr. Output data files MUST NOT contain diagnostic content.
- **FR-024**: In all-tables extraction mode, a failure on one table MUST NOT prevent the remaining tables from being extracted.
- **FR-025**: The tool MUST exit with code `0` only if all requested tables were extracted successfully; it MUST exit with a non-zero code if any table failed.

**Distribution**

- **FR-026**: The tool MUST be distributed as a standalone binary for macOS, Linux, and Windows, available for download from GitHub Releases.

### Key Entities

- **Extraction Run**: A single invocation of the tool. Has a start timestamp, a set of target tables, and a final exit status.
- **Table Extraction**: The work of fetching all pages for one table and writing them to one or more output files. Has its own start timestamp, page count, total row count, and success/failure status.
- **Output File**: A file written to the local filesystem. Named by table, timestamp, and format extension. May be placed in a partition subdirectory. Contains a header row followed by data rows.
- **Partition**: A date-derived subdirectory under `{output_dir}/{table_name}/`. Created on demand. Determined either by run time (`DATERUN`) or by each row's modification timestamp (`DATEMOD`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A data engineer can extract all available tables from a populated environment in a single command invocation, with no manual steps.
- **SC-002**: A full extraction of all tables completes without manual intervention even when individual tables have more than 10,000 rows (pagination handled automatically).
- **SC-003**: Output files are immediately loadable by standard data tools (spreadsheets, SQL loaders, pandas) without pre-processing or manual format correction.
- **SC-004**: A failed or interrupted extraction of one table does not corrupt the output of other tables in the same run.
- **SC-005**: Re-running the tool after a prior successful run produces output containing only rows changed since the last run (no duplicate data).
- **SC-006**: A data engineer new to the tool can configure it and complete a successful extraction within 5 minutes using only the `-h` help output.
- **SC-007**: Transient API failures (network blips, temporary rate limits) are resolved automatically without operator intervention for at least 3 retry attempts.
- **SC-008**: Partitioned output can be loaded by data lake tools using directory-based partition pruning without any renaming or restructuring of the output files.

## Assumptions

- The tool only performs incremental window extractions. Full point-in-time snapshot extraction is out of scope for this version.
- Extraction window state (determining which rows constitute "changed since last run") is managed entirely server-side by the WoodenDollars Data API on a per-API-key basis. The CLI maintains no local cursor file, timestamp file, or extraction state between runs. Each invocation calls the API with no date or cursor parameter; the API returns the appropriate changed-data window.
- Parallel table extraction is out of scope for this version; tables are processed sequentially.
- Date window override flags (specifying a custom start or end date for the extraction window) are out of scope for this version and will be added in a future iteration.
- The tool targets the WoodenDollars platform's Data API exclusively. No other data sources are in scope.
- Local filesystem output is the only supported destination in this version. The tool is architected to support other destinations (e.g. cloud object storage) in a future version.
- The `modified_at` column name is a fixed platform convention present on all extractable tables. No column name configuration is needed.
- Partial file detection on restart (FR-012) relies on the presence of a file with the same name prefix in the output directory. The tool deletes it and starts fresh; no resume-from-page logic is implemented.
- The binary is self-contained. No runtime dependencies (package managers, runtimes) are required on the operator's machine.
- A valid WoodenDollars API key with the `data_engineer` scope is a prerequisite; the tool does not create or manage API keys.
