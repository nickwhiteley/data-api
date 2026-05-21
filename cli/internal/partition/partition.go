package partition

import (
	"fmt"
	"strings"
	"time"
)

// Partition holds the parsed -p flag configuration.
type Partition struct {
	Mode    string // "DATEMOD" or "DATERUN"
	SpecFmt string // spec token e.g. "yyyy-mm-dd"
	GoFmt   string // Go time.Format string e.g. "2006-01-02"
}

// Parse parses a raw -p flag value in the form MODE=fmt.
// Valid modes are DATEMOD and DATERUN; fmt must be one of the 5 spec tokens.
func Parse(raw string) (*Partition, error) {
	eq := strings.IndexByte(raw, '=')
	if eq < 0 {
		return nil, fmt.Errorf("wd-extract: invalid partition %q: expected MODE=fmt (e.g. DATEMOD=yyyy-mm)", raw)
	}
	mode, token := raw[:eq], raw[eq+1:]
	switch mode {
	case "DATEMOD", "DATERUN":
	default:
		return nil, fmt.Errorf("wd-extract: invalid partition mode %q: expected DATEMOD or DATERUN", mode)
	}
	goFmt, err := lookupGoFmt(token)
	if err != nil {
		return nil, fmt.Errorf("wd-extract: %w", err)
	}
	return &Partition{Mode: mode, SpecFmt: token, GoFmt: goFmt}, nil
}

// Resolve returns the formatted partition value for the given time.
func (p *Partition) Resolve(ts time.Time) string {
	return ts.Format(p.GoFmt)
}

// candidateFmts are tried in order when parsing modified_at timestamps from the API.
var candidateFmts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// ParseTimestamp parses a modified_at timestamp string returned by the API.
// Returns zero time and an error if no known format matches.
func ParseTimestamp(s string) (time.Time, error) {
	for _, f := range candidateFmts {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp %q", s)
}
