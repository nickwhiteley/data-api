# CLI Interface Contract: wd-extract

**Feature**: 011-data-cli  
**Date**: 2026-05-18  
**Binary**: `wd-extract`

This document is the authoritative contract for the `wd-extract` command-line interface. Any change to flags, environment variables, exit codes, or output behaviour is a breaking change and requires a version increment.

---

## Synopsis

```
wd-extract [flags]
```

---

## Environment Variables

| Variable      | Required | Default                        | Description                                  |
|---------------|----------|-------------------------------|----------------------------------------------|
| `WD_API_KEY`  | Yes*     | —                              | API key with `data_engineer` scope. *Required unless `-k` is provided. |
| `WD_API_URL`  | No       | `https://woodendollars.com`    | Override the API base URL (e.g. for staging). |

---

## Flags

All flags use single-dash prefix (Go stdlib `flag` style). Flags and their env var equivalents are resolved in this order: flag value → env var → default.

| Flag              | Type   | Default                     | Description |
|-------------------|--------|-----------------------------|-------------|
| `-k <key>`        | string | `$WD_API_KEY`               | API key. Takes precedence over `WD_API_KEY`. |
| `-u <url>`        | string | `$WD_API_URL` or `https://woodendollars.com` | API base URL override. |
| `-t <table>`      | string | —                            | Extract a single named table. Mutually exclusive with `-tables`. |
| `-tables <list>`  | string | —                            | Comma-separated list of table names to extract. Mutually exclusive with `-t`. |
| `-f <format>`     | string | `CSV`                        | Output format. Allowed values: `CSV`, `TSV`, `PIPE`. |
| `-o <dir>`        | string | `.` (current directory)      | Output directory. Created if it does not exist. |
| `-r <n>`          | int    | `1000`                       | Rows per API page request. Range: 1–10000. |
| `-p <partition>`  | string | —                            | Partition pattern. Format: `DATEMOD={fmt}` or `DATERUN={fmt}`. |
| `-h` / `-help`    | flag   | —                            | Print help text and exit 0. |

### Partition format tokens

The `{fmt}` component of `-p` must be one of:

| Token          | Example value  |
|----------------|----------------|
| `yyyy-mm-dd`   | `2026-05-18`   |
| `yyyymmdd`     | `20260518`     |
| `yyyy-mm-dd-hh`| `2026-05-18-10`|
| `yyyy`         | `2026`         |
| `yyyy-mm`      | `2026-05`      |

Any other token is an error (see Exit Codes).

### Mutual exclusivity

- `-t` and `-tables` are mutually exclusive. Providing both is an error.
- If neither `-t` nor `-tables` is provided, all tables are extracted (discovered from the API).

---

## Output Streams

| Stream   | Content |
|----------|---------|
| **Files** | Extracted data written to the output directory as structured text files. No data is written to stdout. |
| **stderr** | All diagnostic output: startup config summary, per-table progress (start, page count, row count, completion), errors, and retry attempts. |
| **stdout** | Nothing. Reserved for future use (e.g. machine-readable JSON summary). |

---

## Output File Naming

```
{outputDir}/{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}
```

Where:
- `{tableName}` is the exact table name returned by the API (e.g. `account`)
- `{YYYYMMDD}-{HHMMSS}` is the local wall-clock time when the tool begins extracting that table
- `{ext}` is `.csv` (CSV), `.tsv` (TSV), or `.txt` (PIPE)

### With partitioning

```
{outputDir}/{tableName}/{partitionValue}/{tableName}-{YYYYMMDD}-{HHMMSS}.{ext}
```

For DATEMOD, multiple files may be created under different `{partitionValue}` directories in the same run.

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | All requested tables extracted successfully. |
| `1`  | One or more tables failed, OR a configuration/startup error occurred. |

A configuration error (missing API key, invalid flag value) exits with code `1` immediately before any extraction begins. Partial extraction failure (one table fails in all-tables mode) also exits with code `1` after all remaining tables have been attempted.

---

## Stderr Progress Format

Each significant event is logged on its own line. The exact format may change between minor versions; callers MUST NOT parse stderr for automation (use exit codes instead).

```
[wd-extract] starting extraction: 5 tables, format=CSV, output=./output
[wd-extract] table account: starting (page 1)
[wd-extract] table account: page 2 (1000 rows so far)
[wd-extract] table account: complete (1847 rows, 2 pages, file: ./output/account-20260518-103045.csv)
[wd-extract] table transaction: starting (page 1)
[wd-extract] table transaction: retry 1/3 (429 Too Many Requests, waiting 1s)
[wd-extract] table transaction: complete (5203 rows, 6 pages, file: ./output/transaction-20260518-103046.csv)
[wd-extract] extraction complete: 5/5 tables succeeded (exit 0)
```

On failure:
```
[wd-extract] table audit_log: ERROR: access denied (403 Forbidden) — skipping
[wd-extract] extraction complete: 4/5 tables succeeded (exit 1)
```

---

## Error Messages

Configuration errors print a single descriptive line to stderr and exit 1 immediately:

```
wd-extract: API key is required. Set WD_API_KEY or use -k.
wd-extract: invalid format "EXCEL": must be CSV, TSV, or PIPE.
wd-extract: invalid partition pattern "DATEMOD=yyyy-dd": allowed formats are yyyy, yyyy-mm, yyyy-mm-dd, yyyymmdd, yyyy-mm-dd-hh.
wd-extract: flags -t and -tables are mutually exclusive.
```

---

## Behaviour Guarantees

1. **Idempotent restarts**: If interrupted mid-extraction, restarting produces clean output with no partial or duplicate rows for completed tables. Partially written files are deleted at startup before re-extraction begins.
2. **Header exactly once**: Column headers appear as the first row of each output file, written exactly once regardless of page count or partition count.
3. **Null fields**: JSON null values are written as empty fields (consecutive delimiters with nothing between them). The string `"NULL"` is never written as a sentinel.
4. **Delimiter safety**: Any field value containing the delimiter character is quoted per RFC 4180 (CSV/TSV) or equivalent convention (PIPE).
5. **Sequential extraction**: Tables are extracted one at a time. Starting a new table extraction while another is in progress is not possible.
6. **Non-interference**: A failure on one table in all-tables mode does not affect output files of other tables.
