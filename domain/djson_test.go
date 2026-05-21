package dataextract

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormaliseValue(t *testing.T) {
	t.Parallel()

	loc := time.UTC
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, loc)

	cases := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"string", "hello", "hello"},
		{"int64", int64(42), int64(42)},
		{"float64", float64(3.14), float64(3.14)},
		{"bool true", true, true},
		{"bool false", false, false},
		{"time UTC", ts, ts.UTC().Format(time.RFC3339Nano)},
		{"time non-UTC", ts.In(time.FixedZone("EST", -5*3600)), ts.UTC().Format(time.RFC3339Nano)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normaliseValue(tc.in)
			// Compare via JSON round-trip for nil-safe equality.
			gotJ, _ := json.Marshal(got)
			wantJ, _ := json.Marshal(tc.want)
			if string(gotJ) != string(wantJ) {
				t.Errorf("normaliseValue(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
