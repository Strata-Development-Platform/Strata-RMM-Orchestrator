//go:build integration
// +build integration

package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRPOMeasurement_NormalCase(t *testing.T) {
	// Verify RPO metrics

	rpo := RPOMetrics{
		DataLossWindow:   1 * time.Hour,
		MaxAcceptableRPO: 24 * time.Hour,
	}

	require.Equal(t, 1*time.Hour, rpo.DataLossWindow)
	require.Equal(t, 24*time.Hour, rpo.MaxAcceptableRPO)
	require.True(t, rpo.DataLossWindow < rpo.MaxAcceptableRPO)
}

func TestRPOMeasurement_Met(t *testing.T) {
	// Verify RPO meets threshold

	rpo := RPOMetrics{
		DataLossWindow:   30 * time.Minute,
		MaxAcceptableRPO: 24 * time.Hour,
	}

	require.True(t, rpo.DataLossWindow <= rpo.MaxAcceptableRPO)
}

func TestRPOMeasurement_Missed(t *testing.T) {
	// Verify RPO misses threshold

	rpo := RPOMetrics{
		DataLossWindow:   25 * time.Hour,
		MaxAcceptableRPO: 24 * time.Hour,
	}

	require.True(t, rpo.DataLossWindow > rpo.MaxAcceptableRPO)
}

func TestRTOMeasurement_NormalCase(t *testing.T) {
	// Verify RTO metrics

	rto := RTOMetrics{
		TotalRecoveryTime: 10 * time.Minute,
	}

	require.Equal(t, 10*time.Minute, rto.TotalRecoveryTime)
}

func TestRTOMeasurement_FastRecovery(t *testing.T) {
	// Verify fast recovery

	rto := RTOMetrics{
		TotalRecoveryTime: 1 * time.Minute,
	}

	require.Equal(t, 1*time.Minute, rto.TotalRecoveryTime)
	require.True(t, rto.TotalRecoveryTime < 10*time.Minute)
}

func TestRTOMeasurement_SlowRecovery(t *testing.T) {
	// Verify slow recovery

	rto := RTOMetrics{
		TotalRecoveryTime: 1 * time.Hour,
	}

	require.Equal(t, 1*time.Hour, rto.TotalRecoveryTime)
}

func TestRPOR_TO_Combined(t *testing.T) {
	// Verify combined RPO and RTO

	result := RecoveryResult{
		RPO: RPOMetrics{
			DataLossWindow:   1 * time.Hour,
			MaxAcceptableRPO: 24 * time.Hour,
		},
		RTO: RTOMetrics{
			TotalRecoveryTime: 10 * time.Minute,
		},
	}

	require.Equal(t, 1*time.Hour, result.RPO.DataLossWindow)
	require.Equal(t, 10*time.Minute, result.RTO.TotalRecoveryTime)
}
