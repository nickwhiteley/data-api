# Tasks: Data CLI (wd-extract)

**Input**: Design documents from `specs/011-data-cli/`
**Branch**: `011-data-cli`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ contracts/cli-interface.md ✅ quickstart.md ✅

**Tests**: Mandatory per Constitution Principle V (Test Discipline, NON-NEGOTIABLE). Unit tests via `net/http/httptest` mock server; table-driven; `-race` flag; `t.Parallel()` where safe; `t.Cleanup` for teardown.

**Organization**: Phases 3–7 map to user stories from spec.md (US1–US5), each independently implementable and testable.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no blocking dependency within phase)
- **[USn]**: Maps to user story n from spec.md

---

## Phase 1: Setup

**Purpose**: New `cli/` Go module, package skeletons, release tooling, and linting config.

- [ ] T001 Create directory tree: `cli/cmd/wd-extract/`, `cli/internal/config/`, `cli/internal/wdapi/`, `cli/internal/format/`, `cli/internal/partition/`, `cli/internal/output/`
- [ ] T002 Initialise `cli/go.mod` with `module github.com/nickwhiteley/woodendollars/cli` and `go 1.26`; create `cli/cmd/wd-extract/main.go` with `package main` stub and `cli/internal/*/` package stubs
- [ ] T003 [P] Create `.goreleaser.yml` at repo root: builds `./cli/cmd/wd-extract` binary `wd-extract` for linux/darwin/windows × amd64/arm64; `ldflags: -trimpath -X main.version={{.Version}}`; tar.gz archives with SHA256 checksums
- [ ] T004 [P] Create `cli/.golangci.yml` aligned with existing project linting rules (errcheck, govet, staticcheck, gofumpt); verify `golangci-lint run ./...` passes against empty stubs from T002

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core interfaces and HTTP infrastructure that every user story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T005 Define `Formatter` interface in `cli/internal/format/formatter.go`: methods `WriteHeader(cols []string) error`, `WriteRow(row []string) error`, `Flush() error`
- [ ] T006 [P] Define `Writer` interface and `LocalWriter` concrete type in `cli/internal/output/writer.go`: `Writer.Create(path string) (io.WriteCloser, error)`; `LocalWriter.Create` calls `os.MkdirAll` on parent dir then `os.Create`
- [ ] T007 [P] Implement HTTP client in `cli/internal/wdapi/client.go`: `Client` struct holding base URL and API key; `do(ctx, req)` method that sets `Authorization: Bearer <key>` header and 30 s timeout; `retryDo(ctx, req)` wrapper: 3 attempts, exponential backoff 1 s/2 s/4 s on 429 and 5xx; returns `(*http.Response, error)`
- [ ] T008 Implement `Config` struct in `cli/internal/config/config.go` with fields `APIKey`, `APIURL`; resolve `APIKey` from `WD_API_KEY` env then `-k` flag (error if neither); resolve `APIURL` from `WD_API_URL` env then `-u` flag (default `https://woodendollars.com`); register `-h`/`-help` to print usage and exit 0
- [ ] T008a [P] Implement custom `slog.Handler` in `cli/internal/log/handler.go` satisfying Constitution Principle VI: outputs lines in the form `[wd-extract] <message>` to `os.Stderr`; expose `func New() *slog.Logger` returning a logger using this handler at `slog.LevelInfo`; API key MUST never appear in any log record or field value

**Checkpoint**: HTTP client, formatter interface, writer interface, base config, and slog logger ready — user story implementation can now begin.

---

## Phase 3: User Story 1 — Extract All Changed Data (Priority: P1) 🎯 MVP

**Goal**: `wd-extract` with no table-selection flags discovers all tables, extracts each one in turn using window (incremental) extraction, handles multi-page pagination, and writes one CSV file per table to the current directory.

**Independent Test**: Run `wd-extract` with a valid API key pointed at a populated environment. Verify one `.csv` file is created per available table; each file has a single header row; page boundaries produce no duplicate or missing rows; the tool exits 0 on success.

### Tests for User Story 1

