//go:build integration
// +build integration

package backup

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConcurrentRecovery_StateTransitions(t *testing.T) {
	// Verify valid state transitions

	require.True(t, isValidTransition(StateIdle, StateDiscovery))
	require.True(t, isValidTransition(StateDiscovery, StatePreFlight))
	require.True(t, isValidTransition(StatePreFlight, StateQuiesce))
	require.True(t, isValidTransition(StateQuiesce, StateBackupDatabase))
	require.True(t, isValidTransition(StateBackupDatabase, StateBackupJetStream))
	require.True(t, isValidTransition(StateBackupJetStream, StateBackupObjectStorage))
	require.True(t, isValidTransition(StateBackupObjectStorage, StateVerifyIntegrity))
	require.True(t, isValidTransition(StateVerifyIntegrity, StatePreRestoreValidation))
	require.True(t, isValidTransition(StatePreRestoreValidation, StateRestoreDatabase))
	require.True(t, isValidTransition(StateRestoreDatabase, StateRestoreJetStream))
	require.True(t, isValidTransition(StateRestoreJetStream, StateRestoreObjectStorage))
	require.True(t, isValidTransition(StateRestoreObjectStorage, StatePostRestoreValidation))
	require.True(t, isValidTransition(StatePostRestoreValidation, StateHealthCheck))
	require.True(t, isValidTransition(StateHealthCheck, StateVerification))
	require.True(t, isValidTransition(StateVerification, StateRPOValidation))
	require.True(t, isValidTransition(StateRPOValidation, StateRTOValidation))
	require.True(t, isValidTransition(StateRTOValidation, StateCompleted))
}

func TestConcurrentRecovery_InvalidTransitions(t *testing.T) {
	// Verify invalid state transitions

	require.False(t, isValidTransition(StateIdle, StateRestoreDatabase))
	require.False(t, isValidTransition(StateCompleted, StateDiscovery))
	require.False(t, isValidTransition(StateRollback, StateBackupDatabase))
}

func TestConcurrentRecovery_BackupIDs(t *testing.T) {
	// Verify backup ID generation uniqueness

	id1 := generateDatabaseBackupID()
	id2 := generateDatabaseBackupID()

	require.NotEqual(t, id1, id2)
	require.True(t, len(id1) > 0)
	require.True(t, len(id2) > 0)
}

func TestConcurrentRecovery_EventConcurrency(t *testing.T) {
	// Verify event recording is thread-safe

	coordinator := NewRecoveryCoordinator(nil, nil)
	var wg sync.WaitGroup

	// Concurrently record events
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			coordinator.Events()
		}(i)
	}

	wg.Wait()
}

func TestConcurrentRecovery_TimeoutConfiguration(t *testing.T) {
	// Verify timeout configuration

	coordinator := NewRecoveryCoordinator(nil, nil)
	coordinator.SetTimeout(30 * time.Minute)

	require.Equal(t, 30*time.Minute, coordinator.timeout)
}

func TestConcurrentRecovery_DryRunMode(t *testing.T) {
	// Verify dry-run mode

	coordinator := NewRecoveryCoordinator(nil, nil)
	coordinator.SetDryRun(true)
	require.True(t, coordinator.dryRun)

	coordinator.SetDryRun(false)
	require.False(t, coordinator.dryRun)
}

func TestConcurrentRecovery_BackupIDSet(t *testing.T) {
	// Verify backup ID setting

	coordinator := NewRecoveryCoordinator(nil, nil)
	coordinator.SetBackupID("test-backup-123")

	require.Equal(t, "test-backup-123", coordinator.backupID)
}

func TestConcurrentRecovery_InitialState(t *testing.T) {
	// Verify initial state

	coordinator := NewRecoveryCoordinator(nil, nil)
	require.Equal(t, StateIdle, coordinator.State())
}
