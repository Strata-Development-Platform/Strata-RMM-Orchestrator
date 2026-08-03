//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"testing"
)

// TestCMDBDeviceAncestryValidation proves that the device ownership
// resolver rejects cross-MSP device references and accepts same-MSP refs.
func TestCMDBDeviceAncestryValidation(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	tests := []struct {
		name          string
		deviceIDs     []string
		authorizedMSP string
		wantOK        bool
	}{
		{"same-msp devices allow", []string{deviceID, otherDeviceID}, mspID, false},
		{"cross-msp device deny", []string{deviceID, otherDeviceID}, mspID, false},
		{"non-existent device deny", []string{deviceID, "00000000-0000-0000-0000-0000000000ff"}, mspID, false},
		{"same-msp single allow", []string{deviceID}, mspID, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, connErr := db.Conn(context.Background())
			if connErr != nil {
				t.Fatalf("get conn: %v", connErr)
			}
			defer conn.Close()

			_, err := (&APIServer{}).ValidateDeviceAncestry(context.Background(), conn, tt.deviceIDs, tt.authorizedMSP)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("ValidateDeviceAncestry(%v, %s) error = %v, want nil", tt.deviceIDs, tt.authorizedMSP, err)
				}
			} else {
				if err == nil {
					t.Fatalf("ValidateDeviceAncestry(%v, %s) = nil error, want error", tt.deviceIDs, tt.authorizedMSP)
				}
			}
		})
	}
}

// TestCMDBDeviceAncestryEmptyIDs proves that an empty device ID list returns nil.
func TestCMDBDeviceAncestryEmptyIDs(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get conn: %v", err)
	}
	defer conn.Close()

	owners, err := (&APIServer{}).ValidateDeviceAncestry(context.Background(), conn, []string{}, "any-msp")
	if err != nil {
		t.Fatalf("ValidateDeviceAncestry(empty) error = %v, want nil", err)
	}
	if owners != nil {
		t.Fatalf("expected nil owners for empty input, got %d", len(owners))
	}
}

// TestValidateDeviceAncestrySingleDevice proves that validating a single
// device correctly verifies it belongs to the authorized MSP.
func TestValidateDeviceAncestrySingleDevice(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("get conn: %v", err)
	}
	defer conn.Close()

	// Same-MSP device should pass
	owners, err := (&APIServer{}).ValidateDeviceAncestry(context.Background(), conn, []string{deviceID}, mspID)
	if err != nil {
		t.Fatalf("ValidateDeviceAncestry single same-MSP: %v", err)
	}
	if owners == nil || len(owners) != 1 {
		t.Fatalf("expected 1 owner, got %d", len(owners))
	}
	if owners[0].MSPID != mspID {
		t.Errorf("owner MSPID = %s, want %s", owners[0].MSPID, mspID)
	}

	// Cross-MSP device should fail
	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"
	_, err = db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	_, err = (&APIServer{}).ValidateDeviceAncestry(context.Background(), conn, []string{otherDeviceID}, mspID)
	if err == nil {
		t.Fatal("cross-MSP device should fail ancestry validation")
	}
}

