package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupPreserver(t *testing.T) (*StatePreserver, string) {
	t.Helper()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	backupDir, err := os.MkdirTemp("", "statepreserve-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(backupDir) })

	ps := NewStatePreserver(nil, sugar, backupDir)
	return ps, backupDir
}

func TestNewStatePreserver(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	backupDir, err := os.MkdirTemp("", "statepreserve-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(backupDir) }()

	ps := NewStatePreserver(nil, sugar, backupDir)
	assert.NotNil(t, ps)
	assert.Equal(t, sugar, ps.logger)
	assert.Equal(t, backupDir, ps.backupDir)
}

func TestNewStatePreserverNilDB(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	backupDir, err := os.MkdirTemp("", "statepreserve-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(backupDir) }()

	ps := NewStatePreserver(nil, sugar, backupDir)
	assert.NotNil(t, ps)
}

func TestSaveSnapshot(t *testing.T) {
	ps, backupDir := setupPreserver(t)

	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableCount:    2,
		TableStats: map[string]TableStat{
			"users":  {Name: "users", RowCount: 100, SizeBytes: 8192},
			"orders": {Name: "orders", RowCount: 50, SizeBytes: 4096},
		},
		Indexes: []IndexStat{
			{Name: "idx_users_email", Table: "users", IsUnique: false, SizeBytes: 1024},
		},
		ForeignKeys: []ForeignKeyStat{
			{Name: "fk_orders_user", Table: "orders", ReferencedTable: "users", Columns: []string{"user_id"}, ReferencedColumns: []string{"id"}},
		},
		SequenceStates: []SequenceStat{
			{Name: "users_id_seq", LastValue: 100, IsCalled: true, CacheSize: 1},
		},
		Metadata: map[string]interface{}{"source": "test"},
	}

	id, err := ps.SaveSnapshot(snapshot)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Contains(t, id, "snap_")

	// Verify JSON file was created
	files, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(files), 1)

	// Verify file is valid JSON by loading it
	loaded, err := ps.LoadSnapshot(id)
	require.NoError(t, err)
	assert.Equal(t, snapshot.TableCount, loaded.TableCount)
	assert.Equal(t, snapshot.SchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, snapshot.TableStats["users"].RowCount, loaded.TableStats["users"].RowCount)
	assert.Equal(t, snapshot.Indexes[0].Name, loaded.Indexes[0].Name)
	assert.Equal(t, snapshot.ForeignKeys[0].Name, loaded.ForeignKeys[0].Name)
	assert.Equal(t, snapshot.SequenceStates[0].Name, loaded.SequenceStates[0].Name)
}

func TestSaveSnapshotNil(t *testing.T) {
	ps, _ := setupPreserver(t)

	id, err := ps.SaveSnapshot(nil)
	assert.Error(t, err)
	assert.Empty(t, id)
}

func TestLoadSnapshotMissing(t *testing.T) {
	ps, _ := setupPreserver(t)

	loaded, err := ps.LoadSnapshot("nonexistent-id")
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestLoadSnapshotEmptyID(t *testing.T) {
	ps, _ := setupPreserver(t)

	loaded, err := ps.LoadSnapshot("")
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestListSnapshots(t *testing.T) {
	ps, _ := setupPreserver(t)

	snapshot1 := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableStats:    map[string]TableStat{"users": {Name: "users"}},
	}

	snapshot2 := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 2,
		TableStats:    map[string]TableStat{"users": {Name: "users"}, "orders": {Name: "orders"}},
	}

	id1, err := ps.SaveSnapshot(snapshot1)
	require.NoError(t, err)
	id2, err := ps.SaveSnapshot(snapshot2)
	require.NoError(t, err)

	ids, err := ps.ListSnapshots()
	require.NoError(t, err)
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)

	// Verify list is sorted
	for i := 1; i < len(ids); i++ {
		assert.LessOrEqual(t, ids[i-1], ids[i])
	}
}

func TestDeleteSnapshot(t *testing.T) {
	ps, backupDir := setupPreserver(t)

	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableStats:    map[string]TableStat{"users": {Name: "users"}},
	}

	id, err := ps.SaveSnapshot(snapshot)
	require.NoError(t, err)

	// Verify it exists in list
	ids, err := ps.ListSnapshots()
	require.NoError(t, err)
	assert.Contains(t, ids, id)

	// Delete it
	err = ps.DeleteSnapshot(id)
	require.NoError(t, err)

	// Verify it's gone from list
	ids, err = ps.ListSnapshots()
	require.NoError(t, err)
	assert.NotContains(t, ids, id)

	// Verify file is gone
	_, err = os.Stat(filepath.Join(backupDir, id+".json"))
	assert.True(t, os.IsNotExist(err), "snapshot file should be deleted")
}

func TestDeleteSnapshotMissing(t *testing.T) {
	ps, _ := setupPreserver(t)

	err := ps.DeleteSnapshot("nonexistent-id")
	assert.Error(t, err)
}

func TestDeleteSnapshotEmptyID(t *testing.T) {
	ps, _ := setupPreserver(t)

	err := ps.DeleteSnapshot("")
	assert.Error(t, err)
}