- [X] T009 [P] Write table-driven unit tests for `Config` in `cli/internal/config/config_test.go`: missing `APIKey` → error; `WD_API_KEY` env overrides default; `-k` flag overrides env; `APIURL` default; `WD_API_URL` env override; `-r` default 1000; `-o` default `.`; `-f` default `CSV`; `-r 0` → `Validate()` error; `-r 10001` → `Validate()` error; `-r 10000` → valid; `-f EXCEL` → `Validate()` error with message listing valid values
- [X] T010 [P] Write `httptest.NewServer` tests for `PageExtract` in `cli/internal/wdapi/extract_test.go`: single-page response (happy path); two-page response (second call carries `data_extraction_execution_id`); 429 triggers retry and succeeds on second attempt; 5xx triggers retry; retries exhausted returns error; server returns 403 returns wrapped error; `json.Number` integers serialise without scientific notation; first-page request URL contains no `since`, `from_date`, or cursor parameter (verifies CLI statelessness — SC-005 is enforced by the API, not the CLI)
- [X] T011 [P] Write table-driven unit tests for CSV `Formatter` in `cli/internal/format/format_test.go`: header written once; `nil` value → empty field; comma inside field → RFC 4180 quoting; boolean `true`/`false` → string; `json.Number` large integer → exact decimal; `Flush` required before file close

### Implementation for User Story 1

- [X] T012 Extend `Config` in `cli/internal/config/config.go` to add `RowCount int` (default 1000, range 1–10000), `OutputDir string` (default `.`), `Format OutputFormat` (default `CSV`); add `Validate() error` that fails fast on out-of-range row count and unknown format
- [X] T013 [P] Define `DJSONPage` struct in `cli/internal/wdapi/extract.go`: fields `ExecutionID string`, `Columns []string`, `Rows [][]any`; implement `PageExtract(ctx context.Context, c *Client, table string, page int, executionID string, rowCount int) (*DJSONPage, error)`: builds URL `GET /v1/data/extract/{table}?row_count=N&page_number=N[&data_extraction_execution_id=...]`, calls `c.retryDo`, decodes response with `json.Decoder.UseNumber()`
- [X] T014 [P] Implement `DiscoverTables(ctx context.Context, c *Client) ([]string, error)` in `cli/internal/wdapi/tables.go`: calls `GET /v1/data/extract`, decodes `{"tables":[{"table_name":"..."},...]}`, returns ordered slice of table names
- [X] T015 Implement `csvFormatter` in `cli/internal/format/csv.go`: wraps `encoding/csv.NewWriter` with comma delimiter; implements `Formatter` interface; `WriteHeader` and `WriteRow` call `csv.Writer.Write`; `Flush` calls `csv.Writer.Flush` then checks `Error()`
- [X] T016 Implement `OutputManager` in `cli/internal/output/manager.go`: `New(w Writer, tableName string, startedAt time.Time, format OutputFormat, partition *partition.Partition, outputDir string) *OutputManager`; non-partitioned mode: single file opened lazily; `WriteRow(row []string, modifiedAt string) error`; on first `WriteRow` call, scan `outputDir` for any existing `{tableName}-*.{ext}` file and delete it (partial cleanup); write header once; `Files() []string`; `Close() error` flushes and closes all handles
- [X] T017 Write unit tests for `OutputManager` in `cli/internal/output/manager_test.go`: file created under correct path formula `{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}`; header row present once; partial-file cleanup deletes pre-existing matching file; `Close` flushes buffered content; zero rows produces file with header only
- [X] T018 Implement `run(cfg *config.Config) error` in `cli/cmd/wd-extract/main.go`: constructs `wdapi.Client`; calls `DiscoverTables`; loops tables sequentially; per-table: creates `OutputManager`, calls `PageExtract` in a loop until `len(page.Rows) < cfg.RowCount`, writes each row, calls `Close`; uses the `slog.Logger` from `cli/internal/log` at `INFO` level for start/page/completion events and `ERROR` level for per-table failures; on per-table error logs and continues; returns error if any table failed
- [X] T019 Wire `main()` in `cli/cmd/wd-extract/main.go`: parse `Config`, call `run`; exit 0 on nil error, exit 1 otherwise; verify `go build ./cli/cmd/wd-extract` succeeds; verify `go test -race ./cli/...` passes

**Checkpoint**: US1 fully functional. `wd-extract` extracts all tables to CSV in CWD. Independent test passes.

---

## Phase 4: User Story 2 — Extract a Specific Table or Selection (Priority: P2)

