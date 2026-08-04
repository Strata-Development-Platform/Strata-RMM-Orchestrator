//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://strata_test:strata_test@localhost:5432/strata_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}


func TestPolicyEnforcementEngineFindMatchingDevicesMSP(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	engine := NewPolicyEnforcementEngine(db, zap.NewNop())

	ctx := context.Background()
	mspID := "test-msp-1"

	// Create test MSP
	_, err := db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
		ON CONFLICT (id) DO NOTHING
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	// Create test devices
	for i := 0; i < 3; i++ {
		deviceID := "test-device-" + string(rune('0'+i))
		clientID := "test-client-1"
		siteID := "test-site-1"

		_, err := db.ExecContext(ctx, `
			INSERT INTO client_organizations (id, msp_id, name, is_active) VALUES ($1, $2, 'Test Client', true)
			ON CONFLICT (id) DO NOTHING
		`, clientID, mspID)
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO sites (id, client_id, name, is_active) VALUES ($1, $2, 'Test Site', true)
			ON CONFLICT (id) DO NOTHING
		`, siteID, clientID)
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO devices (id, msp_id, client_id, site_id, name, is_active) VALUES ($1, $2, $3, $4, 'Test Device', true)
			ON CONFLICT (id) DO NOTHING
		`, deviceID, mspID, clientID, siteID)
		if err != nil {
			t.Fatal(err)
		}
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	devices, err := engine.findDevicesForMSP(ctx, tx, mspID)
	if err != nil {
		t.Fatalf("findDevicesForMSP: %v", err)
	}

	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
}

func TestPolicyEnforcementEngineFindMatchingDevicesClient(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	engine := NewPolicyEnforcementEngine(db, zap.NewNop())

	ctx := context.Background()
	mspID := "test-msp-2"
	clientID := "test-client-2"

	_, err := db.ExecContext(ctx, `
		INSERT INTO msp_tenants (id, name, is_active) VALUES ($1, 'Test MSP', true)
		ON CONFLICT (id) DO NOTHING
	`, mspID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO client_organizations (id, msp_id, name, is_active) VALUES ($1, $2, 'Test Client', true)
		ON CONFLICT (id) DO NOTHING
	`, clientID, mspID)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		deviceID := "test-device-c-" + string(rune('0'+i))
		siteID := "test-site-c-" + string(rune('0'+i))

		_, err := db.ExecContext(ctx, `
			INSERT INTO sites (id, client_id, name, is_active) VALUES ($1, $2, 'Test Site', true)
			ON CONFLICT (id) DO NOTHING
		`, siteID, clientID)
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO devices (id, msp_id, client_id, site_id, name, is_active) VALUES ($1, $2, $3, $4, 'Test Device', true)
			ON CONFLICT (id) DO NOTHING
		`, deviceID, mspID, clientID, siteID)
		if err != nil {
			t.Fatal(err)
		}
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	devices, err := engine.findDevicesForClient(ctx, tx, mspID, clientID)
	if err != nil {
		t.Fatalf("findDevicesForClient: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestPolicySchedulerStartStop(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	engine := NewPolicyEnforcementEngine(db, zap.NewNop())
	scheduler := NewPolicyScheduler(100*time.Millisecond, engine, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)

	// Let it run briefly
	time.Sleep(200 * time.Millisecond)

	scheduler.Stop()
}

func TestPolicyEnforcementEngineEmptyMSP(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	engine := NewPolicyEnforcementEngine(db, zap.NewNop())

	ctx := context.Background()

	err := engine.ApplyPoliciesToDevices(ctx, "nonexistent-msp")
	if err != nil {
		t.Fatalf("expected nil error for empty MSP, got: %v", err)
	}
}
