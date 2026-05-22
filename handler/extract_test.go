package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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

func TestWindowExtract_ScopeCheck(t *testing.T) {
	t.Parallel()
	// A request without the required scope must be rejected with 403.
	h := NewHandler(nil, stubAuth{scope: "other_scope"}, stubConfig{lag: 5}, Config{})

	r := chi.NewRouter()
	r.Get("/data/extract/{table}", h.WindowExtract)

	req := httptest.NewRequest(http.MethodGet, "/data/extract/account", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for wrong scope, got %d", rec.Code)
	}
}

func TestCurrentExtract_ScopeCheck(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{scope: ""}, stubConfig{lag: 5}, Config{})

	r := chi.NewRouter()
	r.Get("/data/extract/{table}/current", h.CurrentExtract)

	req := httptest.NewRequest(http.MethodGet, "/data/extract/account/current", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing scope, got %d", rec.Code)
	}
}

func TestResetExtraction_ScopeCheck(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, stubAuth{scope: "tenant_admin"}, stubConfig{lag: 5}, Config{})

	r := chi.NewRouter()
	r.Post("/data/extract/{table}/reset", h.ResetExtraction)

	req := httptest.NewRequest(http.MethodPost, "/data/extract/account/reset", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-required scope, got %d", rec.Code)
	}
}

// TestRequiredScope_ConfigurableScope verifies that RequiredScope is respected rather than
// the hardcoded "data_engineer" literal. A request with the default scope is rejected when
// the handler is configured with a different scope.
func TestRequiredScope_ConfigurableScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		configScope   string
		requestScope  string
		wantStatus    int
	}{
		{
			name:         "custom scope accepted",
			configScope:  "mop_data_engineer",
			requestScope: "mop_data_engineer",
			// Will hit the pool (nil) after scope check passes — that's expected to panic or fail further,
			// but scope check itself must pass (not return 403 at scope stage).
			// We test the 403 case instead.
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
				// Skip cases that would nil-panic beyond scope check.
				t.Skip("scope passes — further test requires DB")
			}
			h := NewHandler(nil, stubAuth{scope: tc.requestScope}, stubConfig{lag: 5}, Config{
				RequiredScope: tc.configScope,
			})

			r := chi.NewRouter()
			r.Get("/data/extract/{table}", h.WindowExtract)

			req := httptest.NewRequest(http.MethodGet, "/data/extract/account", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("scope=%q config=%q: expected %d, got %d", tc.requestScope, tc.configScope, tc.wantStatus, rec.Code)
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
	// Empty schema = no qualification; host app relies on search_path.
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
	// Schema is stored with trailing dot so table refs are "core.tablename".
	if h.schema != "core." {
		t.Errorf("expected schema 'core.', got %q", h.schema)
	}
	if h.requiredScope != "mop_data_engineer" {
		t.Errorf("expected scope 'mop_data_engineer', got %q", h.requiredScope)
	}
}