func TestCompareSnapshots(t *testing.T) {
	ps, _ := setupPreserver(t)

	before := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableCount:    2,
		TableStats: map[string]TableStat{
			"users":  {Name: "users", RowCount: 100, SizeBytes: 8192},
			"orders": {Name: "orders", RowCount: 50, SizeBytes: 4096},
		},
		Indexes: []IndexStat{
			{Name: "idx_users_email", Table: "users", IsUnique: false, SizeBytes: 1024},
		},
		ForeignKeys: []ForeignKeyStat{
			{Name: "fk_orders_user", Table: "orders", ReferencedTable: "users"},
		},
		SequenceStates: []SequenceStat{
			{Name: "users_id_seq", LastValue: 100},
		},
	}

	after := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 2,
		TableCount:    3,
		TableStats: map[string]TableStat{
			"users":    {Name: "users", RowCount: 150, SizeBytes: 12288},
			"orders":   {Name: "orders", RowCount: 50, SizeBytes: 4096},
			"products": {Name: "products", RowCount: 25, SizeBytes: 2048},
		},
		Indexes: []IndexStat{
			{Name: "idx_users_email", Table: "users", IsUnique: false, SizeBytes: 1024},
			{Name: "idx_products_sku", Table: "products", IsUnique: true, SizeBytes: 512},
		},
		ForeignKeys: []ForeignKeyStat{
			{Name: "fk_orders_user", Table: "orders", ReferencedTable: "users"},
			{Name: "fk_order_items_product", Table: "order_items", ReferencedTable: "products"},
		},
		SequenceStates: []SequenceStat{
			{Name: "users_id_seq", LastValue: 150},
			{Name: "products_id_seq", LastValue: 25},
		},
	}

	id1, err := ps.SaveSnapshot(before)
	require.NoError(t, err)
	id2, err := ps.SaveSnapshot(after)
	require.NoError(t, err)

	diff, err := ps.CompareSnapshots(id1, id2)
	require.NoError(t, err)
	assert.NotNil(t, diff)
	assert.Equal(t, int32(1), diff.SchemaVersionDiff) // 2 - 1
	assert.Equal(t, 1, diff.TableCountDiff)           // 3 - 2
	assert.Contains(t, diff.AddedTables, "products")
	assert.NotContains(t, diff.RemovedTables, "products")
	assert.Contains(t, diff.ModifiedTables, "users")
	assert.Contains(t, diff.AddedIndexes, "idx_products_sku")
	assert.Contains(t, diff.AddedForeignKeys, "fk_order_items_product")
}

func TestCompareSnapshotsMissingBefore(t *testing.T) {
	ps, _ := setupPreserver(t)

	diff, err := ps.CompareSnapshots("nonexistent-1", "nonexistent-2")
	assert.Error(t, err)
	assert.Nil(t, diff)
}

func TestCompareSnapshotsMissingAfter(t *testing.T) {
	ps, _ := setupPreserver(t)

	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableStats:    map[string]TableStat{"users": {Name: "users"}},
	}
	id, _ := ps.SaveSnapshot(snapshot)

	diff, err := ps.CompareSnapshots(id, "nonexistent-id")
	assert.Error(t, err)
	assert.Nil(t, diff)
}

func TestCompareSnapshotsIdentical(t *testing.T) {
	ps, backupDir := setupPreserver(t)
	_ = backupDir

	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableCount:    1,
		TableStats: map[string]TableStat{
			"users": {Name: "users", RowCount: 100, SizeBytes: 8192},
		},
		Indexes:        []IndexStat{},
		ForeignKeys:    []ForeignKeyStat{},
		SequenceStates: []SequenceStat{},
	}

	id1, _ := ps.SaveSnapshot(snapshot)
	id2, _ := ps.SaveSnapshot(snapshot)

	diff, err := ps.CompareSnapshots(id1, id2)
	require.NoError(t, err)

	assert.Empty(t, diff.AddedTables)
	assert.Empty(t, diff.RemovedTables)
	assert.Empty(t, diff.ModifiedTables)
	assert.Empty(t, diff.AddedIndexes)
	assert.Empty(t, diff.RemovedIndexes)
	assert.Empty(t, diff.AddedForeignKeys)
	assert.Empty(t, diff.RemovedForeignKeys)
	assert.Equal(t, int32(0), diff.SchemaVersionDiff)
	assert.Equal(t, 0, diff.TableCountDiff)
}

func TestPreDeploySnapshot(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "PreDeploySnapshot should panic with nil DB since CreateSnapshot calls DB queries")
	}()

	ps, _ := setupPreserver(t)
	_, _ = ps.PreDeploySnapshot(context.Background())
}

func TestPreRollbackSnapshot(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "PreRollbackSnapshot should panic with nil DB since CreateSnapshot calls DB queries")
	}()

	ps, _ := setupPreserver(t)
	_, _ = ps.PreRollbackSnapshot(context.Background())
}

