package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nickwhiteley/data-api/cli/internal/config"
	wdlog "github.com/nickwhiteley/data-api/cli/internal/log"
	"github.com/nickwhiteley/data-api/cli/internal/output"
	"github.com/nickwhiteley/data-api/cli/internal/wdapi"
)

func main() {
	cfg, err := config.ParseArgs(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "wd-extract: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "wd-extract: %v\n", err)
		os.Exit(1)
	}
}

// run executes the full extraction for the configured tables.
func run(cfg *config.Config) error {
	logger := wdlog.New()
	client := wdapi.NewClient(cfg.APIURL, cfg.APIKey)
	ctx := context.Background()

	tables, named, err := discoverOrSelect(ctx, client, cfg)
	if err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("starting extraction: %d tables, format=%s, output=%s",
		len(tables), cfg.Format, cfg.OutputDir))

	var failed int
	for _, table := range tables {
		if extractErr := extractTable(ctx, cfg, client, table); extractErr != nil {
			if named {
				// Named tables: 404 and other errors are fatal — the caller was explicit.
				var notFound *wdapi.TableNotFoundError
				if errors.As(extractErr, &notFound) {
					logger.Error(fmt.Sprintf("table %s: not found", table))
				} else {
					logger.Error(fmt.Sprintf("table %s: ERROR: %v", table, extractErr))
				}
				return extractErr
			}
			logger.Error(fmt.Sprintf("table %s: ERROR: %v — skipping", table, extractErr))
			failed++
		}
	}

	total := len(tables)
	if failed > 0 {
		logger.Info(fmt.Sprintf("extraction complete: %d/%d tables succeeded (exit 1)", total-failed, total))
		return fmt.Errorf("%d table(s) failed", failed)
	}
	logger.Info(fmt.Sprintf("extraction complete: %d/%d tables succeeded (exit 0)", total, total))
	return nil
}

// discoverOrSelect returns the list of tables to extract and whether they were
// named explicitly by the caller (true) or discovered from the API (false).
// Named selection skips the DiscoverTables API call.
func discoverOrSelect(ctx context.Context, client *wdapi.Client, cfg *config.Config) (tables []string, named bool, err error) {
	switch {
	case cfg.Table != "":
		return []string{cfg.Table}, true, nil
	case len(cfg.Tables) > 0:
		return cfg.Tables, true, nil
	default:
		discovered, discoverErr := wdapi.DiscoverTables(ctx, client)
		if discoverErr != nil {
			return nil, false, fmt.Errorf("discover tables: %w", discoverErr)
		}
		// api_key rotates on every extraction and is rarely useful in bulk runs.
		// It can still be extracted explicitly via -t api_key or -tables api_key.
		filtered := discovered[:0]
		for _, t := range discovered {
			if t != "api_key" {
				filtered = append(filtered, t)
			}
		}
		return filtered, false, nil
	}
}

// extractTable downloads all pages for one table and writes them to the output directory.
func extractTable(ctx context.Context, cfg *config.Config, client *wdapi.Client, table string) error {
	startedAt := time.Now()
	mgr := output.New(&output.LocalWriter{}, table, startedAt, cfg.Format, cfg.Partition, cfg.OutputDir)

	var (
		pageNum       int
		executionID   string
		headerDone    bool
		modifiedAtIdx = -1
	)

	for {
		pageNum++
		page, err := wdapi.PageExtract(ctx, client, wdapi.PageExtractInput{
			Table:       table,
			PageNum:     pageNum,
			ExecutionID: executionID,
			RowCount:    cfg.RowCount,
		})
		if err != nil {
			_ = mgr.Close()
			return fmt.Errorf("page %d: %w", pageNum, err)
		}

		if executionID == "" {
			executionID = page.ExecutionID
		}

		if !headerDone {
			if err := mgr.WriteHeader(page.Columns); err != nil {
				_ = mgr.Close()
				return fmt.Errorf("write header: %w", err)
			}
			headerDone = true
			// Find modified_at column index for DATEMOD partitioning.
			if cfg.Partition != nil && cfg.Partition.Mode == "DATEMOD" {
				for i, col := range page.Columns {
					if col == "modified_at" {
						modifiedAtIdx = i
						break
					}
				}
			}
		}

		for _, rawRow := range page.Rows {
			strRow := make([]string, len(rawRow))
			for i, cell := range rawRow {
				strRow[i] = cellToString(cell)
			}
			modifiedAt := ""
			if modifiedAtIdx >= 0 && modifiedAtIdx < len(strRow) {
				modifiedAt = strRow[modifiedAtIdx]
			}
			if err := mgr.WriteRow(strRow, modifiedAt); err != nil {
				_ = mgr.Close()
				return fmt.Errorf("write row: %w", err)
			}
		}

		if len(page.Rows) < cfg.RowCount {
			break
		}
	}

	if err := mgr.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	return nil
}

// cellToString converts an API cell value to its string representation.
// json.Number values are preserved as their exact decimal string.
// nil (null) becomes an empty string (FR-015).
func cellToString(v any) string {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
