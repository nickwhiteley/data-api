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
	// A request without data_engineer scope must be rejected with 403.
	h := NewHandler(nil, stubAuth{scope: "other_scope"}, stubConfig{lag: 5})

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
	h := NewHandler(nil, stubAuth{scope: ""}, stubConfig{lag: 5})

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
	h := NewHandler(nil, stubAuth{scope: "tenant_admin"}, stubConfig{lag: 5})

	r := chi.NewRouter()
	r.Post("/data/extract/{table}/reset", h.ResetExtraction)

	req := httptest.NewRequest(http.MethodPost, "/data/extract/account/reset", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-data_engineer scope, got %d", rec.Code)
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