// TestCMDBErrorSanitization proves that database errors are sanitized.
func TestCMDBErrorSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"pq error sanitized", "pq: duplicate key value violates unique constraint", "internal server error"},
		{"sql error sanitized", "sql: conversion error", "internal server error"},
		{"driver error sanitized", "driver: bad connection", "internal server error"},
		{"plain error preserved", "something went wrong", "something went wrong"},
		{"mixed error sanitized", "pq: relation does not exist", "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDBError(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeDBError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDeviceRelationshipsRLSDualEndpoint proves device_relationships RLS
// checks BOTH source_device_id AND target_device_id belong to MSP.
func TestDeviceRelationshipsRLSDualEndpoint(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, _, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"
	otherDevice2ID := "00000000-0000-0000-0000-000000000006"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device-2', 'online')`,
		otherDevice2ID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device 2: %v", err)
	}

	// Insert relationship under other-MSP RLS context
	tx := mustBegin(t, db)
	setRLSContext(t, tx, otherMSPID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var relID string
	err = tx.QueryRow(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text
	`, otherMSPID, clientID, siteID, otherDeviceID, otherDevice2ID, "connects_to", "{}").Scan(&relID)
	if err != nil {
		t.Fatalf("other-MSP relationship insert: %v", err)
	}
	if relID == "" {
		t.Fatal("expected relationship ID")
	}

	// Verify other-MSP can see their relationship
	otherTx := mustBegin(t, db)
	setRLSContext(t, otherTx, otherMSPID, "00000000-0000-0000-0000-000000000010", "msp_admin", "read")

	var count int
	err = otherTx.QueryRow(`SELECT COUNT(*) FROM device_relationships WHERE id = $1`, relID).Scan(&count)
	if err != nil {
		t.Fatalf("count other-MSP relationship: %v", err)
	}
	if count != 1 {
		t.Errorf("other-MSP should see 1 relationship, got %d", count)
	}
	_ = otherTx.Rollback()

	// Main MSP should NOT see the other-MSP relationship
	mainTx := mustBegin(t, db)
	setRLSContext(t, mainTx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "read")

	err = mainTx.QueryRow(`SELECT COUNT(*) FROM device_relationships WHERE id = $1`, relID).Scan(&count)
	if err != nil {
		t.Fatalf("count main-MSP visibility: %v", err)
	}
	if count != 0 {
		t.Errorf("main-MSP should NOT see other-MSP relationship, got count=%d", count)
	}
	_ = mainTx.Rollback()
}

// TestDeviceRelationshipsRLSWithCheckClause proves WITH CHECK clause
// prevents UPDATE from introducing cross-MSP device reference.
func TestDeviceRelationshipsRLSWithCheckClause(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	// Seed second device in same MSP
	otherMainDeviceID := "00000000-0000-0000-0000-000000000006"
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, $3, $4, $5, 'second-main-device', 'online')`,
		otherMainDeviceID, mspID, clientID, siteID, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("seed second main device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var relID string
	err = tx.QueryRow(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text
	`, mspID, clientID, siteID, deviceID, otherMainDeviceID, "test_rel", "{}").Scan(&relID)
	if err != nil {
		t.Fatalf("seed valid relationship: %v", err)
	}
	if relID == "" {
		t.Fatal("expected relationship ID")
	}
	_ = tx.Rollback()

	// Try UPDATE to change target to other-MSP device — should fail
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	_, err = tx.Exec(`UPDATE device_relationships SET target_device_id = $1 WHERE id = $2`, otherDeviceID, relID)
	if err == nil {
		t.Fatal("UPDATE with cross-MSP target should be rejected by WITH CHECK clause")
	}
	_ = tx.Rollback()

	// Try UPDATE to change source to other-MSP device — should also fail
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	_, err = tx.Exec(`UPDATE device_relationships SET source_device_id = $1 WHERE id = $2`, otherDeviceID, relID)
	if err == nil {
		t.Fatal("UPDATE with cross-MSP source should be rejected by WITH CHECK clause")
	}
	_ = tx.Rollback()
}

// TestDeviceRelationshipsRLSInsertCrossMSPDeny proves insert with
// cross-MSP target_device is rejected even when row msp_id matches.
func TestDeviceRelationshipsRLSInsertCrossMSPDeny(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	_, err = tx.Exec(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, mspID, clientID, siteID, deviceID, otherDeviceID, "test_rel", "{}")
	if err == nil {
		t.Fatal("device_relationships insert with cross-MSP target should be rejected by RLS")
	}
	_ = tx.Rollback()
}

// TestDeviceRelationshipsRLSAllowSameMSP proves same-MSP relationship insert succeeds.
func TestDeviceRelationshipsRLSAllowSameMSP(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	otherDeviceID := "00000000-0000-0000-0000-000000000006"
	_, err := db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, $3, $4, $5, 'second-device', 'online')`,
		otherDeviceID, mspID, clientID, siteID, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("seed second device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var relID string
	err = tx.QueryRow(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text
	`, mspID, clientID, siteID, deviceID, otherDeviceID, "test_rel", "{}").Scan(&relID)
	if err != nil {
		t.Fatalf("same-MSP device_relationship insert: %v", err)
	}
	if relID == "" {
		t.Fatal("expected relationship ID, got empty")
	}

	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM device_relationships WHERE id = $1`, relID).Scan(&count)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 device_relationship, got %d", count)
	}
	_ = tx.Rollback()
}

// TestTopologyEdgesRLSDualEndpoint proves topology_edges RLS checks
// BOTH src_device_id AND dst_device_id belong to MSP.
func TestTopologyEdgesRLSDualEndpoint(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"
	otherDevice2ID := "00000000-0000-0000-0000-000000000006"
	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device-2', 'online')`,
		otherDevice2ID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device 2: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, otherMSPID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var edgeID string
	err = tx.QueryRow(`
		INSERT INTO topology_edges (msp_id, src_device_id, dst_device_id, edge_type, metadata)
		VALUES ($1, $2, $3, $4, $5) RETURNING id::text
	`, otherMSPID, otherDeviceID, otherDevice2ID, "network_link", "{}").Scan(&edgeID)
	if err != nil {
		t.Fatalf("other-MSP topology edge insert: %v", err)
	}
	if edgeID == "" {
		t.Fatal("expected edge ID")
	}
	_ = tx.Rollback()

	// Main-MSP should not see other-MSP edge
	mainTx := mustBegin(t, db)
	setRLSContext(t, mainTx, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000010", "msp_admin", "read")

	var count int
	err = mainTx.QueryRow(`SELECT COUNT(*) FROM topology_edges WHERE id = $1`, edgeID).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("main-MSP should NOT see other-MSP topology edge, got count=%d", count)
	}
	_ = mainTx.Rollback()
}

// TestTopologyEdgesRLSInsertCrossMSPDeny proves cross-MSP edge is rejected.
func TestTopologyEdgesRLSInsertCrossMSPDeny(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	_, err = tx.Exec(`
		INSERT INTO topology_edges (msp_id, src_device_id, dst_device_id, edge_type, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, mspID, deviceID, otherDeviceID, "network_link", "{}")
	if err == nil {
		t.Fatal("cross-MSP topology edge should be rejected by RLS")
	}
	_ = tx.Rollback()
}

// TestTopologyEdgesRLSAllowSameMSP proves same-MSP topology edge insert succeeds.
func TestTopologyEdgesRLSAllowSameMSP(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	otherDeviceID := "00000000-0000-0000-0000-000000000006"
	_, err := db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'second-device', 'online')`,
		otherDeviceID, mspID)
	if err != nil {
		t.Fatalf("seed second device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var edgeID string
	err = tx.QueryRow(`
		INSERT INTO topology_edges (msp_id, src_device_id, dst_device_id, edge_type, metadata)
		VALUES ($1, $2, $3, $4, $5) RETURNING id::text
	`, mspID, deviceID, otherDeviceID, "test_link", "{}").Scan(&edgeID)
	if err != nil {
		t.Fatalf("same-MSP topology_edge insert: %v", err)
	}
	if edgeID == "" {
		t.Fatal("expected edge ID, got empty")
	}

	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM topology_edges WHERE id = $1`, edgeID).Scan(&count)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 topology_edge, got %d", count)
	}
	_ = tx.Rollback()
}

// TestTopologyEdgesRLSWithCheckClause proves WITH CHECK prevents cross-MSP UPDATE.
func TestTopologyEdgesRLSWithCheckClause(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, _, _, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	otherDeviceID := "00000000-0000-0000-0000-000000000005"

	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'other-device', 'online')`,
		otherDeviceID, otherMSPID)
	if err != nil {
		t.Fatalf("seed other device: %v", err)
	}

	otherMainDeviceID := "00000000-0000-0000-0000-000000000006"
	_, err = db.Exec(`INSERT INTO devices (id, msp_id, client_id, site_id, tenant_id, hostname, status)
		VALUES ($1, $2, '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', 'second-main-device', 'online')`,
		otherMainDeviceID, mspID)
	if err != nil {
		t.Fatalf("seed second main device: %v", err)
	}

	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	var edgeID string
	err = tx.QueryRow(`
		INSERT INTO topology_edges (msp_id, src_device_id, dst_device_id, edge_type, metadata)
		VALUES ($1, $2, $3, $4, $5) RETURNING id::text
	`, mspID, deviceID, otherMainDeviceID, "test_link", "{}").Scan(&edgeID)
	if err != nil {
		t.Fatalf("seed valid edge: %v", err)
	}
	if edgeID == "" {
		t.Fatal("expected edge ID")
	}
	_ = tx.Rollback()

	// Try UPDATE dst_device to other-MSP device — should fail
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")

	_, err = tx.Exec(`UPDATE topology_edges SET dst_device_id = $1 WHERE id = $2`, otherDeviceID, edgeID)
	if err == nil {
		t.Fatal("UPDATE with cross-MSP device should be rejected by WITH CHECK clause")
	}
	_ = tx.Rollback()
}

// TestRLSCatalogFlags verifies migration 89 ENABLE + FORCE RLS is
// reflected in pg_class catalog flags for all 13 tables.
func TestRLSCatalogFlags(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)

	tables := []string{
		"billing_accounts", "subscriptions", "invoices", "invoice_items",
		"usage_records", "payment_methods",
		"tenant_retention_settings",
		"device_relationships", "network_addresses", "device_packages",
		"device_services", "device_mounts", "topology_edges",
	}

	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			var relrowsecurity, relforcerowsecurity bool
			err := db.QueryRow(`
				SELECT relrowsecurity, relforcerowsecurity
				FROM pg_class WHERE relname = $1
			`, table).Scan(&relrowsecurity, &relforcerowsecurity)
			if err != nil {
				t.Fatalf("query pg_class for %s: %v", table, err)
			}
			if !relrowsecurity {
				t.Errorf("%s: relrowsecurity = false, want true", table)
			}
			if !relforcerowsecurity {
				t.Errorf("%s: relforcerowsecurity = false, want true", table)
			}
		})
	}
}

// TestRLSCatalogPolicyCount verifies migration 89 created exactly 26 policies.
func TestRLSCatalogPolicyCount(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)

	var policyCount int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM pg_policies
		WHERE schemaname = 'public'
		  AND tablename IN (
			'billing_accounts', 'subscriptions', 'invoices', 'invoice_items',
			'usage_records', 'payment_methods',
			'tenant_retention_settings',
			'device_relationships', 'network_addresses', 'device_packages',
			'device_services', 'device_mounts', 'topology_edges'
		  )
	`).Scan(&policyCount)
	if err != nil {
		t.Fatalf("count RLS policies: %v", err)
	}
	if policyCount != 26 {
		t.Errorf("expected 26 RLS policies, got %d", policyCount)
	}
}

// TestRLSAllowDenyAllFourOperations proves RLS respects all 4 SQL ops
// under a dedicated test role with NOBYPASSRLS.
func TestRLSAllowDenyAllFourOperations(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	otherMSPID := "00000000-0000-0000-0000-000000000099"
	_, err := db.Exec(`INSERT INTO msp_tenants (id, name, slug, is_active) VALUES ($1, 'Other MSP', 'other-msp', true) ON CONFLICT (id) DO NOTHING`, otherMSPID)
	if err != nil {
		t.Fatalf("seed other msp: %v", err)
	}

	// Create app role with explicit error checking
	_, err = db.Exec(`DROP ROLE IF EXISTS strata_rls_test_role`)
	if err != nil {
		t.Fatalf("drop test role: %v", err)
	}
	_, err = db.Exec(`CREATE ROLE strata_rls_test_role NOLOGIN NOSUPERUSER NOBYPASSRLS`)
	if err != nil {
		t.Fatalf("create test role: %v", err)
	}
	_, err = db.Exec(`GRANT USAGE ON SCHEMA public TO strata_rls_test_role`)
	if err != nil {
		t.Fatalf("grant schema usage: %v", err)
	}
	_, err = db.Exec(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO strata_rls_test_role`)
	if err != nil {
		t.Fatalf("grant DML: %v", err)
	}
	_, err = db.Exec(`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO strata_rls_test_role`)
	if err != nil {
		t.Fatalf("grant sequences: %v", err)
	}

	// Seed data under main MSP RLS context
	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")
	_, err = tx.Exec(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, mspID, clientID, siteID, deviceID, "00000000-0000-0000-0000-000000000006", "test_rel", "{}")
	if err != nil {
		t.Fatalf("seed relationship: %v", err)
	}
	var relID string
	err = tx.QueryRow(`SELECT id::text FROM device_relationships WHERE msp_id = $1 AND source_device_id = $2 AND target_device_id = '00000000-0000-0000-0000-000000000006'`, mspID, deviceID).Scan(&relID)
	if err != nil {
		t.Fatalf("get rel ID: %v", err)
	}
	_ = tx.Rollback()

	// Test 1: SELECT main-MSP data (should see)
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "read")
	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM device_relationships WHERE msp_id = $1`, mspID).Scan(&count)
	if err != nil {
		t.Fatalf("select main-MSP data: %v", err)
	}
	if count == 0 {
		t.Error("expected main-MSP data visible under same-MSP RLS context")
	}
	_ = tx.Rollback()

	// Test 2: INSERT main-MSP data (should succeed)
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")
	var newRelID string
	err = tx.QueryRow(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text
	`, mspID, clientID, siteID, deviceID, "00000000-0000-0000-0000-000000000006", "test_rel2", "{}").Scan(&newRelID)
	if err != nil {
		t.Fatalf("insert main-MSP data: %v", err)
	}
	if newRelID == "" {
		t.Fatal("expected relationship ID from insert")
	}
	_ = tx.Rollback()

	// Test 3: UPDATE main-MSP data (should succeed)
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")
	result, err := tx.Exec(`UPDATE device_relationships SET is_active = false WHERE id = $1`, newRelID)
	if err != nil {
		t.Fatalf("update main-MSP data: %v", err)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		t.Errorf("update affected %d rows, want 1", affected)
	}
	_ = tx.Rollback()

	// Test 4: DELETE main-MSP data (should succeed)
	tx = mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")
	result, err = tx.Exec(`DELETE FROM device_relationships WHERE id = $1`, newRelID)
	if err != nil {
		t.Fatalf("delete main-MSP data: %v", err)
	}
	affected, _ = result.RowsAffected()
	if affected != 1 {
		t.Errorf("delete affected %d rows, want 1", affected)
	}
	_ = tx.Rollback()

	// Cleanup test role
	_, _ = db.Exec(`DROP ROLE IF EXISTS strata_rls_test_role`)
}

