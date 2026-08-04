package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// EffectivePolicy holds the resolved effective configuration for a single category.
type EffectivePolicy struct {
	Category            string
	Config              map[string]interface{}
	Maintenance         map[string]interface{}
	Layers              []policyLayer
	ScopeLevel          string
	PublishedVersion    int
}

// ComputeEffectivePolicy resolves the effective policy configuration for a device
// across all categories, applying most-specific-wins merging across the
// hierarchy: msp → client → site → device.
//
// The function queries the policies table for all active, published policies
// relevant to the device's hierarchy, orders them by scope rank (msp first),
// and merges configs so that more-specific scopes override less-specific ones.
//
// Parameters:
//   - ctx: context for the database query
//   - db: database connection (read-only)
//   - mspID: the MSP ID that owns the device hierarchy
//   - deviceID: the target device ID (empty string to resolve for MSP only)
//
// Returns:
//   - A map of category → EffectivePolicy
//   - An error if the database query fails
//
// Policy categories resolved: patch, alerting, monitoring, software, script,
// maintenance, maintenance_window.
func ComputeEffectivePolicy(ctx context.Context, db *sql.DB, mspID string, deviceID string) (map[string]EffectivePolicy, error) {
	if mspID == "" {
		return nil, fmt.Errorf("msp_id is required")
	}

	categories := []string{"patch", "alerting", "monitoring", "software", "script", "maintenance", "maintenance_window"}
	result := make(map[string]EffectivePolicy)

	for _, category := range categories {
		effective, err := computeEffectiveForCategory(ctx, db, mspID, deviceID, category)
		if err != nil {
			return nil, fmt.Errorf("category %s: %w", category, err)
		}
		result[category] = effective
	}

	return result, nil
}

// computeEffectiveForCategory resolves the effective policy for a single category
// using most-specific-wins merging across the MSP hierarchy.
func computeEffectiveForCategory(ctx context.Context, db *sql.DB, mspID, deviceID, category string) (EffectivePolicy, error) {
	// Build the device's hierarchy: find client_id and site_id for the device
	var clientID, siteID string
	if deviceID != "" {
		err := db.QueryRowContext(ctx, `
			SELECT client_id, site_id FROM devices
			WHERE id = $1 AND msp_id = $2 AND is_active = true
		`, deviceID, mspID).Scan(&clientID, &siteID)
		if err == sql.ErrNoRows {
			// Device not found or not active — return empty result for this category
			return EffectivePolicy{Category: category, Config: map[string]interface{}{}, Maintenance: map[string]interface{}{}}, nil
		}
		if err != nil {
			return EffectivePolicy{}, fmt.Errorf("query device hierarchy: %w", err)
		}
	}

	// Load all policies for this category, ordered by scope rank (msp first, device last)
	// The ORDER BY ensures lower-rank scopes appear first, so mergePolicyLayers
	// will apply higher-rank overrides on top.
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope_level, published_version,
		       (published_config)::text,
		       maintenance_start, maintenance_end,
		       maintenance_days::text, maintenance_timezone
		FROM policies
		WHERE msp_id = $1
		  AND category = $2
		  AND status = 'active'
		  AND published_version IS NOT NULL
		  AND (
			(scope_level = 'msp')
			OR (scope_level = 'client' AND client_id = NULLIF($3, ''))
			OR (scope_level = 'site' AND site_id = NULLIF($4, ''))
			OR (scope_level = 'device' AND device_id = NULLIF($5, ''))
		  )
		ORDER BY
			CASE scope_level
				WHEN 'msp' THEN 1
				WHEN 'client' THEN 2
				WHEN 'site' THEN 3
				WHEN 'device' THEN 4
			END ASC, id ASC
	`, mspID, category, clientID, siteID, deviceID)
	if err != nil {
		return EffectivePolicy{}, fmt.Errorf("query policies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var layers []policyLayer
	for rows.Next() {
		var layer policyLayer
		var configText, maintenanceDaysText string
		if err := rows.Scan(
			&layer.ID, &layer.ScopeLevel, &layer.PublishedVersion,
			&configText,
			&layer.MaintenanceStart, &layer.MaintenanceEnd,
			&maintenanceDaysText, &layer.MaintenanceTimezone,
		); err != nil {
			return EffectivePolicy{}, fmt.Errorf("scan policy layer: %w", err)
		}
		if err := json.Unmarshal([]byte(configText), &layer.Config); err != nil {
			return EffectivePolicy{}, fmt.Errorf("unmarshal config: %w", err)
		}
		if maintenanceDaysText != "" && maintenanceDaysText != "null" {
			var days []string
			if err := json.Unmarshal([]byte(maintenanceDaysText), &days); err != nil {
				return EffectivePolicy{}, fmt.Errorf("unmarshal maintenance_days: %w", err)
			}
			if len(days) > 0 {
				layer.MaintenanceDays = &days
			}
		}
		layers = append(layers, layer)
	}
	if err := rows.Err(); err != nil {
		return EffectivePolicy{}, fmt.Errorf("iterate policy rows: %w", err)
	}

	// Determine the most-specific scope level
	scopeLevel := "msp"
	publishedVersion := 0
	for _, layer := range layers {
		rank := policyScopeRank[layer.ScopeLevel]
		currentRank := policyScopeRank[scopeLevel]
		if rank > currentRank {
			scopeLevel = layer.ScopeLevel
			publishedVersion = layer.PublishedVersion
		}
	}

	// Merge configs: lower-rank scopes first, higher-rank overrides
	effectiveConfig := mergePolicyLayers(layers)

	// Compute effective maintenance separately (non-deep-merge)
	effectiveMaintenance := computeEffectiveMaintenance(layers)

	return EffectivePolicy{
		Category:         category,
		Config:           effectiveConfig,
		Maintenance:      effectiveMaintenance,
		Layers:           layers,
		ScopeLevel:       scopeLevel,
		PublishedVersion: publishedVersion,
	}, nil
}

// computeEffectiveMaintenance computes the effective maintenance window by
// merging layers. Unlike config merging, maintenance fields are set by the
// most-specific layer (device > site > client > msp), not deep-merged.
func computeEffectiveMaintenance(layers []policyLayer) map[string]interface{} {
	effective := map[string]interface{}{}
	for _, layer := range layers {
		if layer.MaintenanceStart != nil {
			effective["maintenance_start"] = *layer.MaintenanceStart
		}
		if layer.MaintenanceEnd != nil {
			effective["maintenance_end"] = *layer.MaintenanceEnd
		}
		if layer.MaintenanceDays != nil && len(*layer.MaintenanceDays) > 0 {
			effective["maintenance_days"] = *layer.MaintenanceDays
		}
		if layer.MaintenanceTimezone != "" {
			effective["maintenance_timezone"] = layer.MaintenanceTimezone
		}
	}
	return effective
}
