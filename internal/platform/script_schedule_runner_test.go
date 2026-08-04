//go:build dbintegration

package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func setupScheduleTestDB(t *testing.T) *sql.DB {
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

func TestScriptScheduleRunnerStartStop(t *testing.T) {
	db := setupScheduleTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	so := NewScheduleOrchestrator(nil, db, zap.NewNop())
	runner := NewScriptScheduleRunner(100*time.Millisecond, so, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner.Start(ctx)

	if !runner.Healthy() {
		t.Fatal("runner should be healthy after start")
	}

	time.Sleep(200 * time.Millisecond)

	runner.Stop()

	time.Sleep(100 * time.Millisecond)

	if runner.Healthy() {
		t.Fatal("runner should not be healthy after stop")
	}
}

func TestScriptScheduleRunnerGetDueSchedules(t *testing.T) {
	db := setupScheduleTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	so := NewScheduleOrchestrator(nil, db, zap.NewNop())
	runner := NewScriptScheduleRunner(5*time.Minute, so, zap.NewNop())

	ctx := context.Background()

	// Create a script
	scriptID := "test-script-schedule-1"
	_, err := db.ExecContext(ctx, `
		INSERT INTO scripts (id, tenant_id, name, language, content, timeout_sec)
		VALUES ($1, 'test-tenant-1', 'Test Script', 'bash', 'echo hello', 30)
		ON CONFLICT (id) DO NOTHING
	`, scriptID)
	if err != nil {
		t.Fatal(err)
	}

	// Create an active schedule with next_run_at in the past
	scheduleID := "test-schedule-due-1"
	nextRunPast := time.Now().Add(-2 * time.Hour)
	_, err = db.ExecContext(ctx, `
		INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
		                       target_devices, max_retries, retry_interval, status, next_run_at)
		VALUES ($1, 'test-tenant-1', 'Due Schedule', $2, 'daily',
		        '{"time":"09:00"}', '["device-1","device-2"]', 3, 60, 'active', $3)
		ON CONFLICT (id) DO NOTHING
	`, scheduleID, scriptID, nextRunPast)
	if err != nil {
		t.Fatal(err)
	}

	// Create an active schedule with next_run_at in the future
	scheduleID2 := "test-schedule-future-1"
	nextRunFuture := time.Now().Add(2 * time.Hour)
	_, err = db.ExecContext(ctx, `
		INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
		                       target_devices, max_retries, retry_interval, status, next_run_at)
		VALUES ($1, 'test-tenant-1', 'Future Schedule', $2, 'daily',
		        '{"time":"09:00"}', '["device-1"]', 3, 60, 'active', $3)
		ON CONFLICT (id) DO NOTHING
	`, scheduleID2, scriptID, nextRunFuture)
	if err != nil {
		t.Fatal(err)
	}

	// Create a paused schedule (should not be returned)
	scheduleID3 := "test-schedule-paused-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
		                       target_devices, max_retries, retry_interval, status, next_run_at)
		VALUES ($1, 'test-tenant-1', 'Paused Schedule', $2, 'daily',
		        '{"time":"09:00"}', '["device-1"]', 3, 60, 'paused', $3)
		ON CONFLICT (id) DO NOTHING
	`, scheduleID3, scriptID, nextRunPast)
	if err != nil {
		t.Fatal(err)
	}

	ids, err := runner.getDueSchedules(ctx)
	if err != nil {
		t.Fatalf("getDueSchedules: %v", err)
	}

	if len(ids) != 1 {
		t.Fatalf("expected 1 due schedule, got %d: %v", len(ids), ids)
	}

	if ids[0] != scheduleID {
		t.Fatalf("expected schedule ID %s, got %s", scheduleID, ids[0])
	}
}

func TestScriptScheduleRunnerNoDueSchedules(t *testing.T) {
	db := setupScheduleTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	so := NewScheduleOrchestrator(nil, db, zap.NewNop())
	runner := NewScriptScheduleRunner(5*time.Minute, so, zap.NewNop())

	ctx := context.Background()

	// Create a script
	scriptID := "test-script-no-due-1"
	_, err := db.ExecContext(ctx, `
		INSERT INTO scripts (id, tenant_id, name, language, content, timeout_sec)
		VALUES ($1, 'test-tenant-1', 'Test Script', 'bash', 'echo hello', 30)
		ON CONFLICT (id) DO NOTHING
	`, scriptID)
	if err != nil {
		t.Fatal(err)
	}

	// Create a schedule with next_run_at in the future (no due schedules)
	scheduleID := "test-schedule-no-due-1"
	nextRunFuture := time.Now().Add(1 * time.Hour)
	_, err = db.ExecContext(ctx, `
		INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
		                       target_devices, max_retries, retry_interval, status, next_run_at)
		VALUES ($1, 'test-tenant-1', 'Future Schedule', $2, 'daily',
		        '{"time":"09:00"}', '["device-1"]', 3, 60, 'active', $3)
		ON CONFLICT (id) DO NOTHING
	`, scheduleID, scriptID, nextRunFuture)
	if err != nil {
		t.Fatal(err)
	}

	ids, err := runner.getDueSchedules(ctx)
	if err != nil {
		t.Fatalf("getDueSchedules: %v", err)
	}

	if len(ids) != 0 {
		t.Fatalf("expected 0 due schedules, got %d: %v", len(ids), ids)
	}
}

func TestScriptScheduleRunnerDueSchedulesWithTenantIsolation(t *testing.T) {
	db := setupScheduleTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	so := NewScheduleOrchestrator(nil, db, zap.NewNop())
	runner := NewScriptScheduleRunner(5*time.Minute, so, zap.NewNop())

	ctx := context.Background()

	// Create scripts for two tenants
	scriptID1 := "test-script-tenant-1"
	scriptID2 := "test-script-tenant-2"
	_, err := db.ExecContext(ctx, `
		INSERT INTO scripts (id, tenant_id, name, language, content, timeout_sec)
		VALUES ($1, 'tenant-a', 'Script A', 'bash', 'echo a', 30),
		       ($2, 'tenant-b', 'Script B', 'bash', 'echo b', 30)
		ON CONFLICT (id) DO NOTHING
	`, scriptID1, scriptID2)
	if err != nil {
		t.Fatal(err)
	}

	// Create active schedules for both tenants with past next_run_at
	now := time.Now()
	pastA := now.Add(-1 * time.Hour)
	pastB := now.Add(-30 * time.Minute)

	_, err = db.ExecContext(ctx, `
		INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
		                       target_devices, max_retries, retry_interval, status, next_run_at)
		VALUES
		  ('sched-a-1', 'tenant-a', 'Schedule A', $1, 'hourly', 'null', '["d1"]', 3, 60, 'active', $2),
		  ('sched-b-1', 'tenant-b', 'Schedule B', $3, 'hourly', 'null', '["d1"]', 3, 60, 'active', $4)
		ON CONFLICT (id) DO NOTHING
	`, scriptID1, pastA, scriptID2, pastB)
	if err != nil {
		t.Fatal(err)
	}

	ids, err := runner.getDueSchedules(ctx)
	if err != nil {
		t.Fatalf("getDueSchedules: %v", err)
	}

	if len(ids) != 2 {
		t.Fatalf("expected 2 due schedules, got %d: %v", len(ids), ids)
	}
}

func TestScriptScheduleRunnerMultipleDueSchedules(t *testing.T) {
	db := setupScheduleTestDB(t)
	defer db.Close()
	applyMigrations(t, db)

	so := NewScheduleOrchestrator(nil, db, zap.NewNop())
	runner := NewScriptScheduleRunner(5*time.Minute, so, zap.NewNop())

	ctx := context.Background()

	scriptID := "test-script-multi-due-1"
	_, err := db.ExecContext(ctx, `
		INSERT INTO scripts (id, tenant_id, name, language, content, timeout_sec)
		VALUES ($1, 'test-tenant-1', 'Test Script', 'bash', 'echo hello', 30)
		ON CONFLICT (id) DO NOTHING
	`, scriptID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for i := 1; i <= 5; i++ {
		scheduleID := "test-schedule-multi-" + string(rune('0'+i))
		pastRun := now.Add(time.Duration(-i) * time.Hour)
		_, err = db.ExecContext(ctx, `
			INSERT INTO schedules (id, tenant_id, name, script_id, schedule_type, schedule_params,
			                       target_devices, max_retries, retry_interval, status, next_run_at)
			VALUES ($1, 'test-tenant-1', 'Multi Schedule', $2, 'daily',
			        '{"time":"09:00"}', '["device-1"]', 3, 60, 'active', $3)
			ON CONFLICT (id) DO NOTHING
		`, scheduleID, scriptID, pastRun)
		if err != nil {
			t.Fatal(err)
		}
	}

	ids, err := runner.getDueSchedules(ctx)
	if err != nil {
		t.Fatalf("getDueSchedules: %v", err)
	}

	if len(ids) != 5 {
		t.Fatalf("expected 5 due schedules, got %d: %v", len(ids), ids)
	}
}
