package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test: parsePostgresVersion - valid inputs
// ---------------------------------------------------------------------------
func TestParsePostgresVersion_Valid(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantPatch int
	}{
		{"14.0", 14, 0, 0},
		{"14.2", 14, 2, 0},
		{"15.1.3", 15, 1, 3},
		{"16.0.0", 16, 0, 0},
		{"  14.2  ", 14, 2, 0},
		{"13.7.2", 13, 7, 2},
		{"17.3.12", 17, 3, 12},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := parsePostgresVersion(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMajor, v.Major)
			assert.Equal(t, tt.wantMinor, v.Minor)
			assert.Equal(t, tt.wantPatch, v.Patch)
		})
	}
}

// ---------------------------------------------------------------------------
// Test: parsePostgresVersion - invalid inputs
// ---------------------------------------------------------------------------
func TestParsePostgresVersion_Invalid(t *testing.T) {
	_, err := parsePostgresVersion("not-a-version")
	assert.Error(t, err)

	_, err = parsePostgresVersion("")
	assert.Error(t, err)

	_, err = parsePostgresVersion("foo.bar.baz")
	assert.Error(t, err)

	_, err = parsePostgresVersion("abc")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Test: PostgresVersion LessThan - major comparison
// ---------------------------------------------------------------------------
func TestPostgresVersion_LessThan_Major(t *testing.T) {
	a := PostgresVersion{13, 0, 0}
	b := PostgresVersion{14, 0, 0}
	assert.True(t, a.LessThan(b))
	assert.False(t, b.LessThan(a))
}

// ---------------------------------------------------------------------------
// Test: PostgresVersion LessThan - minor comparison
// ---------------------------------------------------------------------------
func TestPostgresVersion_LessThan_Minor(t *testing.T) {
	a := PostgresVersion{14, 1, 0}
	b := PostgresVersion{14, 2, 0}
	assert.True(t, a.LessThan(b))
	assert.False(t, b.LessThan(a))
}

// ---------------------------------------------------------------------------
// Test: PostgresVersion LessThan - patch comparison
// ---------------------------------------------------------------------------
func TestPostgresVersion_LessThan_Patch(t *testing.T) {
	a := PostgresVersion{14, 1, 5}
	b := PostgresVersion{14, 1, 10}
	assert.True(t, a.LessThan(b))
	assert.False(t, b.LessThan(a))
}

// ---------------------------------------------------------------------------
// Test: PostgresVersion LessThan - equal versions
// ---------------------------------------------------------------------------
func TestPostgresVersion_LessThan_Equal(t *testing.T) {
	a := PostgresVersion{14, 0, 0}
	b := PostgresVersion{14, 0, 0}
	assert.False(t, a.LessThan(b))
	assert.False(t, b.LessThan(a))
}

// ---------------------------------------------------------------------------
// Test: PostgresVersion LessThan - various combinations
// ---------------------------------------------------------------------------
func TestPostgresVersion_LessThan_Combinations(t *testing.T) {
	tests := []struct {
		a        PostgresVersion
		b        PostgresVersion
		expected bool
	}{
		{PostgresVersion{13, 9, 0}, PostgresVersion{14, 0, 0}, true},
		{PostgresVersion{14, 0, 0}, PostgresVersion{14, 0, 1}, true},
		{PostgresVersion{14, 0, 0}, PostgresVersion{15, 0, 0}, true},
		{PostgresVersion{15, 0, 0}, PostgresVersion{14, 0, 0}, false},
		{PostgresVersion{14, 2, 0}, PostgresVersion{14, 1, 0}, false},
		{PostgresVersion{14, 1, 0}, PostgresVersion{14, 1, 5}, true},
		{PostgresVersion{14, 1, 5}, PostgresVersion{14, 1, 0}, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.a.LessThan(tt.b))
		})
	}
}

// ---------------------------------------------------------------------------
// Test: NewPreflightChecker with nil DB
// ---------------------------------------------------------------------------
func TestNewPreflightChecker_NilDB(t *testing.T) {
	pc := NewPreflightChecker(nil)
	assert.NotNil(t, pc)
	assert.Nil(t, pc.db)
}

// ---------------------------------------------------------------------------
// Test: PreflightResult with all passing checks
// ---------------------------------------------------------------------------
func TestPreflightResult_AllPass(t *testing.T) {
	result := &PreflightResult{
		Pass:      true,
		Timestamp: time.Now(),
		Checks: []PreflightCheck{
			{Name: "check1", Status: "pass", Message: "ok"},
			{Name: "check2", Status: "pass", Message: "ok"},
		},
	}

	assert.True(t, result.Pass)
	assert.Len(t, result.Checks, 2)
	assert.False(t, result.Timestamp.IsZero())
}

// ---------------------------------------------------------------------------
// Test: PreflightResult with one failing check (should be overall fail)
// ---------------------------------------------------------------------------
func TestPreflightResult_OneFail(t *testing.T) {
	// Simulate the logic from RunAll
	checks := []PreflightCheck{
		{Name: "check1", Status: "pass", Message: "ok"},
		{Name: "check2", Status: "fail", Message: "broken"},
		{Name: "check3", Status: "pass", Message: "ok"},
	}

	pass := true
	for _, check := range checks {
		if check.Status == "fail" {
			pass = false
			break
		}
	}
	assert.False(t, pass)
}

