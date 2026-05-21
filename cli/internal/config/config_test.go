package config

import (
	"strings"
	"testing"

	"github.com/nickwhiteley/data-api/cli/internal/format"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantKey  string
		wantURL  string
		wantRows int
		wantDir  string
		wantFmt  format.OutputFormat
		wantErr  bool
	}{
		{
			name:     "WD_API_KEY env sets APIKey",
			args:     []string{},
			env:      map[string]string{"WD_API_KEY": "env-key"},
			wantKey:  "env-key",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-k flag overrides env",
			args:     []string{"-k", "flag-key"},
			env:      map[string]string{"WD_API_KEY": "env-key"},
			wantKey:  "flag-key",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-k flag with no env",
			args:     []string{"-k", "only-flag"},
			env:      map[string]string{},
			wantKey:  "only-flag",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "neither -k nor WD_API_KEY gives empty key",
			args:     []string{},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "WD_API_URL env sets APIURL",
			args:     []string{},
			env:      map[string]string{"WD_API_URL": "https://custom.example.com"},
			wantKey:  "",
			wantURL:  "https://custom.example.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "default APIURL is woodendollars.com",
			args:     []string{},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-r sets RowCount",
			args:     []string{"-r", "500"},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 500,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-r default is 1000",
			args:     []string{},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-o sets OutputDir with absolute path",
			args:     []string{"-o", "/tmp/output"},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  "/tmp/output",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-o accepts relative path as-is",
			args:     []string{"-o", "relative/subdir"},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  "relative/subdir",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-o default is dot",
			args:     []string{},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-f CSV",
			args:     []string{"-f", "CSV"},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:     "-f TSV",
			args:     []string{"-f", "TSV"},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatTSV,
		},
		{
			name:     "-f default is CSV",
			args:     []string{},
			env:      map[string]string{},
			wantKey:  "",
			wantURL:  "https://woodendollars.com",
			wantRows: 1000,
			wantDir:  ".",
			wantFmt:  format.FormatCSV,
		},
		{
			name:    "unknown flag returns error",
			args:    []string{"-z"},
			env:     map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environ := func(key string) string { return tt.env[key] }
			cfg, err := ParseArgs(tt.args, environ)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.APIKey != tt.wantKey {
				t.Errorf("APIKey = %q, want %q", cfg.APIKey, tt.wantKey)
			}
			if cfg.APIURL != tt.wantURL {
				t.Errorf("APIURL = %q, want %q", cfg.APIURL, tt.wantURL)
			}
			if cfg.RowCount != tt.wantRows {
				t.Errorf("RowCount = %d, want %d", cfg.RowCount, tt.wantRows)
			}
			if cfg.OutputDir != tt.wantDir {
				t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, tt.wantDir)
			}
			if cfg.Format != tt.wantFmt {
				t.Errorf("Format = %q, want %q", cfg.Format, tt.wantFmt)
			}
		})
	}
}

func TestParseArgsTableSelection(t *testing.T) {
	t.Parallel()

	env := map[string]string{"WD_API_KEY": "k"}
	environ := func(key string) string { return env[key] }

	t.Run("-t sets Table", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{"-t", "account"}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Table != "account" {
			t.Errorf("Table = %q, want %q", cfg.Table, "account")
		}
		if len(cfg.Tables) != 0 {
			t.Errorf("Tables should be empty, got %v", cfg.Tables)
		}
	})

	t.Run("-tables parses comma-separated list", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{"-tables", "account,transaction,user"}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Table != "" {
			t.Errorf("Table should be empty, got %q", cfg.Table)
		}
		want := []string{"account", "transaction", "user"}
		if len(cfg.Tables) != len(want) {
			t.Fatalf("Tables = %v, want %v", cfg.Tables, want)
		}
		for i, v := range want {
			if cfg.Tables[i] != v {
				t.Errorf("Tables[%d] = %q, want %q", i, cfg.Tables[i], v)
			}
		}
	})

	t.Run("neither flag gives empty Table and Tables", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Table != "" {
			t.Errorf("Table should be empty, got %q", cfg.Table)
		}
		if len(cfg.Tables) != 0 {
			t.Errorf("Tables should be nil/empty, got %v", cfg.Tables)
		}
	})
}

