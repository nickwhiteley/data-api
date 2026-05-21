# data-api

Standalone data extraction library and CLI for products built on the woodendollars database schema.

This repository contains two components:

| Component | Module | Purpose |
|---|---|---|
| Server library | `github.com/nickwhiteley/data-api` | Go HTTP handlers for incremental data extraction via `_log` tables |
| CLI | `github.com/nickwhiteley/data-api/cli` | `wd-extract` binary for downloading data to CSV/TSV files |

Every merge to `main` publishes a new release automatically — binary releases for the CLI and a tagged Go module version for the library.

---

## Server library

The library mounts five routes on your chi router, implementing window extraction, current-state snapshots, table discovery, execution history, and cursor reset.

### Requirements

- Go 1.26+
- PostgreSQL with the `wd` schema and migrations from `migrations/` applied
- chi router (`github.com/go-chi/chi/v5`)
- pgx/v5 connection pool

### Install

```bash
go get github.com/nickwhiteley/data-api@latest
```

### Implement the two required interfaces

The handler needs to read auth context (scope, user ID, tenant ID) and configuration from the host application. Implement two interfaces:

```go
package main

import (
    "context"
    dataapi "github.com/nickwhiteley/data-api/handler"
)

// myAuth wraps your middleware's context keys.
type myAuth struct{}

func (myAuth) Scope(ctx context.Context) string    { return myMiddleware.ScopeFromCtx(ctx) }
func (myAuth) UserID(ctx context.Context) string   { return myMiddleware.UserIDFromCtx(ctx) }
func (myAuth) TenantID(ctx context.Context) string { return myMiddleware.TenantIDFromCtx(ctx) }

// myConfig reads from your live config source.
type myConfig struct{ cfg *myapp.Config }

func (c myConfig) SafetyLagSeconds() int { return c.cfg.DataExtractSafetyLagSeconds }
```

### Register routes

```go
import (
    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    dataapi "github.com/nickwhiteley/data-api/handler"
)

func setupRouter(pool *pgxpool.Pool, auth myAuth, cfg myConfig) http.Handler {
    r := chi.NewRouter()
    // ... your auth middleware here ...
    r.Group(func(r chi.Router) {
        r.Use(yourAuthMiddleware)
        dataapi.NewHandler(pool, auth, cfg).RegisterRoutes(r)
    })
    return r
}
```

### Routes registered

All routes are prefixed with whatever path your router mounts them under.

| Method | Path | Description |
|---|---|---|
| `GET` | `/data/extract` | Discover extractable tables |
| `GET` | `/data/extract/{table}` | Window extraction (incremental changes) |
| `GET` | `/data/extract/{table}/current` | Current-state snapshot (2/day/table limit) |
| `POST` | `/data/extract/{table}/reset` | Reset extraction cursor |
| `GET` | `/data/executions` | List execution history |

All extraction endpoints require an API key with `data_engineer` scope. The tier check requires `platform_usage_tier.data_api_enabled = true` for the tenant (Enterprise tier).

### Database migrations

Run both migration files from `migrations/` against your PostgreSQL database before starting the application:

```bash
# Using goose (raw SQL mode):
goose -dir migrations postgres "$DATABASE_URL" up
```

Migrations create:
- `wd.data_extraction_execution` — execution tracking and cursor state
- `wd.data_extraction_deny` — platform-wide table deny list
- `data_api_enabled` column on `wd.platform_usage_tier`
- `data_engineer` role in `wd.user_role`

### AuthContext interface

```go
type AuthContext interface {
    Scope(ctx context.Context) string    // API key scope, e.g. "data_engineer"
    UserID(ctx context.Context) string   // Authenticated user UUID
    TenantID(ctx context.Context) string // Tenant UUID
}
```

### DataConfig interface

```go
type DataConfig interface {
    SafetyLagSeconds() int // Seconds subtracted from now() for default end_at
}
```

---

## CLI: wd-extract

`wd-extract` downloads data from the Data API to local CSV, TSV, or pipe-delimited files. Cursor state is managed server-side — re-running the tool picks up from where the last run finished.

### Install

**Download binary (recommended):**

```bash
# Linux x86-64
gh release download --repo nickwhiteley/data-api --pattern 'wd-extract_linux_amd64.tar.gz' -D /tmp
tar -xzf /tmp/wd-extract_linux_amd64.tar.gz -C /usr/local/bin
```

See the [Releases page](https://github.com/nickwhiteley/data-api/releases) for all platforms:

| Platform | Archive |
|---|---|
| Linux x86-64 | `wd-extract_linux_amd64.tar.gz` |
| Linux ARM64 | `wd-extract_linux_arm64.tar.gz` |
| macOS x86-64 | `wd-extract_darwin_amd64.tar.gz` |
| macOS ARM64 | `wd-extract_darwin_arm64.tar.gz` |
| Windows x86-64 | `wd-extract_windows_amd64.zip` |

**Install with Go:**

```bash
go install github.com/nickwhiteley/data-api/cli/cmd/wd-extract@latest
```

### Usage

```bash
wd-extract [flags]
```

| Flag | Env | Default | Description |
|---|---|---|---|
| `-k` | `WD_API_KEY` | required | API key with `data_engineer` scope |
| `-u` | `WD_API_URL` | `https://woodendollars.com` | Base URL of the Data API |
| `-o` | | `.` | Output directory |
| `-f` | | `CSV` | Output format: `CSV`, `TSV`, or `PIPE` |
| `-r` | | `1000` | Rows per page (1–10000) |
| `-t` | | | Extract a single table by name |
| `-tables` | | | Comma-separated list of tables |
| `-p` | | | Partition: `DATERUN=yyyy-mm-dd` or `DATEMOD=yyyy-mm` |

### Examples

```bash
# Extract all available tables to CSV
WD_API_KEY=your_key wd-extract -u https://api.example.com -o /data/export

# Extract specific tables
WD_API_KEY=your_key wd-extract -t account -o /data/export

# Partition output by modification date (monthly)
WD_API_KEY=your_key wd-extract -p DATEMOD=yyyy-mm -o /data/export

# Partition by run date (daily), TSV format
WD_API_KEY=your_key wd-extract -p DATERUN=yyyy-mm-dd -f TSV -o /data/export
```

Output files are named `{table}-{YYYYMMDD}-{HHMMSS}.{ext}`. When partitioning, files go into subdirectories under the output path.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | All tables extracted successfully |
| `1` | One or more tables failed (others continue in all-tables mode) |

---

## Development

```bash
# Library
cd /path/to/data-api
go build ./...
go test -race ./...

# CLI
cd cli
go build ./cmd/wd-extract
go test -race ./...
```

---

## Releases

Every merge to `main` automatically:
- Tags a new patch version of the library (importable as a Go module)
- Builds and publishes `wd-extract` binaries for all platforms via goreleaser