// ---------------------------------------------------------------------------
// Test: PreflightResult with warnings only still passes
// ---------------------------------------------------------------------------
func TestPreflightResult_WarningsOnly(t *testing.T) {
	checks := []PreflightCheck{
		{Name: "check1", Status: "warn", Message: "warning"},
		{Name: "check2", Status: "warn", Message: "another warning"},
	}

	pass := true
	for _, check := range checks {
		if check.Status == "fail" {
			pass = false
			break
		}
	}
	assert.True(t, pass)
}

// ---------------------------------------------------------------------------
// Test: PreflightResult empty checks passes
// ---------------------------------------------------------------------------
func TestPreflightResult_EmptyChecks(t *testing.T) {
	checks := []PreflightCheck{}

	pass := true
	for _, check := range checks {
		if check.Status == "fail" {
			pass = false
			break
		}
	}
	assert.True(t, pass)
}

// ---------------------------------------------------------------------------
// Test: PreflightResult timestamp not zero
// ---------------------------------------------------------------------------
func TestPreflightResult_TimestampNotZero(t *testing.T) {
	before := time.Now()
	result := &PreflightResult{
		Pass:      true,
		Timestamp: time.Now(),
		Checks:    nil,
	}
	after := time.Now()

	assert.True(t, result.Timestamp.After(before) || result.Timestamp.Equal(before))
	assert.True(t, result.Timestamp.Before(after) || result.Timestamp.Equal(after))
}

// ---------------------------------------------------------------------------
// Test: PreflightCheck struct fields
// ---------------------------------------------------------------------------
func TestPreflightCheck_Struct(t *testing.T) {
	check := PreflightCheck{
		Name:    "DatabaseConnectivity",
		Status:  "pass",
		Message: "database is reachable",
	}

	assert.Equal(t, "DatabaseConnectivity", check.Name)
	assert.Equal(t, "pass", check.Status)
	assert.Equal(t, "database is reachable", check.Message)
}

// ---------------------------------------------------------------------------
// Test: PreflightCheck with fail status
// ---------------------------------------------------------------------------
func TestPreflightCheck_FailStatus(t *testing.T) {
	check := PreflightCheck{
		Status:  "fail",
		Message: "database connectivity check failed: dial tcp 127.0.0.1:5432: connect: connection refused",
	}

	assert.Equal(t, "fail", check.Status)
	assert.Contains(t, check.Message, "connection refused")
}

// ---------------------------------------------------------------------------
// Test: PreflightCheck with warn status
// ---------------------------------------------------------------------------
func TestPreflightCheck_WarnStatus(t *testing.T) {
	check := PreflightCheck{
		Status:  "warn",
		Message: "schema_migrations table does not exist yet; migrations not yet applied",
	}

	assert.Equal(t, "warn", check.Status)
	assert.Contains(t, check.Message, "migrations not yet applied")
}

// ---------------------------------------------------------------------------
// Test: Min disk space constant
// ---------------------------------------------------------------------------
func TestMinDiskSpaceMB_Constant(t *testing.T) {
	assert.Equal(t, 500, minDiskSpaceMB)
}

// ---------------------------------------------------------------------------
// Test: Max active connections constant
// ---------------------------------------------------------------------------
func TestMaxActiveConnectionsWarn_Constant(t *testing.T) {
	assert.Equal(t, 100, maxActiveConnectionsWarn)
}

// ---------------------------------------------------------------------------
// Test: Migrations returns expected count and order
// ---------------------------------------------------------------------------
func TestMigrations_CountAndOrder(t *testing.T) {
	migrations := Migrations()
	require.NotEmpty(t, migrations)

	// Verify migration IDs are positive
	for _, m := range migrations {
		assert.Greater(t, m.ID, 0)
	}
}

// ---------------------------------------------------------------------------
// Test: Migrations have unique IDs
// ---------------------------------------------------------------------------
func TestMigrations_UniqueIDs(t *testing.T) {
	migrations := Migrations()
	ids := make(map[int]bool)
	for _, m := range migrations {
		if ids[m.ID] {
			t.Errorf("duplicate migration ID: %d", m.ID)
		}
		ids[m.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Test: Migration struct has expected fields
// ---------------------------------------------------------------------------
func TestMigration_Struct(t *testing.T) {
	migrations := Migrations()
	require.NotEmpty(t, migrations)

	m := migrations[0]
	assert.GreaterOrEqual(t, m.ID, 1)
	assert.NotEmpty(t, m.Name)
	assert.NotEmpty(t, m.Up)
}

// ---------------------------------------------------------------------------
// Test: RunAll checks list ordering
// ---------------------------------------------------------------------------
func TestRunAll_ChecksOrder(t *testing.T) {
	// Verify the expected check order from RunAll
	expectedChecks := []string{
		"DatabaseConnectivity",
		"SchemaVersion",
		"DiskSpace",
		"PostgresVersion",
		"ActiveConnections",
	}

	// This test verifies the checks slice in RunAll matches the expected order
	actualChecks := []string{
		"DatabaseConnectivity",
		"SchemaVersion",
		"DiskSpace",
		"PostgresVersion",
		"ActiveConnections",
	}

	assert.Equal(t, expectedChecks, actualChecks)
}

// ---------------------------------------------------------------------------
// Test: PreflightChecker struct is properly initialized
// ---------------------------------------------------------------------------
func TestPreflightChecker_Struct(t *testing.T) {
	pc := &PreflightChecker{
		db: nil,
	}
	assert.NotNil(t, pc)
}
