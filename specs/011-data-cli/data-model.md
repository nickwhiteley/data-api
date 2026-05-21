# Data Model: Data CLI (wd-extract)

**Feature**: 011-data-cli  
**Date**: 2026-05-18

This document describes the key types and their relationships within the `wd-extract` binary. There is no database; all state is in-process for the duration of a single extraction run.

---

## Core Types

### Config

Holds all resolved runtime configuration. Populated from environment variables and CLI flags at startup. Validated before any API calls are made.

```
Config
├── APIKey       string       // API authentication credential (never logged)
├── APIURL       string       // Base URL for the WoodenDollars API
├── Table        string       // Single table name (-t flag); empty if not set
├── Tables       []string     // Parsed list of table names (-tables flag); nil if not set
├── Format       OutputFormat // CSV | TSV | PIPE
├── OutputDir    string       // Resolved absolute path for output files
├── RowCount     int          // Rows per API page request (1–10000)
└── Partition    *Partition   // nil if no -p flag provided
```

**Validation rules**:
- `APIKey` must be non-empty
- `APIURL` must be a valid absolute URL with https scheme
- `Format` must be one of the three allowed values
- `RowCount` must be 1–10000
- Only one of `Table` / `Tables` may be set (mutually exclusive with all-tables mode)
- `Partition` must have a valid mode (DATEMOD or DATERUN) and a valid format string

---

### OutputFormat

Enumeration of the three supported output formats.

```
OutputFormat: "CSV" | "TSV" | "PIPE"
```

Maps to file extension: `CSV→.csv`, `TSV→.tsv`, `PIPE→.txt`

---

### Partition

Describes a date-based partitioning strategy for output files.

```
Partition
├── Mode      string   // "DATEMOD" or "DATERUN"
├── SpecFmt   string   // User-supplied format token (e.g. "yyyy-mm-dd")
└── GoFmt     string   // Derived Go time.Format string (e.g. "2006-01-02")
```

**Allowed SpecFmt values and their Go equivalents**:

| SpecFmt        | GoFmt              | Example output |
|----------------|-------------------|----------------|
| `yyyy-mm-dd`   | `2006-01-02`      | `2026-05-18`   |
| `yyyymmdd`     | `20060102`        | `20260518`     |
| `yyyy-mm-dd-hh`| `2006-01-02-15`   | `2026-05-18-10`|
| `yyyy`         | `2006`            | `2026`         |
| `yyyy-mm`      | `2006-01`         | `2026-05`      |

---

### DJSONPage

Represents a single page of API response from `GET /v1/data/extract/{table}`.

```
DJSONPage
├── ExecutionID  string        // UUID of the extraction execution (required for page 2+)
├── Columns      []string      // Ordered list of column names (header row)
└── Rows         [][]any       // Data rows; each cell decoded with UseNumber()
```

**Cell type invariants** (after `json.Decoder.UseNumber()`):
- Text values → `string`
- Numeric values → `json.Number` (preserves exact representation)
- Boolean values → `bool`
- Null values → `nil`

---

### TableExtraction

Represents the state of extracting one table during a run. Created per-table, discarded after the table completes or fails.

```
TableExtraction
├── TableName    string        // e.g. "account"
├── StartedAt    time.Time     // Client-side timestamp when extraction began
├── PageNum      int           // Current page number (1-based)
├── ExecutionID  string        // Populated after first page response
├── TotalRows    int           // Running count of rows received
├── OutputFiles  []string      // Resolved paths of files written
└── Err          error         // Non-nil if extraction failed
```

---

### ExtractionRun

Top-level result of a complete `wd-extract` invocation. Used to determine the final exit code.

```
ExtractionRun
├── Tables    []TableExtraction   // Results for each extracted table (in order)
├── StartedAt time.Time
└── EndedAt   time.Time
```

**Exit code rule**: exit 0 iff all `TableExtraction.Err == nil`.

---

## Interface Contracts

### Formatter

Abstracts the output encoding (CSV, TSV, PIPE). Implemented by `csvFormatter`, `tsvFormatter`, `pipeFormatter`.

```
Formatter interface
├── WriteHeader(cols []string) error
├── WriteRow(row []string) error
└── Flush() error
```

All three implementations delegate to `encoding/csv` with the appropriate delimiter rune.

---

### Writer

Abstracts the destination filesystem. `LocalWriter` is the only implementation in v1.

```
Writer interface
└── Create(path string) (io.WriteCloser, error)
```

`LocalWriter.Create` calls `os.MkdirAll` on the parent directory, then `os.Create`. Future implementations (e.g. `S3Writer`) satisfy the same interface without touching the extraction loop.

---

### OutputManager

Manages the mapping from partition value to open file + formatter. Handles lazy file creation, header-once logic, and coordinated close.

```
OutputManager
├── New(writer Writer, tableName string, startedAt time.Time, format OutputFormat, partition *Partition) *OutputManager
├── WriteRow(row []string, modifiedAt string) error    // routes to correct file; modifiedAt only used for DATEMOD
├── Files() []string                                    // returns sorted list of files written
└── Close() error                                       // flushes and closes all open handles
```

For non-partitioned output, `OutputManager` holds a single file. For DATEMOD, it holds one file per distinct partition value encountered.

---

## Output File Path Formulae

### Non-partitioned

```
{outputDir}/{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}
```

Example: `./account-20260518-103045.csv`

### Partitioned (DATERUN)

```
{outputDir}/{tableName}/{runDateValue}/{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}
```

Example: `./account/2026-05/account-20260518-103045.csv`

### Partitioned (DATEMOD)

```
{outputDir}/{tableName}/{modDateValue}/{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}
```

Example: `./account/2026-04/account-20260518-103045.csv`  
         `./account/2026-05/account-20260518-103045.csv`

Note: The filename timestamp (`-{YYYYMMDD}-{HHMMSS}`) is always the extraction start time, not the partition date. Multiple files in different subdirectories may share the same filename; they are distinguished by their parent directory.
