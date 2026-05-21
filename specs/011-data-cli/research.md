# Research: Data CLI (wd-extract)

**Phase**: 0 — Pre-design research  
**Feature**: 011-data-cli  
**Date**: 2026-05-18

## Decision Log

---

### D-001: CLI Flag Library

**Decision**: Go stdlib `flag` package  
**Rationale**: `wd-extract` is a single-command binary with ~8 flat flags and no subcommands. `cobra` is designed for multi-command CLIs (e.g. `git`, `kubectl`) and adds ~3–5 MB to binary size plus significant boilerplate. The stdlib `flag` package covers all required flag patterns (`-t value`, `-f CSV`, `-h`), generates help text automatically, and adds zero external dependencies.  
**Alternatives considered**:
- `cobra` — rejected; overkill for a single-command tool, adds binary weight and structural overhead
- `urfave/cli/v2` — rejected; external dependency for marginal cosmetic benefit

---

### D-002: HTTP Client and Retry Strategy

**Decision**: `net/http.Client` with a custom `retryDo()` helper (3 attempts, exponential backoff: 1s → 2s → 4s)  
**Rationale**: The retry requirements are simple and fixed: 3 attempts, triggered on 429 and 5xx status codes or connection errors. No external retry library adds meaningful value over a 15-line loop. The default `http.Client` timeout is set to 30 seconds per request.  
**Alternatives considered**:
- `hashicorp/go-retryablehttp` — rejected; external dep, overkill for a simple bounded retry policy

---

### D-003: DJSON Row Unmarshaling

**Decision**: Unmarshal `rows` as `[][]any` with `json.Decoder.UseNumber()` enabled  
**Rationale**: `UseNumber()` tells the JSON decoder to produce `json.Number` values instead of `float64` for all JSON numbers. This preserves exact integer representations for large UUIDs and amounts (which lose precision when cast to `float64`). `json.Number.String()` then returns the original decimal representation without scientific notation.  
**Type-to-string mapping**:
- `json.Number` → `n.String()` (exact decimal; no scientific notation)
- `string` → value as-is
- `bool` → `"true"` / `"false"`
- `nil` (JSON null) → `""` (empty string — standard CSV convention)
- Fallback (unexpected type) → `fmt.Sprintf("%v", v)`

**Alternatives considered**:
- `[][]json.RawMessage` with per-cell re-unmarshal — rejected; double-parsing with no benefit since `UseNumber()` already handles precision
- `[][]float64` — rejected; loses precision for integers >2^53

---

### D-004: DATEMOD Multi-File Output

**Decision**: `map[string]*openHandle` keyed by partition path, with lazy file creation and header-once-per-file logic  
**Rationale**: DATEMOD mode routes individual rows to different output files based on their `modified_at` value. The number of distinct partition values per table extraction is bounded (≤ 365 for `yyyy-mm-dd`, ≤ 12 for `yyyy-mm`). Keeping all files open simultaneously is safe. Each file gets a `bufio.Writer` wrapper to reduce syscall overhead. Headers are written exactly once when each file is first created (checked via file size on open). All file handles are explicitly closed after the table extraction completes.  
**Alternatives considered**:
- Buffer all rows in memory, group by partition, then write — rejected; unbounded memory use on large tables
- Re-open file for every row append — rejected; filesystem overhead, no atomicity guarantees

---

### D-005: Module Structure and Goreleaser

**Decision**: New `cli/` directory with its own `go.mod` (`module github.com/nickwhiteley/woodendollars/cli`). `.goreleaser.yml` placed at repo root, referencing `./cli/cmd/wd-extract` as the build entry point.  
**Rationale**: Matches the existing `github.com/nickwhiteley/woodendollars/api` naming convention. Separate `go.mod` prevents the CLI from being accidentally imported as a library and allows independent versioning. Root-level goreleaser is the monorepo standard — one config drives all release artifacts.  
**Goreleaser targets**: linux/darwin/windows × amd64/arm64. Archives as `.tar.gz` (`.zip` for Windows). SHA256 checksum file included.  
**Alternatives considered**:
- `cli/.goreleaser.yml` — rejected; root placement is standard for monorepos and avoids CI path configuration complexity
- Single monolithic `go.mod` at repo root — rejected; would expose `cli/internal/` to the API module and create coupling

---

### D-006: Formatter Interface Approach

**Decision**: A `Formatter` interface with three concrete implementations (CSV, TSV, PIPE) wrapping `encoding/csv` with a configurable delimiter  
**Rationale**: `encoding/csv` handles correct RFC 4180 quoting (double-quote wrapping, internal quote escaping) for all three formats when the delimiter is changed. PIPE-delimited shares CSV's quoting conventions in practice. A thin interface (`WriteHeader`, `WriteRow`, `Flush`) allows the extraction loop to be format-agnostic.  
**Alternatives considered**:
- Separate hand-rolled quoting for each format — rejected; `encoding/csv` already handles edge cases correctly

---

### D-007: Partial File Cleanup on Restart

**Decision**: On startup, before extracting a table, scan the output directory (and partition subdirectories) for any file whose name matches `{table-name}-*.{ext}`. If found, delete it before creating the new output file.  
**Rationale**: Keeps restart behaviour clean and predictable. The API extraction cursor only advances when an extraction reaches `status=completed` (final page). A crash leaves the cursor at the prior position, so the re-run correctly re-extracts from the same window. The partial file is a leftover artefact that must not be appended to.  
**Scope**: Only files matching the exact table name prefix and current format extension are deleted. Files for other tables are untouched.

---

### D-008: No Code Reuse from api/ Module

**Decision**: Re-implement DJSON types and HTTP response parsing directly in `cli/internal/wdapi/`  
**Rationale**: The `cli/` module cannot import `api/internal/...` packages (Go's `internal` visibility rule prevents cross-module internal imports). The DJSON response struct is a simple 3-field type that takes ~10 lines to define. No shared library is warranted at this stage.  
**Alternatives considered**:
- Moving DJSON types to `api/pkg/...` for sharing — rejected; introduces coupling, premature abstraction for now
