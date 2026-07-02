package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubAuth is a test implementation of AuthContext.
type stubAuth struct {
	scope    string
	userID   string
	tenantID string
}

func (s stubAuth) Scope(context.Context) string    { return s.scope }
func (s stubAuth) UserID(context.Context) string   { return s.userID }
func (s stubAuth) TenantID(context.Context) string { return s.tenantID }

// stubConfig is a test implementation of DataConfig.
type stubConfig struct{ lag int }

func (c stubConfig) SafetyLagSeconds() int { return c.lag }

// scopeCheckRequest sends a request to the handler's mux and returns the response code.
// The path must include any URL path values (e.g. /data/extract/account).
func scopeCheckRequest(t *testing.T, h *ExtractHandler, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestWindowExtract_ScopeCheck(t *testing.T) {
	t.Parallel()
	// A request without the required scope must be rejected with 403.
	h := NewHandler(nil, stubAuth{scope: "other_scope"}, stubConfig{lag: 5}, Config{})
	if got := scopeCheckRequest(t, h, http.MethodGet, "/data/extract/account"); got != http.StatusForbidden {
		t.Errorf("expected 403 for wrong scope, got %d", got)
	}
}

func TestCurrentExtract_ScopeCheck(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{scope: ""}, stubConfig{lag: 5}, Config{})
	if got := scopeCheckRequest(t, h, http.MethodGet, "/data/extract/account/current"); got != http.StatusForbidden {
		t.Errorf("expected 403 for missing scope, got %d", got)
	}
}

func TestResetExtraction_ScopeCheck(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{scope: "tenant_admin"}, stubConfig{lag: 5}, Config{})
	if got := scopeCheckRequest(t, h, http.MethodPost, "/data/extract/account/reset"); got != http.StatusForbidden {
		t.Errorf("expected 403 for non-required scope, got %d", got)
	}
}

// TestRequiredScope_ConfigurableScope verifies that RequiredScope is respected rather than
// the hardcoded "data_engineer" literal.
func TestRequiredScope_ConfigurableScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		configScope  string
		requestScope string
		wantStatus   int
	}{
		{
			name:         "custom scope accepted",
			configScope:  "mop_data_engineer",
			requestScope: "mop_data_engineer",
			// Scope passes but nil pool will fail further — skip.
		},
		{
			name:         "default scope rejected when custom configured",
			configScope:  "mop_data_engineer",
			requestScope: "data_engineer",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "zero-value config defaults to data_engineer, wrong scope rejected",
			configScope:  "",
			requestScope: "other",
			wantStatus:   http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.wantStatus == 0 {
				t.Skip("scope passes — further test requires DB")
			}
			h := NewHandler(nil, stubAuth{scope: tc.requestScope}, stubConfig{lag: 5}, Config{
				RequiredScope: tc.configScope,
			})
			if got := scopeCheckRequest(t, h, http.MethodGet, "/data/extract/account"); got != tc.wantStatus {
				t.Errorf("scope=%q config=%q: expected %d, got %d", tc.requestScope, tc.configScope, tc.wantStatus, got)
			}
		})
	}
}

func TestParseTimeParam(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"2024-01-15T12:00:00Z", false},
		{"1705320000", false},
		{"not-a-time", true},
		{"2024-99-99", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			_, err := parseTimeParam(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseTimeParam(%q) error=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestNewHandler_InterfaceSatisfaction(t *testing.T) {
	// Compile-time check that stubAuth and stubConfig satisfy the interfaces.
	var _ AuthContext = stubAuth{}
	var _ DataConfig = stubConfig{}
}

func TestNewHandler_DefaultSchema(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{}, stubConfig{}, Config{})
	if h.schema != "" {
		t.Errorf("expected empty schema for zero config, got %q", h.schema)
	}
	if h.requiredScope != "data_engineer" {
		t.Errorf("expected default scope 'data_engineer', got %q", h.requiredScope)
	}
}

func TestNewHandler_ExplicitConfig(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{}, stubConfig{}, Config{Schema: "core", RequiredScope: "mop_data_engineer"})
	if h.schema != "core." {
		t.Errorf("expected schema 'core.', got %q", h.schema)
	}
	if h.requiredScope != "mop_data_engineer" {
		t.Errorf("expected scope 'mop_data_engineer', got %q", h.requiredScope)
	}
}
