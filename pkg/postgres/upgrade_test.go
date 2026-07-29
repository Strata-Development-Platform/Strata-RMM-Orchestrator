package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockResult implements sql.Result for testing.
type mockResult struct{}

func (m mockResult) LastInsertId() (int64, error) { return 0, nil }
func (m mockResult) RowsAffected() (int64, error)  { return 0, nil }

// mockRow implements sql.Scanner for testing.
type mockRow struct {
	err    error
	values []interface{}
}

func (r *mockRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		if i < len(r.values) {
			switch d := dest[i].(type) {
			case *bool:
				if b, ok := r.values[i].(bool); ok {
					*d = b
				} else {
					*d = false
				}
			case *string:
				if s, ok := r.values[i].(string); ok {
					*d = s
				}
			case *int64:
				if v, ok := r.values[i].(int64); ok {
					*d = v
				}
			case *int32:
				if v, ok := r.values[i].(int32); ok {
					*d = v
				} else if v, ok := r.values[i].(int64); ok {
					*d = int32(v)
				} else if v, ok := r.values[i].(int); ok {
					*d = int32(v)
				}
			case *interface{}:
				*d = r.values[i]
			default:
				dest[i] = r.values[i]
			}
		}
	}
	return nil
}

func newMockRow(err error, values ...interface{}) *mockRow {
	return &mockRow{err: err, values: values}
}

var (
	errGetVersion    = errors.New("get version error")
	errSetVersion    = errors.New("set version error")
	errLockTimeout   = errors.New("advisory lock acquisition timed out")
)

// mockConn implements dbConnConn for testing.
type mockConn struct {
	queryRowFn func(ctx context.Context, query string, args ...interface{}) rowScanner
	execFn     func(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	closeFn    func() error
}

func (m *mockConn) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockConn) QueryRowContext(ctx context.Context, query string, args ...interface{}) rowScanner {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, query, args...)
	}
	return newMockRow(nil, true)
}

func (m *mockConn) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}
	return mockResult{}, nil
}

func (m *mockConn) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// mockDB implements the dbConn interface for testing.
type mockDB struct {
	connFn func() (dbConnConn, error)
}

func (m *mockDB) Conn(ctx context.Context) (dbConnConn, error) {
	if m.connFn == nil {
		return &mockConn{}, nil
	}
	return m.connFn()
}

// mockVersionStore implements VersionStore for testing.
type mockVersionStore struct {
	mu      sync.Mutex
	version int32
	setErr  error
	getErr  error
}

func newMockVersionStore(initialVersion int32) *mockVersionStore {
	return &mockVersionStore{version: initialVersion}
}

func (m *mockVersionStore) GetVersion() (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return 0, m.getErr
	}
	return m.version, nil
}

func (m *mockVersionStore) SetVersion(v int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.version = v
	return nil
}

func (m *mockVersionStore) SetVersionAndChecksum(v int32, cs string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.version = v
	return nil
}

func (m *mockVersionStore) GetChecksum(key string) (string, error) { return "", nil }
func (m *mockVersionStore) AddChecksum(key, value string) error    { return nil }
func (m *mockVersionStore) ExistsChecksum(key string) (bool, error) { return false, nil }

func newTestLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

// ---------------------------------------------------------------------------
// Test: successful upgrade with all hooks registered
// ---------------------------------------------------------------------------
func TestRunUpgrade_Success(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	for _, phase := range allPhases {
		um.RegisterHook(phase, func(ctx context.Context, version int32) error {
			return nil
		})
	}

	ctx := context.Background()
	result, err := um.RunUpgrade(ctx, 2)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected upgrade to succeed")
	}
	if result.FromVersion != 1 {
		t.Errorf("expected from version 1, got %d", result.FromVersion)
	}
	if result.ToVersion != 2 {
		t.Errorf("expected to version 2, got %d", result.ToVersion)
	}
	if len(result.StepsCompleted) != 5 {
		t.Errorf("expected 5 steps completed, got %d", len(result.StepsCompleted))
	}
}

// ---------------------------------------------------------------------------
// Test: missing hooks fall back to defaults
// ---------------------------------------------------------------------------
func TestRunUpgrade_MissingHooks_UsesDefaults(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	result, err := um.RunUpgrade(ctx, 2)
	if err != nil {
		t.Fatalf("expected no error with default hooks, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected upgrade to succeed with default hooks")
	}
}