func TestParseArgsPartition(t *testing.T) {
	t.Parallel()

	env := map[string]string{"WD_API_KEY": "k"}
	environ := func(key string) string { return env[key] }

	t.Run("-p DATEMOD=yyyy-mm sets Partition", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{"-p", "DATEMOD=yyyy-mm"}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Partition == nil {
			t.Fatal("Partition is nil, want non-nil")
		}
		if cfg.Partition.Mode != "DATEMOD" {
			t.Errorf("Mode = %q, want DATEMOD", cfg.Partition.Mode)
		}
		if cfg.Partition.SpecFmt != "yyyy-mm" {
			t.Errorf("SpecFmt = %q, want yyyy-mm", cfg.Partition.SpecFmt)
		}
	})

	t.Run("-p DATERUN=yyyymmdd sets Partition", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{"-p", "DATERUN=yyyymmdd"}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Partition == nil {
			t.Fatal("Partition is nil, want non-nil")
		}
		if cfg.Partition.Mode != "DATERUN" {
			t.Errorf("Mode = %q, want DATERUN", cfg.Partition.Mode)
		}
	})

	t.Run("invalid format token returns error", func(t *testing.T) {
		t.Parallel()
		_, err := ParseArgs([]string{"-p", "DATEMOD=dd-mm-yyyy"}, environ)
		if err == nil {
			t.Fatal("expected error for invalid token, got nil")
		}
	})

	t.Run("missing = returns error", func(t *testing.T) {
		t.Parallel()
		_, err := ParseArgs([]string{"-p", "DATEMODyyyy-mm"}, environ)
		if err == nil {
			t.Fatal("expected error for missing =, got nil")
		}
	})

	t.Run("no -p flag gives nil Partition", func(t *testing.T) {
		t.Parallel()
		cfg, err := ParseArgs([]string{}, environ)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Partition != nil {
			t.Errorf("Partition should be nil, got %+v", cfg.Partition)
		}
	})
}

func TestValidateTableSelection(t *testing.T) {
	t.Parallel()

	base := Config{APIKey: "key", RowCount: 1000, OutputDir: ".", Format: format.FormatCSV}

	t.Run("-t and -tables mutually exclusive", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.Table = "account"
		cfg.Tables = []string{"account", "transaction"}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error for mutual exclusivity, got nil")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("error %q does not contain 'mutually exclusive'", err.Error())
		}
	})

	t.Run("-t alone is valid", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.Table = "account"
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("-tables alone is valid", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.Tables = []string{"account", "transaction"}
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("neither is valid (all-tables mode)", func(t *testing.T) {
		t.Parallel()
		cfg := base
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Config
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing APIKey returns error",
			cfg:         Config{APIKey: "", RowCount: 1000, OutputDir: ".", Format: format.FormatCSV},
			wantErr:     true,
			errContains: "API key is required",
		},
		{
			name:        "row count 0 returns error",
			cfg:         Config{APIKey: "key", RowCount: 0, OutputDir: ".", Format: format.FormatCSV},
			wantErr:     true,
			errContains: "invalid row count",
		},
		{
			name:        "row count 10001 returns error",
			cfg:         Config{APIKey: "key", RowCount: 10001, OutputDir: ".", Format: format.FormatCSV},
			wantErr:     true,
			errContains: "invalid row count",
		},
		{
			name:    "row count 10000 is valid",
			cfg:     Config{APIKey: "key", RowCount: 10000, OutputDir: ".", Format: format.FormatCSV},
			wantErr: false,
		},
		{
			name:    "row count 1 is valid",
			cfg:     Config{APIKey: "key", RowCount: 1, OutputDir: ".", Format: format.FormatCSV},
			wantErr: false,
		},
		{
			name:        "invalid format returns error",
			cfg:         Config{APIKey: "key", RowCount: 1000, OutputDir: ".", Format: "EXCEL"},
			wantErr:     true,
			errContains: "invalid format",
		},
		{
			name:    "valid CSV config passes",
			cfg:     Config{APIKey: "key", RowCount: 1000, OutputDir: ".", Format: format.FormatCSV},
			wantErr: false,
		},
		{
			name:    "valid TSV config passes",
			cfg:     Config{APIKey: "key", RowCount: 500, OutputDir: "/tmp", Format: format.FormatTSV},
			wantErr: false,
		},
		{
			name:    "valid PIPE config passes",
			cfg:     Config{APIKey: "key", RowCount: 100, OutputDir: ".", Format: format.FormatPIPE},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestQuickstartPatterns verifies that every command pattern shown in quickstart.md
// parses and validates correctly.
func TestQuickstartPatterns(t *testing.T) {
	t.Parallel()

	apiEnv := map[string]string{"WD_API_KEY": "test-key"}
	environ := func(key string) string { return apiEnv[key] }

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "all tables default (wd-extract)",
			args: []string{},
		},
		{
			name: "single table (-t transaction)",
			args: []string{"-t", "transaction"},
		},
		{
			name: "table subset (-tables account,transaction,escrow)",
			args: []string{"-tables", "account,transaction,escrow"},
		},
		{
			name: "TSV format with output dir (-f TSV -o /data/exports)",
			args: []string{"-f", "TSV", "-o", "/data/exports"},
		},
		{
			name: "DATEMOD partition (-p DATEMOD=yyyy-mm -o /data/lake)",
			args: []string{"-p", "DATEMOD=yyyy-mm", "-o", "/data/lake"},
		},
		{
			name: "DATERUN partition (-p DATERUN=yyyy-mm-dd -o /data/daily)",
			args: []string{"-p", "DATERUN=yyyy-mm-dd", "-o", "/data/daily"},
		},
		{
			name: "custom page size (-r 5000 -t transaction)",
			args: []string{"-r", "5000", "-t", "transaction"},
		},
		{
			name: "staging URL override (-u with -t)",
			args: []string{"-u", "https://staging.woodendollars.com", "-t", "account"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := ParseArgs(tt.args, environ)
			if err != nil {
				t.Fatalf("ParseArgs: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
