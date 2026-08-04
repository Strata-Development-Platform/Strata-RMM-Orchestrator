//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestComputeEffectivePolicyMspOnly(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mspID := generateUUID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDB(ctx, t, db, mspID)
	defer cleanupDB(ctx, t, db, mspID)

	// Create MSP-level patch policy
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO policies (id, msp_id, name, category, description, config, scope_level, status, published_version, published_config)
		VALUES ($1, $2, 'msp-patch-policy', 'patch', 'MSP-level patch policy', '{"auto_update": true, "frequency": "daily"}', 'msp', 'active', 1, '{"auto_update": true, "frequency": "daily"}')
	`, generateUUID(), mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Compute effective policy for MSP (no device)
	result, err := ComputeEffectivePolicy(ctx, db, mspID, "")
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	// Verify patch category has MSP-level config
	patchPolicy, ok := result["patch"]
	if !ok {
		t.Fatal("patch category not in result")
	}

	autoUpdate, ok := patchPolicy.Config["auto_update"]
	if !ok {
		t.Fatal("auto_update not in effective config")
	}
	if autoUpdate != true {
		t.Errorf("auto_update = %v, want true", autoUpdate)
	}

	if patchPolicy.ScopeLevel != "msp" {
		t.Errorf("ScopeLevel = %q, want %q", patchPolicy.ScopeLevel, "msp")
	}
}

func TestComputeEffectivePolicyDeviceOverridesMsp(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mspID := generateUUID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDB(ctx, t, db, mspID)
	defer cleanupDB(ctx, t, db, mspID)

	// Create MSP
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create MSP-level patch policy
	mspPolicyID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO policies (id, msp_id, name, category, description, config, scope_level, status, published_version, published_config)
		VALUES ($1, $2, 'msp-patch', 'patch', 'MSP patch', '{"auto_update": true, "frequency": "daily"}', 'msp', 'active', 1, '{"auto_update": true, "frequency": "daily"}')
	`, mspPolicyID, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create client
	clientID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, is_active) VALUES ($1, $2, 'Test Client', true)
	`, clientID, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create site
	siteID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO sites (id, client_id, name, is_active) VALUES ($1, $2, 'Test Site', true)
	`, siteID, clientID)
	if err != nil {
		t.Fatal(err)
	}

	// Create device
	deviceID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO devices (id, msp_id, client_id, site_id, name, is_active) VALUES ($1, $2, $3, $4, 'Test Device', true)
	`, deviceID, mspID, clientID, siteID)
	if err != nil {
		t.Fatal(err)
	}

	// Create device-level patch policy (overrides MSP)
	devicePolicyID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO policies (id, msp_id, name, category, description, config, scope_level, client_id, site_id, device_id, status, published_version, published_config)
		VALUES ($1, $2, 'device-patch', 'patch', 'Device patch', '{"auto_update": false, "frequency": "weekly"}', 'device', $3, $4, $5, 'active', 1, '{"auto_update": false, "frequency": "weekly"}')
	`, devicePolicyID, mspID, clientID, siteID, deviceID)
	if err != nil {
		t.Fatal(err)
	}

	// Compute effective policy for device
	result, err := ComputeEffectivePolicy(ctx, db, mspID, deviceID)
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	patchPolicy, ok := result["patch"]
	if !ok {
		t.Fatal("patch category not in result")
	}

	// Device-level should override MSP-level
	autoUpdate, ok := patchPolicy.Config["auto_update"]
	if !ok {
		t.Fatal("auto_update not in effective config")
	}
	if autoUpdate != false {
		t.Errorf("auto_update = %v, want false (device overrides msp)", autoUpdate)
	}

	freq, ok := patchPolicy.Config["frequency"]
	if !ok {
		t.Fatal("frequency not in effective config")
	}
	if freq != "weekly" {
		t.Errorf("frequency = %v, want weekly (device overrides msp)", freq)
	}

	if patchPolicy.ScopeLevel != "device" {
		t.Errorf("ScopeLevel = %q, want device", patchPolicy.ScopeLevel)
	}

	// Verify both layers are present
	if len(patchPolicy.Layers) != 2 {
		t.Errorf("Layers count = %d, want 2 (msp + device)", len(patchPolicy.Layers))
	}
}

