# Quickstart: wd-extract

A data engineer can complete their first extraction in under 5 minutes following this guide.

---

## Prerequisites

- A WoodenDollars API key with the `data_engineer` scope (obtain from the Tenant Administration panel)
- The `wd-extract` binary downloaded from [GitHub Releases](https://github.com/nickwhiteley/woodendollars/releases) for your platform

---

## 1. Install

**macOS (Apple Silicon)**
```bash
curl -sL https://github.com/nickwhiteley/woodendollars/releases/latest/download/wd-extract_darwin_arm64.tar.gz | tar -xz
mv wd-extract /usr/local/bin/
```

**macOS (Intel)**
```bash
curl -sL https://github.com/nickwhiteley/woodendollars/releases/latest/download/wd-extract_darwin_amd64.tar.gz | tar -xz
mv wd-extract /usr/local/bin/
```

**Linux (amd64)**
```bash
curl -sL https://github.com/nickwhiteley/woodendollars/releases/latest/download/wd-extract_linux_amd64.tar.gz | tar -xz
mv wd-extract /usr/local/bin/
```

**Windows**
Download `wd-extract_windows_amd64.zip` from GitHub Releases, extract, and add to your `PATH`.

---

## 2. Configure

Set your API key as an environment variable:

```bash
export WD_API_KEY=your-api-key-here
```

To persist across sessions, add this line to your `~/.bashrc` or `~/.zshrc`.

---

## 3. First Extraction

Extract all tables to the current directory:

```bash
wd-extract
```

You will see progress on stderr:
```
[wd-extract] starting extraction: 8 tables, format=CSV, output=.
[wd-extract] table account: starting (page 1)
[wd-extract] table account: complete (1847 rows, 2 pages, file: ./account-20260518-103045.csv)
...
[wd-extract] extraction complete: 8/8 tables succeeded (exit 0)
```

---

## 4. Common Patterns

### Extract a single table
```bash
wd-extract -t transaction
```

### Extract a subset of tables
```bash
wd-extract -tables account,transaction,escrow
```

### Output as TSV to a specific directory
```bash
wd-extract -f TSV -o /data/exports
```

### Partition output by modification date (monthly, for data lake loading)
```bash
wd-extract -p DATEMOD=yyyy-mm -o /data/lake
```

This creates a directory structure like:
```
/data/lake/
└── transaction/
    ├── 2026-04/
    │   └── transaction-20260518-103045.txt
    └── 2026-05/
        └── transaction-20260518-103045.txt
```

### Partition by run date (for time-based ingestion pipelines)
```bash
wd-extract -p DATERUN=yyyy-mm-dd -o /data/daily
```

### Adjust page size for large tables
```bash
wd-extract -r 5000 -t transaction
```

### Target a staging environment
```bash
WD_API_URL=https://staging.woodendollars.com wd-extract -t account
```

---

## 5. Scheduling

To run nightly and extract all changed data, add a cron job:

```cron
0 2 * * * WD_API_KEY=your-key /usr/local/bin/wd-extract -o /data/warehouse/raw >> /var/log/wd-extract.log 2>&1
```

---

## 6. Help

```bash
wd-extract -h
```

Prints all available flags with descriptions and default values.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `API key is required` | `WD_API_KEY` not set | Export the variable or pass `-k` |
| `access denied (403)` | Key lacks `data_engineer` scope | Re-issue key with correct scope from Admin panel |
| `table not found (404)` | Table name typo or not available | Run with no flags to list all available tables |
| `too many requests (429)` | Rate limit hit | Retried automatically; if persistent, reduce `-r` |
| Exit code 1, some tables succeeded | At least one table failed | Check stderr for the specific table and error message |
