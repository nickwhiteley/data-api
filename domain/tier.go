package dataextract

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IsDataAPIEnabled returns true if the tenant's current usage tier has data_api_enabled = true.
func IsDataAPIEnabled(ctx context.Context, pool *pgxpool.Pool, tenantID string) (bool, error) {
	var enabled bool
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(put.data_api_enabled, false)
		FROM wd.tenant t
		JOIN wd.platform_usage_tier put
		  ON put.platform_usage_tier_id = t.usage_tier_id
		WHERE t.tenant_id = $1 AND t.deleted_at IS NULL`,
		tenantID,
	).Scan(&enabled); err != nil {
		return false, fmt.Errorf("check data api enabled for tenant %s: %w", tenantID, err)
	}
	return enabled, nil
}