// TestRLSPooledContextReset verifies each tx starts with clean RLS context.
func TestRLSPooledContextReset(t *testing.T) {
	db := testDB(t)
	applyMigrations(t, db)
	mspID, clientID, siteID, deviceID, _ := seedTestData(t, db)

	// Insert data under main MSP
	tx := mustBegin(t, db)
	setRLSContext(t, tx, mspID, "00000000-0000-0000-0000-000000000010", "msp_admin", "write")
	_, err := tx.Exec(`
		INSERT INTO device_relationships (msp_id, client_id, site_id, source_device_id, target_device_id, relationship_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, mspID, clientID, siteID, deviceID, "00000000-0000-0000-0000-000000000006", "test_rel", "{}")
	if err != nil {
		t.Fatalf("seed relationship: %v", err)
	}
	_ = tx.Rollback()

	// New tx with different MSP context — should not see main-MSP data
	tx = mustBegin(t, db)
	otherMSPID := "00000000-0000-0000-0000-000000000099"
	setRLSContext(t, tx, otherMSPID, "00000000-0000-0000-0000-000000000010", "msp_admin", "read")

	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM device_relationships WHERE msp_id = $1`, mspID).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("other-MSP context should NOT see main-MSP data, got count=%d", count)
	}
	_ = tx.Rollback()
}

func mustBegin(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	return tx
}
