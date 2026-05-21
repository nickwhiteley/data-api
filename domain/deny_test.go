package dataextract

import (
	"testing"
)

func TestIsSensitiveColumn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		col  string
		want bool
	}{
		{"password_hash", true},
		{"key_hash", true},
		{"api_key_secret", true},
		{"session_token", true},
		{"email_verifier", true},
		{"user_id", false},
		{"email", false},
		{"inserted_at", false},
		{"balance", false},
	}
	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			t.Parallel()
			if got := IsSensitiveColumn(tc.col); got != tc.want {
				t.Errorf("IsSensitiveColumn(%q) = %v, want %v", tc.col, got, tc.want)
			}
		})
	}
}

func TestNormalizeTableName(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"user", "user"},
		{"user_log", "user"},
		{"account_log", "account"},
		{"api_key", "api_key"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizeTableName(tc.in); got != tc.want {
				t.Errorf("normalizeTableName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHardBlockedTables(t *testing.T) {
	t.Parallel()
	blocked := []string{
		"user_credential", "session", "api_key", "platform_config",
		"tenant_auth_method", "login_attempt", "rate_limit_bucket",
		"sso_state", "password_reset",
	}
	for _, name := range blocked {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !hardBlockedTables[name] {
				t.Errorf("expected %q to be hard-blocked", name)
			}
		})
	}
}