**Goal**: `-t <table>` and `-tables <list>` flags restrict extraction to the named table(s) without calling DiscoverTables.

**Independent Test**: `wd-extract -t account` creates only `account-*.csv`. `wd-extract -tables account,transaction` creates exactly two files. `wd-extract -t nonexistent` exits 1 with a descriptive error.

### Tests for User Story 2

- [X] T020 [P] Extend `config_test.go` with cases: `-t` and `-tables` mutually exclusive → error; `-t` alone sets `Config.Table`; `-tables` parses comma-separated list; neither flag → `Config.Table` and `Config.Tables` both empty (all-tables mode)

### Implementation for User Story 2

- [X] T021 Extend `Config` in `cli/internal/config/config.go`: add `Table string` (`-t`) and `Tables []string` (`-tables`, parsed from comma-separated string); add mutual-exclusivity check in `Validate()`: error if both `-t` and `-tables` are set
- [X] T022 Extend `run()` in `cli/cmd/wd-extract/main.go`: when `cfg.Table` is set, use `[]string{cfg.Table}` directly; when `cfg.Tables` is set, use it directly; skip `DiscoverTables` call in both cases; return error (exit 1) if a named table returns 404 from the API
- [X] T023 Write `httptest.NewServer` test in `cli/internal/wdapi/extract_test.go`: 404 response returns a typed `TableNotFoundError`; verify `run()` surfaces it correctly
- [X] T024 Verify `go test -race ./cli/...` passes; run `wd-extract -t account` and `wd-extract -tables account,transaction` manually against the mock server test to confirm correct file counts

**Checkpoint**: US2 complete. Selective extraction works independently of US1's all-tables path.

---

## Phase 5: User Story 3 — Choose Output Format (Priority: P3)

**Goal**: `-f TSV` produces `.tsv` files with tab-separated values; `-f PIPE` produces `.txt` files with pipe-delimited values. File extensions match format. Default CSV unchanged.

**Independent Test**: `wd-extract -f TSV` produces `.tsv` files whose content is parseable as tab-separated. `wd-extract -f PIPE` produces `.txt` files. `-f INVALID` exits 1 with a clear error listing valid values.

### Tests for User Story 3

- [X] T025 [P] Extend `format_test.go`: TSV formatter uses tab delimiter; PIPE formatter uses `|`; both handle null→empty and quoting identically to CSV formatter; extension mapping: `CSV→.csv`, `TSV→.tsv`, `PIPE→.txt`

### Implementation for User Story 3

- [X] T026 [P] Implement `tsvFormatter` in `cli/internal/format/tsv.go`: wraps `encoding/csv.NewWriter` with `\t` delimiter; implements `Formatter` interface
- [X] T027 [P] Implement `pipeFormatter` in `cli/internal/format/pipe.go`: wraps `encoding/csv.NewWriter` with `|` delimiter; implements `Formatter` interface
- [X] T028 Add `NewFormatter(format config.OutputFormat) (Formatter, error)` factory in `cli/internal/format/formatter.go` and `FileExtension(format config.OutputFormat) string` helper returning `.csv`/`.tsv`/`.txt`
- [X] T029 Update `OutputManager` in `cli/internal/output/manager.go` to call `format.NewFormatter(cfg.Format)` and `format.FileExtension(cfg.Format)` when creating output files; verify `go test -race ./cli/...` passes

**Checkpoint**: US3 complete. All three formats produce correctly delimited, correctly named files.

---

## Phase 6: User Story 4 — Partition Output by Date (Priority: P3)

**Goal**: `-p DATERUN=yyyy-mm-dd` places files in `{outputDir}/{table}/{runDate}/`. `-p DATEMOD=yyyy-mm` routes each row to the matching month subdirectory. Multiple DATEMOD partitions in one run produce separate files.

**Independent Test**: `wd-extract -p DATERUN=yyyy-mm-dd -o /tmp/out` creates `account/2026-05-18/account-*.csv`. `wd-extract -p DATEMOD=yyyy-mm` with data spanning April and May creates both `account/2026-04/` and `account/2026-05/` subdirectories each containing their respective rows. `-p INVALID=xyz` exits 1 with a clear error.

### Tests for User Story 4