func TestComputeEffectivePolicyClientOverridesMsp(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mspID := generateUUID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDB(ctx, t, db, mspID)
	defer cleanupDB(ctx, t, db, mspID)

	// Create MSP
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create MSP-level alerting policy
	_, err = db.ExecContext(ctx, `
		INSERT INTO policies (id, msp_id, name, category, description, config, scope_level, status, published_version, published_config)
		VALUES ($1, $2, 'msp-alerting', 'alerting', 'MSP alerting', '{"severity": "info", "channel": "email"}', 'msp', 'active', 1, '{"severity": "info", "channel": "email"}')
	`, generateUUID(), mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create client
	clientID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, is_active) VALUES ($1, $2, 'Test Client', true)
	`, clientID, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create client-level alerting policy (overrides MSP)
	_, err = db.ExecContext(ctx, `
		INSERT INTO policies (id, msp_id, name, category, description, config, scope_level, client_id, status, published_version, published_config)
		VALUES ($1, $2, 'client-alerting', 'alerting', 'Client alerting', '{"severity": "warning", "channel": "pagerduty"}', 'client', $3, 'active', 1, '{"severity": "warning", "channel": "pagerduty"}')
	`, generateUUID(), mspID, clientID)
	if err != nil {
		t.Fatal(err)
	}

	// Compute effective policy for MSP (client policies should not apply since no client specified)
	result, err := ComputeEffectivePolicy(ctx, db, mspID, "")
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	alertingPolicy, ok := result["alerting"]
	if !ok {
		t.Fatal("alerting category not in result")
	}

	// MSP-level should be effective (no client specified)
	severity, ok := alertingPolicy.Config["severity"]
	if !ok {
		t.Fatal("severity not in effective config")
	}
	if severity != "info" {
		t.Errorf("severity = %v, want info (msp, no client)", severity)
	}

	// Now create a device under this client and verify client-level overrides MSP
	_, err = db.ExecContext(ctx, `
		INSERT INTO sites (id, client_id, name, is_active) VALUES ($1, $2, 'Test Site', true)
	`, generateUUID(), clientID)
	if err != nil {
		t.Fatal(err)
	}

	deviceID := generateUUID()
	_, err = db.ExecContext(ctx, `
		INSERT INTO devices (id, msp_id, client_id, site_id, name, is_active) VALUES ($1, $2, $3, $4, 'Test Device', true)
	`, deviceID, mspID, clientID, generateUUID())
	if err != nil {
		t.Fatal(err)
	}

	result, err = ComputeEffectivePolicy(ctx, db, mspID, deviceID)
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	alertingPolicy, ok = result["alerting"]
	if !ok {
		t.Fatal("alerting category not in result")
	}

	// Client-level should override MSP when device is under that client
	severity, ok = alertingPolicy.Config["severity"]
	if !ok {
		t.Fatal("severity not in effective config")
	}
	if severity != "warning" {
		t.Errorf("severity = %v, want warning (client overrides msp for device under client)", severity)
	}
}

func TestComputeEffectivePolicyEmptyCategory(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mspID := generateUUID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDB(ctx, t, db, mspID)
	defer cleanupDB(ctx, t, db, mspID)

	// No policies at all — only MSP exists
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ComputeEffectivePolicy(ctx, db, mspID, "")
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	// All categories should be present with empty config
	for _, cat := range []string{"patch", "alerting", "monitoring", "software", "script", "maintenance", "maintenance_window"} {
		policy, ok := result[cat]
		if !ok {
			t.Errorf("category %s not in result", cat)
			continue
		}
		if len(policy.Config) != 0 {
			t.Errorf("category %s config = %v, want empty", cat, policy.Config)
		}
	}
}

func TestComputeEffectivePolicyMissingMSP(t *testing.T) {
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = ComputeEffectivePolicy(context.Background(), db, "", "some-device")
	if err == nil {
		t.Fatal("expected error for empty msp_id")
	}
}

func TestComputeEffectivePolicyDeviceNotFound(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	dbURL := os.Getenv("TEST_POSTGRES_DSN")
	if dbURL == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mspID := generateUUID()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDB(ctx, t, db, mspID)
	defer cleanupDB(ctx, t, db, mspID)

	// MSP exists but no policies
	_, err = db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Query for non-existent device
	result, err := ComputeEffectivePolicy(ctx, db, mspID, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("ComputeEffectivePolicy: %v", err)
	}

	// Should return empty configs, not error
	for cat, policy := range result {
		if len(policy.Config) != 0 {
			t.Errorf("category %s config = %v, want empty for non-existent device", cat, policy.Config)
		}
	}
}

func generateUUID() string {
	return fmt.Sprintf("%d-%d-%d-%d-%d", time.Now().UnixNano(), time.Now().Nanosecond(), os.Getpid(), os.Getppid(), time.Now().Unix()%100000)
}

func setupDB(ctx context.Context, t *testing.T, db *sql.DB, mspID string) {
	// Create required tables if they don't exist
	_, _ = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS msp_tenants (
			id         UUID PRIMARY KEY,
			name       TEXT NOT NULL,
			is_active  BOOLEAN NOT NULL DEFAULT true
		)
	`)
	_, _ = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS policies (
			id                    UUID PRIMARY KEY,
			msp_id                UUID NOT NULL,
			client_id             UUID,
			site_id               UUID,
			device_id             UUID,
			name                  TEXT NOT NULL,
			category              TEXT NOT NULL,
			description           TEXT NOT NULL DEFAULT '',
			config                JSONB NOT NULL DEFAULT '{}',
			scope_level           TEXT NOT NULL DEFAULT 'msp',
			status                TEXT NOT NULL DEFAULT 'draft',
			published_version     INT,
			published_config      JSONB,
			validated_at          TIMESTAMPTZ,
			previewed_at          TIMESTAMPTZ,
			maintenance_start     TIME,
			maintenance_end       TIME,
			maintenance_days      JSONB,
			maintenance_timezone  TEXT NOT NULL DEFAULT 'UTC',
			created_by            TEXT,
			created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	_, _ = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS client_organizations (
			id         UUID PRIMARY KEY,
			msp_id     UUID NOT NULL,
			name       TEXT NOT NULL,
			is_active  BOOLEAN NOT NULL DEFAULT true
		)
	`)
	_, _ = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sites (
			id         UUID PRIMARY KEY,
			client_id  UUID NOT NULL,
			name       TEXT NOT NULL,
			is_active  BOOLEAN NOT NULL DEFAULT true
		)
	`)
	_, _ = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS devices (
			id         UUID PRIMARY KEY,
			msp_id     UUID NOT NULL,
			client_id  UUID NOT NULL,
			site_id    UUID,
			name       TEXT NOT NULL,
			is_active  BOOLEAN NOT NULL DEFAULT true
		)
	`)
}

func cleanupDB(ctx context.Context, t *testing.T, db *sql.DB, mspID string) {
	_, _ = db.ExecContext(ctx, "DELETE FROM policies WHERE msp_id = $1", mspID)
	_, _ = db.ExecContext(ctx, "DELETE FROM client_organizations WHERE msp_id = $1", mspID)
	_, _ = db.ExecContext(ctx, "DELETE FROM msp_tenants WHERE id = $1", mspID)
}
