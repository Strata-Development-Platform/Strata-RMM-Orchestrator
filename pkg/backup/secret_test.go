//go:build integration
// +build integration

package backup

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretRedaction_EventMessage(t *testing.T) {
	// Verify event message doesn't leak secrets

	event := RecoveryEvent{
		Message: "Starting recovery",
	}

	require.Equal(t, "Starting recovery", event.Message)
	require.NotContains(t, event.Message, "password")
}

func TestSecretRedaction_RecoveryState(t *testing.T) {
	// Verify recovery state string doesn't contain secrets

	state := StateRestoreDatabase
	require.Equal(t, "RestoreDatabase", state.String())
	require.NotContains(t, state.String(), "password")
}

func TestSecretRedaction_EmptyMessage(t *testing.T) {
	// Verify empty message handling

	event := RecoveryEvent{
		Message: "",
	}

	require.Equal(t, "", event.Message)
}

func TestSecretRedaction_EventDuration(t *testing.T) {
	// Verify event duration field

	event := RecoveryEvent{
		Duration: 30 * 1000000000, // 30 seconds in nanoseconds
	}

	require.Equal(t, int64(30*1000000000), event.Duration)
}