// ---------------------------------------------------------------------------
// Test: context cancellation during a blocking hook
// ---------------------------------------------------------------------------
func TestRunUpgrade_ContextCancellation(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	um.RegisterHook(PreCheck, func(ctx context.Context, version int32) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := um.RunUpgrade(ctx, 2)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

// ---------------------------------------------------------------------------
// Test: concurrent upgrades are serialized by mutex
// ---------------------------------------------------------------------------
func TestRunUpgrade_ConcurrentPrevention(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	for _, phase := range allPhases {
		um.RegisterHook(phase, func(ctx context.Context, version int32) error {
			time.Sleep(20 * time.Millisecond)
			return nil
		})
	}

	ctx := context.Background()
	done := make(chan bool, 2)

	go func() {
		r, e := um.RunUpgrade(ctx, 2)
		done <- r != nil && e == nil
	}()

	time.Sleep(10 * time.Millisecond)

	go func() {
		r, e := um.RunUpgrade(ctx, 3)
		done <- r != nil && e == nil
	}()

	for i := 0; i < 2; i++ {
		select {
		case ok := <-done:
			if !ok {
				t.Errorf("upgrade %d did not complete successfully", i+1)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("upgrade timed out waiting for completion")
		}
	}
}

// ---------------------------------------------------------------------------
// Test: ValidateTarget rejects target exceeding max migration ID
// ---------------------------------------------------------------------------
func TestValidateTarget_VersionExceedsMaxMigration(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	migs := Migrations()
	maxID := int32(migs[len(migs)-1].ID)

	err := um.ValidateTarget(ctx, maxID+1)
	if err == nil {
		t.Fatal("expected error for target version exceeding max migration")
	}
}

// ---------------------------------------------------------------------------
// Test: ValidateTarget rejects target lower than current
// ---------------------------------------------------------------------------
func TestValidateTarget_TargetLowerThanCurrent(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(10)
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	err := um.ValidateTarget(ctx, 5)
	if err == nil {
		t.Fatal("expected error for target version lower than current")
	}
}

// ---------------------------------------------------------------------------
// Test: hook execution order matches allPhases slice
// ---------------------------------------------------------------------------
func TestHookExecutionOrder(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	var executed []string
	var mu sync.Mutex

	for _, phase := range allPhases {
		p := phase
		um.RegisterHook(p, func(ctx context.Context, version int32) error {
			mu.Lock()
			executed = append(executed, string(p))
			mu.Unlock()
			return nil
		})
	}

	ctx := context.Background()
	result, err := um.RunUpgrade(ctx, 2)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Success {
		t.Fatal("expected upgrade to succeed")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(executed) != 5 {
		t.Fatalf("expected 5 phases executed, got %d: %v", len(executed), executed)
	}

	expectedOrder := []string{"pre_check", "version_upgrade", "data_migration", "post_upgrade", "finalize"}
	for i, exp := range expectedOrder {
		if executed[i] != exp {
			t.Errorf("expected phase %d to be %s, got %s", i, exp, executed[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Test: version store errors propagate correctly
// ---------------------------------------------------------------------------
func TestRunUpgrade_VersionStoreError(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	store.getErr = errGetVersion
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	_, err := um.RunUpgrade(ctx, 2)
	if err == nil {
		t.Fatal("expected error when version store fails to return version")
	}
}

// ---------------------------------------------------------------------------
// Test: SetVersion failure during version_upgrade phase
// ---------------------------------------------------------------------------
func TestRunUpgrade_SetVersionError(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(1)
	store.setErr = errSetVersion
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	result, err := um.RunUpgrade(ctx, 2)
	if err == nil {
		t.Fatal("expected error when SetVersion fails")
	}
	if result == nil {
		t.Fatal("expected result to be non-nil even on failure")
	}
	if result.Success {
		t.Fatal("expected result to be not successful on error")
	}
}

// ---------------------------------------------------------------------------
// Test: GetSchemaVersion caching via atomic.Bool
// ---------------------------------------------------------------------------
func TestGetSchemaVersion_Caching(t *testing.T) {
	db := &mockDB{}
	store := newMockVersionStore(42)
	um := NewUpgradeManager(db, newTestLogger(), store)

	ctx := context.Background()
	v1, err := um.GetSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if v1 != 42 {
		t.Errorf("expected version 42, got %d", v1)
	}

	// Change version in store — should still return cached value
	_ = store.SetVersion(99)
	v2, err := um.GetSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if v2 != 42 {
		t.Errorf("expected cached version 42, got %d", v2)
	}
}

// ---------------------------------------------------------------------------
// Test: lock acquisition error is propagated
// ---------------------------------------------------------------------------
func TestRunUpgrade_LockError(t *testing.T) {
	db := &mockDB{
		connFn: func() (dbConnConn, error) {
			return nil, errLockTimeout
		},
	}
	store := newMockVersionStore(1)
	um := NewUpgradeManager(db, newTestLogger(), store)

	for _, phase := range allPhases {
		um.RegisterHook(phase, func(ctx context.Context, version int32) error {
			return nil
		})
	}

	ctx := context.Background()
	_, err := um.RunUpgrade(ctx, 2)
	if err == nil {
		t.Fatal("expected error when lock acquisition fails")
	}
}
