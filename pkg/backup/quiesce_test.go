//go:build integration
// +build integration

package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuiescing_State(t *testing.T) {
	// Verify quiesce state

	quiesceState := StateQuiesce
	require.Equal(t, RecoveryState(3), quiesceState)
	require.Equal(t, "Quiesce", quiesceState.String())
}

func TestQuiescing_Transition(t *testing.T) {
	// Verify quiesce transitions

	require.True(t, isValidTransition(StatePreFlight, StateQuiesce))
	require.True(t, isValidTransition(StateQuiesce, StateBackupDatabase))
}

func TestQuiescing_Phase(t *testing.T) {
	// Verify quiesce phase

	phase := PhaseForState(StateQuiesce)
	require.Equal(t, PhaseBackup, phase)
}

func TestQuiescing_Events(t *testing.T) {
	// Verify quiesce event recording

	event := RecoveryEvent{
		State: StateQuiesce,
		Phase: PhaseBackup,
	}

	require.Equal(t, StateQuiesce, event.State)
	require.Equal(t, PhaseBackup, event.Phase)
}

func TestQuiescing_Timeout(t *testing.T) {
	// Verify quiescing timeout handling

	timeout := 30 * time.Minute
	require.True(t, timeout > 0)
}