- [X] T030 [P] Write table-driven unit tests for `Partition` in `cli/internal/partition/partition_test.go`: all 5 spec format tokens parse correctly to Go `time.Format` strings; unknown token returns error; `DATERUN` path resolution for each format; `DATEMOD` path resolution from sample `modified_at` timestamp string
- [X] T031 [P] Extend `manager_test.go` for partitioned mode: DATERUN creates single subdirectory; DATEMOD with two distinct `modified_at` dates creates two subdirectories each with their own file; header written once per file; DATEMOD rows with `modified_at=null` routed to `unknown/` partition

### Implementation for User Story 4

- [X] T032 Implement `Partition` struct in `cli/internal/partition/partition.go`: fields `Mode string` (DATEMOD/DATERUN), `SpecFmt string`, `GoFmt string`; `Parse(raw string) (*Partition, error)` function parses `-p DATEMOD=yyyy-mm-dd` style input; validates mode and format token; `Resolve(ts time.Time) string` returns formatted partition value
- [X] T033 Add partition format token lookup table in `cli/internal/partition/formats.go`: maps all 5 spec tokens to Go `time.Format` strings; returns error for any other value
- [X] T034 Extend `Config` in `cli/internal/config/config.go`: add `Partition *partition.Partition` field; parse `-p` flag using `partition.Parse`; add validation to `Validate()`
- [X] T035 Extend `Config` unit tests in `cli/internal/config/config_test.go`: valid `-p DATEMOD=yyyy-mm-dd` sets partition; valid `-p DATERUN=yyyymmdd` sets partition; invalid format token returns error; missing `=` in `-p` value returns error
- [X] T036 Extend `OutputManager` in `cli/internal/output/manager.go` for partitioned mode: DATERUN — compute `partition.Resolve(startedAt)` once and route all rows to a single partitioned file; DATEMOD — call `partition.Resolve(parseModifiedAt(modifiedAt))` per row, maintain `map[string]*openHandle` for lazy file creation; write header once per file; `Close()` iterates map and flushes/closes all handles; partial cleanup on startup scans `{outputDir}/{tableName}/` tree for any matching files from prior runs; verify `go test -race ./cli/...` passes

**Checkpoint**: US4 complete. Partitioned output organises files into date-labelled subdirectories correctly for both DATERUN and DATEMOD modes.

---

## Phase 7: User Story 5 — Configure Output Location and Row Batch Size (Priority: P4)

**Goal**: `-o /path/to/dir` writes all output under the specified directory (created if absent). `-r 500` fetches 500 rows per API page. Both flags override their defaults cleanly without affecting other behaviour.

**Independent Test**: `wd-extract -o /tmp/testout` writes files under `/tmp/testout/`. `wd-extract -r 500` with a 1200-row table creates 3 pages (500 + 500 + 200) and produces 1200 data rows in a single file. `-r 0` and `-r 10001` exit 1.

### Tests for User Story 5

- [X] T037 Extend `config_test.go`: `-o /abs/path` sets `Config.OutputDir` correctly; `-r 500` sets `Config.RowCount = 500`; relative path for `-o` is accepted as-is (creation is deferred to OutputManager)

### Implementation for User Story 5

- [X] T038 Verify `RowCount` and `OutputDir` fields are already in `Config` from T012 (they are — confirm tests pass); confirm `OutputManager` already passes `outputDir` to `LocalWriter.Create` so files land under the correct directory; confirm `LocalWriter.Create` calls `os.MkdirAll` so non-existent directories are created
- [X] T039 Run full integration smoke test: build `wd-extract`; run against `httptest.NewServer` fixture serving 3 pages of 500 rows for table `account`; assert output file at `{outputDir}/account-*.csv` contains 1500 data rows + 1 header row; assert exit code 0; verify `go test -race ./cli/...` passes

**Checkpoint**: US5 complete. All configuration flags work independently and in combination.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Goreleaser validation, documentation verification, final lint gate.

