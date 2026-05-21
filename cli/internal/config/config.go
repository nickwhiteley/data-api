package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nickwhiteley/data-api/cli/internal/format"
	"github.com/nickwhiteley/data-api/cli/internal/partition"
)

// Config holds all resolved runtime configuration for a wd-extract invocation.
type Config struct {
	APIKey    string
	APIURL    string
	RowCount  int
	OutputDir string
	Format    format.OutputFormat
	Table     string               // -t: extract a single named table
	Tables    []string             // -tables: extract a comma-separated list of tables
	Partition *partition.Partition // -p: optional partitioning spec
}

// ParseArgs resolves configuration from the given argument slice and environment lookup function.
// environ should be os.Getenv in production and a custom function in tests.
func ParseArgs(args []string, environ func(string) string) (*Config, error) {
	fs := flag.NewFlagSet("wd-extract", flag.ContinueOnError)
	fs.Usage = printUsage(fs)

	apiKey := fs.String("k", environ("WD_API_KEY"), "API key (overrides WD_API_KEY)")
	apiURL := fs.String("u", envOrDefault(environ, "WD_API_URL", "https://woodendollars.com"), "API base URL override")
	rowCount := fs.Int("r", 1000, "Rows per API page request (1–10000)")
	outputDir := fs.String("o", ".", "Output directory (created if absent)")
	fmtStr := fs.String("f", "CSV", "Output format: CSV, TSV, or PIPE")
	table := fs.String("t", "", "Extract a single named table")
	tablesStr := fs.String("tables", "", "Comma-separated list of tables to extract")
	partStr := fs.String("p", "", "Partition pattern: DATEMOD=fmt or DATERUN=fmt")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	var tables []string
	if *tablesStr != "" {
		for _, t := range strings.Split(*tablesStr, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tables = append(tables, t)
			}
		}
	}

	var part *partition.Partition
	if *partStr != "" {
		var parseErr error
		part, parseErr = partition.Parse(*partStr)
		if parseErr != nil {
			return nil, parseErr
		}
	}

	return &Config{
		APIKey:    *apiKey,
		APIURL:    *apiURL,
		RowCount:  *rowCount,
		OutputDir: *outputDir,
		Format:    format.OutputFormat(strings.ToUpper(*fmtStr)),
		Table:     *table,
		Tables:    tables,
		Partition: part,
	}, nil
}

// Validate returns an error if the config contains an invalid or missing required value.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("wd-extract: API key is required. Set WD_API_KEY or use -k")
	}
	if c.RowCount < 1 || c.RowCount > 10000 {
		return fmt.Errorf("wd-extract: invalid row count %d: must be 1–10000", c.RowCount)
	}
	switch c.Format {
	case format.FormatCSV, format.FormatTSV, format.FormatPIPE:
		// valid
	default:
		return fmt.Errorf("wd-extract: invalid format %q: must be CSV, TSV, or PIPE", c.Format)
	}
	if c.Table != "" && len(c.Tables) > 0 {
		return fmt.Errorf("wd-extract: -t and -tables are mutually exclusive")
	}
	return nil
}

func envOrDefault(environ func(string) string, key, def string) string {
	if v := environ(key); v != "" {
		return v
	}
	return def
}

// usageHeader is printed before flag defaults when -h or -help is given.
const usageHeader = `Usage: wd-extract [flags]

wd-extract extracts data from the WoodenDollars Data API and writes it to
local files. All tables are extracted by default; use -t or -tables to limit
the selection.

Environment Variables:
  WD_API_KEY  Required API key (data_engineer scope). Overridden by -k.
  WD_API_URL  API base URL (default: https://woodendollars.com). Overridden by -u.

Flags:
`

func printUsage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprint(os.Stderr, usageHeader)
		fs.PrintDefaults()
	}
}
