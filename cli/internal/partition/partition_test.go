package partition

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantMode  string
		wantSpec  string
		wantGoFmt string
		wantErr   bool
	}{
		{
			name:      "DATEMOD yyyy",
			raw:       "DATEMOD=yyyy",
			wantMode:  "DATEMOD",
			wantSpec:  "yyyy",
			wantGoFmt: "2006",
		},
		{
			name:      "DATEMOD yyyy-mm",
			raw:       "DATEMOD=yyyy-mm",
			wantMode:  "DATEMOD",
			wantSpec:  "yyyy-mm",
			wantGoFmt: "2006-01",
		},
		{
			name:      "DATEMOD yyyy-mm-dd",
			raw:       "DATEMOD=yyyy-mm-dd",
			wantMode:  "DATEMOD",
			wantSpec:  "yyyy-mm-dd",
			wantGoFmt: "2006-01-02",
		},
		{
			name:      "DATERUN yyyymmdd",
			raw:       "DATERUN=yyyymmdd",
			wantMode:  "DATERUN",
			wantSpec:  "yyyymmdd",
			wantGoFmt: "20060102",
		},
		{
			name:      "DATERUN yyyy-mm-dd-hh",
			raw:       "DATERUN=yyyy-mm-dd-hh",
			wantMode:  "DATERUN",
			wantSpec:  "yyyy-mm-dd-hh",
			wantGoFmt: "2006-01-02-15",
		},
		{
			name:    "unknown format token returns error",
			raw:     "DATEMOD=dd-mm-yyyy",
			wantErr: true,
		},
		{
			name:    "unknown mode returns error",
			raw:     "WEEKLY=yyyy-mm",
			wantErr: true,
		},
		{
			name:    "missing = returns error",
			raw:     "DATEMODyyyy-mm",
			wantErr: true,
		},
		{
			name:    "empty string returns error",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := Parse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", p.Mode, tt.wantMode)
			}
			if p.SpecFmt != tt.wantSpec {
				t.Errorf("SpecFmt = %q, want %q", p.SpecFmt, tt.wantSpec)
			}
			if p.GoFmt != tt.wantGoFmt {
				t.Errorf("GoFmt = %q, want %q", p.GoFmt, tt.wantGoFmt)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	ref := time.Date(2026, 5, 18, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		token string
		want  string
	}{
		{"yyyy", "2026"},
		{"yyyy-mm", "2026-05"},
		{"yyyy-mm-dd", "2026-05-18"},
		{"yyyymmdd", "20260518"},
		{"yyyy-mm-dd-hh", "2026-05-18-14"},
	}

	for _, tt := range tests {
		t.Run("DATERUN="+tt.token, func(t *testing.T) {
			t.Parallel()
			p, err := Parse("DATERUN=" + tt.token)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := p.Resolve(ref); got != tt.want {
				t.Errorf("Resolve(%v) = %q, want %q", ref, got, tt.want)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantYear  int
		wantMonth time.Month
		wantDay   int
		wantErr   bool
	}{
		{
			name:      "RFC3339 with timezone",
			input:     "2026-04-15T10:30:00Z",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:      "RFC3339 with offset",
			input:     "2026-04-15T10:30:00+01:00",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:      "datetime without timezone",
			input:     "2026-04-15T10:30:00",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:      "space-separated datetime",
			input:     "2026-04-15 10:30:00",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:      "date only",
			input:     "2026-04-15",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:    "unparseable returns error",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ts, err := ParseTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ts.Year() != tt.wantYear {
				t.Errorf("year = %d, want %d", ts.Year(), tt.wantYear)
			}
			if ts.Month() != tt.wantMonth {
				t.Errorf("month = %v, want %v", ts.Month(), tt.wantMonth)
			}
			if ts.Day() != tt.wantDay {
				t.Errorf("day = %d, want %d", ts.Day(), tt.wantDay)
			}
		})
	}
}

func TestDateModPathResolution(t *testing.T) {
	t.Parallel()

	p, err := Parse("DATEMOD=yyyy-mm")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	ts, err := ParseTimestamp("2026-04-15T10:30:00Z")
	if err != nil {
		t.Fatalf("ParseTimestamp: %v", err)
	}

	got := p.Resolve(ts)
	if got != "2026-04" {
		t.Errorf("DATEMOD yyyy-mm resolution = %q, want %q", got, "2026-04")
	}
}