- [X] T040 [P] Add `USAGE` const and `flag.Usage` override in `cli/cmd/wd-extract/main.go` to print structured help output matching `contracts/cli-interface.md` format; verify `-h` prints all flags with descriptions and defaults
- [X] T041 [P] Validate `quickstart.md` against the built binary: run each command example from the quickstart against the mock server; confirm the described output matches actual behaviour
- [X] T042 [P] Run `golangci-lint run ./...` from `cli/`; fix any linting findings; run `go vet ./...`; ensure zero warnings
- [X] T043 Run `go test -race -count=1 ./cli/...` and confirm all tests pass; tag a test release (`git tag cli/v0.1.0-dev`) and run `goreleaser build --snapshot --clean` from repo root to verify binaries are produced for all 6 platform targets

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — **BLOCKS all user stories**
- **Phase 3 (US1 P1)**: Depends on Phase 2 — delivers the MVP
- **Phase 4 (US2 P2)**: Depends on Phase 2 (foundational) + Phase 3 (uses same extraction loop)
- **Phase 5 (US3 P3)**: Depends on Phase 2 (Formatter interface from T005); parallelisable with Phase 4
- **Phase 6 (US4 P3)**: Depends on Phase 3 (OutputManager from T016, T017); depends on Phase 5 (format extension from T029)
- **Phase 7 (US5 P4)**: Depends on Phase 3 (Config fields already in place); lightweight verification phase
- **Phase 8 (Polish)**: Depends on all story phases complete

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2 — no story dependencies
- **US2 (P2)**: Depends on US1 (extends same `run()` loop)
- **US3 (P3)**: Depends on Phase 2 only (Formatter interface) — parallelisable with US2
- **US4 (P3)**: Depends on US1 (OutputManager) and US3 (format extension) — partition is an output concern
- **US5 (P4)**: Depends on US1 (Config and OutputManager already have these fields) — primarily a verification story

### Within Each User Story

- Test tasks marked [P] within a phase can be written simultaneously
- Implementation tasks follow: Config extension → API layer → Formatter → OutputManager → main.go wiring
- Run `go test -race ./cli/...` after each story checkpoint before moving to the next

### Parallel Opportunities

- **Phase 1**: T003 and T004 in parallel with T001/T002 completion
- **Phase 2**: T006, T007, T008 can all proceed in parallel once T005 is defined (all different packages)
- **Phase 3**: T009, T010, T011 (test files) can be written in parallel; T013 and T014 can be written in parallel
- **Phase 5**: T026 and T027 in parallel (separate files)
- **Phase 8**: T040, T041, T042 in parallel

---

## Parallel Example: User Story 1 (MVP)

```
# Write tests in parallel (different files):
T009  cli/internal/config/config_test.go
T010  cli/internal/wdapi/extract_test.go
T011  cli/internal/format/format_test.go

# Implement in parallel (different packages):
T013  cli/internal/wdapi/extract.go  (DJSONPage + PageExtract)
T014  cli/internal/wdapi/tables.go   (DiscoverTables)
T015  cli/internal/format/csv.go     (csvFormatter)

# Then sequential (dependencies):
T012  cli/internal/config/config.go  (extend with RowCount, OutputDir, Format)
T016  cli/internal/output/manager.go (OutputManager)
T017  cli/internal/output/manager_test.go
T018  cli/cmd/wd-extract/main.go     (run() function)
T019  Verify build + tests pass
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T004)
2. Complete Phase 2: Foundational (T005–T008) — **CRITICAL GATE**
3. Complete Phase 3: User Story 1 (T009–T019)
4. **STOP and VALIDATE**: Build binary, run against mock server, confirm all-tables CSV extraction end-to-end
5. **SHIP**: This is a fully working `wd-extract` binary

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 (P1) → all-tables CSV extraction → **first deployable binary**
3. US2 (P2) → selective table extraction → covers recovery workflows
4. US3 (P3) → TSV + PIPE format support → unlocks more pipeline integrations
5. US4 (P3) → date partitioning → enables data lake / Hive-style loading
6. US5 (P4) → output dir + batch size config → operational polish
7. Polish → goreleaser release → GitHub Releases binary available

---

## Notes

- `[P]` tasks touch different files and have no incomplete-task dependencies within their phase
- `[USn]` label maps each task to its user story for traceability
- Constitution Principle V is non-negotiable: every `[USn]` phase includes test tasks
- Each story checkpoint includes `go test -race ./cli/...` — never skip the race detector
- API key must never appear in any test log, assertion message, or file output
- The mock server approach (`httptest.NewServer`) keeps tests hermetic — no real API needed in CI