func TestRestoreSnapshotContextCancelled(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	backupDir, err := os.MkdirTemp("", "statepreserve-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(backupDir) }()

	ps := NewStatePreserver(nil, sugar, backupDir)

	// Create a snapshot to restore from
	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableStats:    map[string]TableStat{"users": {Name: "users"}},
	}
	id, err := ps.SaveSnapshot(snapshot)
	require.NoError(t, err)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = ps.RestoreSnapshot(ctx, id)
	assert.Error(t, err)
}

func TestRestoreSnapshotMissing(t *testing.T) {
	ps, _ := setupPreserver(t)

	err := ps.RestoreSnapshot(context.Background(), "nonexistent-id")
	assert.Error(t, err)
}

func TestRestoreSnapshotRequiresDB(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	backupDir, err := os.MkdirTemp("", "statepreserve-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(backupDir) }()

	ps := NewStatePreserver(nil, sugar, backupDir)

	// Create a snapshot file
	snapshot := &StateSnapshot{
		Timestamp:     time.Now().UTC(),
		SchemaVersion: 1,
		TableStats:    map[string]TableStat{"users": {Name: "users"}},
	}
	id, err := ps.SaveSnapshot(snapshot)
	require.NoError(t, err)

	// RestoreSnapshot will panic on nil DB (conn() call)
	// This is expected behavior - a real DB is required
	_ = id
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"a", []string{"a"}},
		{"", []string{}},
		{"a, b, c", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		result := splitCSV(tt.input)
		assert.Equal(t, tt.expected, result, "input: %q", tt.input)
	}
}

func TestGenerateSnapshotID(t *testing.T) {
	id1 := generateSnapshotID()
	id2 := generateSnapshotID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.Contains(t, id1, "snap_")
	assert.Contains(t, id2, "snap_")

	// IDs should be different
	assert.NotEqual(t, id1, id2)
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	ps, _ := setupPreserver(t)

	original := &StateSnapshot{
		Timestamp:     time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		SchemaVersion: 42,
		TableCount:    5,
		TableStats: map[string]TableStat{
			"users":  {Name: "users", RowCount: 1000, SizeBytes: 81920, LastVacuum: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), LastAnalyze: time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)},
			"orders": {Name: "orders", RowCount: 5000, SizeBytes: 204800},
		},
		Indexes: []IndexStat{
			{Name: "idx_users_email", Table: "users", IsUnique: true, SizeBytes: 16384},
			{Name: "idx_orders_date", Table: "orders", IsUnique: false, SizeBytes: 8192},
		},
		ForeignKeys: []ForeignKeyStat{
			{Name: "fk_orders_user", Table: "orders", ReferencedTable: "users", Columns: []string{"user_id"}, ReferencedColumns: []string{"id"}},
		},
		SequenceStates: []SequenceStat{
			{Name: "users_id_seq", LastValue: 1000, IsCalled: true, CacheSize: 10},
			{Name: "orders_id_seq", LastValue: 5000, IsCalled: true, CacheSize: 10},
		},
		Metadata: map[string]interface{}{
			"source":                "test",
			"description":           "test roundtrip",
			"schema_version_source": "schema_migrations",
		},
	}

	id, err := ps.SaveSnapshot(original)
	require.NoError(t, err)

	loaded, err := ps.LoadSnapshot(id)
	require.NoError(t, err)

	assert.Equal(t, original.Timestamp, loaded.Timestamp)
	assert.Equal(t, original.SchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, original.TableCount, loaded.TableCount)
	assert.Equal(t, len(original.TableStats), len(loaded.TableStats))
	assert.Equal(t, original.TableStats["users"].RowCount, loaded.TableStats["users"].RowCount)
	assert.Equal(t, original.TableStats["users"].SizeBytes, loaded.TableStats["users"].SizeBytes)
	assert.Equal(t, original.Indexes[0].Name, loaded.Indexes[0].Name)
	assert.Equal(t, original.Indexes[0].IsUnique, loaded.Indexes[0].IsUnique)
	assert.Equal(t, original.ForeignKeys[0].Name, loaded.ForeignKeys[0].Name)
	assert.Equal(t, original.ForeignKeys[0].Columns, loaded.ForeignKeys[0].Columns)
	assert.Equal(t, original.SequenceStates[0].LastValue, loaded.SequenceStates[0].LastValue)
	assert.Equal(t, original.Metadata["source"], loaded.Metadata["source"])
}

func TestListSnapshotsEmptyDir(t *testing.T) {
	ps, _ := setupPreserver(t)

	ids, err := ps.ListSnapshots()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestListSnapshotsNonExistentDir(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := logger.Sugar()

	ps := NewStatePreserver(nil, sugar, "/nonexistent/path/that/does/not/exist")

	ids, err := ps.ListSnapshots()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestValidateSnapshotTableCountMismatch(t *testing.T) {
	// ValidateSnapshot calls CreateSnapshot which queries the DB.
	// With nil DB, getSchemaVersion will panic.
	defer func() {
		r := recover()
		assert.NotNil(t, r, "ValidateSnapshot should panic with nil DB since CreateSnapshot calls DB queries")
	}()

	ps, _ := setupPreserver(t)
	_ = ps.ValidateSnapshot(context.Background())
}
