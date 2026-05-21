# Implementation Plan: Data CLI (wd-extract)

**Branch**: `011-data-cli` | **Date**: 2026-05-18 | **Spec**: [spec.md](spec.md)  
**Jira**: WOOD-011 (Task under Epic: Data Engineering)  
**Input**: Feature specification from `/specs/011-data-cli/spec.md`

## Summary

`wd-extract` is a standalone binary that incrementally extracts changed data from the WoodenDollars Data API and writes it to local files. It is a new top-level Go module (`cli/`) with no shared code with the existing API module. The tool handles multi-page pagination, three output formats (CSV/TSV/PIPE), date-partitioned output, automatic retry on transient failures, and sequential extraction of multiple tables. It is distributed as a cross-platform binary via GitHub Releases using goreleaser.

## Technical Context

**Language/Version**: Go 1.26+ (matches existing project standard, enforced in `go.mod`)  
**Primary Dependencies**: stdlib only — `net/http`, `encoding/csv`, `encoding/json`, `flag`, `log/slog`, `os`, `bufio`, `time`  
**Storage**: Local filesystem (output files); no database  
**Testing**: `go test -race ./...`; unit tests with `net/http/httptest` mock server  
**Target Platform**: Linux, macOS, Windows (amd64 + arm64 via goreleaser cross-compilation)  
**Project Type**: CLI tool (standalone binary)  
**Performance Goals**: Throughput bounded by API rate limits; memory O(1) per page (streaming, no full-table buffering)  
**Constraints**: Must handle tables with millions of rows without OOM; DATEMOD mode opens at most ~365 files simultaneously (safe for default OS file descriptor limits)  
**Scale/Scope**: Single-binary tool; no server, no daemon, no persistent state beyond output files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Applicable? | Assessment |
|-----------|-------------|------------|
| I. Multi-Tenant Isolation | No | CLI is a read-only API consumer. Tenant isolation is enforced by the API; the CLI passes the API key which is already scoped to a single tenant. No DB access. |
| II. Transaction Integrity | No | Read-only extraction; no financial operations. |
| III. Data Integrity & Auditability | Indirect | The CLI reads from `*_log` tables via the API, which relies on the `modified_at = clock_timestamp()` convention. The CLI itself makes no writes to platform data. The correct use of `modified_at` for windowed extraction is enforced by the API, not the CLI. |
| IV. API-First Architecture | Yes ✅ | CLI accesses data exclusively via the HTTP API. No direct database access. |
| V. Test Discipline | Yes ✅ | Table-driven unit tests; `-race` flag; `httptest.NewServer` for pagination and error path coverage. |
| VI. Observability & Configuration | Yes ✅ | Structured logging via `log/slog` (stdlib) with a custom handler outputting `[wd-extract] …` lines to stderr; API key never logged; config validated at startup with fail-fast behaviour. |
| VII. Security & Least Privilege | Yes ✅ | API key treated as a secret (never logged, never written to files). TLS enforced (https default). Filesystem access limited to the output directory. |
| VIII. Configuration Management | No | Client-side binary; no `platform_config` table involvement. |

**Result**: No constitution violations. ✅

## Project Structure

### Documentation (this feature)

```text
specs/011-data-cli/
├── plan.md              # This file
├── research.md          # Phase 0 research decisions
├── data-model.md        # Phase 1 data types and interfaces
├── quickstart.md        # Phase 1 getting-started guide
├── contracts/
│   └── cli-interface.md # Phase 1 CLI flag + exit code contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
.goreleaser.yml                         # Release config: builds wd-extract for all platforms

cli/                                    # New top-level Go module
├── go.mod                              # module github.com/nickwhiteley/woodendollars/cli
├── go.sum
├── cmd/
│   └── wd-extract/
│       └── main.go                     # Entry point: flag parsing, config validation, run loop
└── internal/
    ├── config/
    │   ├── config.go                   # Config struct, flag parsing, env resolution, validation
    │   └── config_test.go
    ├── log/
    │   └── handler.go                  # Custom slog.Handler: outputs [wd-extract] lines to stderr
    ├── wdapi/
    │   ├── client.go                   # HTTP client with auth header, timeout, retry wrapper
    │   ├── tables.go                   # DiscoverTables → GET /v1/data/extract
    │   ├── extract.go                  # PageExtract → GET /v1/data/extract/{table}, DJSONPage type
    │   └── extract_test.go             # httptest.NewServer-based pagination + retry tests
    ├── format/
    │   ├── formatter.go                # Formatter interface (WriteHeader, WriteRow, Flush)
    │   ├── csv.go                      # CSV: comma delimiter via encoding/csv
    │   ├── tsv.go                      # TSV: tab delimiter
    │   ├── pipe.go                     # PIPE: | delimiter
    │   └── format_test.go              # Table-driven: quoting, null handling, delimiter escaping
    ├── partition/
    │   ├── partition.go                # Partition struct, spec-to-Go format mapping, path resolution
    │   └── partition_test.go           # Table-driven: all 5 format tokens, DATEMOD vs DATERUN paths
    └── output/
        ├── writer.go                   # Writer interface + LocalWriter (os.MkdirAll + os.Create)
        ├── manager.go                  # OutputManager: lazy file creation, header-once, DATEMOD routing
        └── manager_test.go             # Partitioned write, cleanup, header-once invariant
```

**Structure Decision**: New `cli/` top-level module (separate `go.mod`) to avoid coupling to the API module. All packages under `cli/internal/` to prevent accidental external imports. Binary entry point in `cli/cmd/wd-extract/main.go`, consistent with existing `api/cmd/` layout.

## Complexity Tracking

No constitution violations. No complexity justification required.
