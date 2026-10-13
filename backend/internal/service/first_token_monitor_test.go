package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFirstTokenMonitorConfigValidation(t *testing.T) {
	c := firstTokenMonitorConfig(nil)
	require.False(t, c.Enabled)
	require.Equal(t, 30, c.ThresholdSeconds)
	require.Equal(t, 3, c.AnomalyCount)
	require.Equal(t, 10, c.CooldownMinutes)
	require.NoError(t, ValidateFirstTokenMonitorCredentials(map[string]any{"first_token_monitor_enabled": true, "first_token_monitor_threshold_seconds": float64(40)}))
	for _, v := range []any{0, -1, 1.5, "30", 3601} {
		require.Error(t, ValidateFirstTokenMonitorCredentials(map[string]any{"first_token_monitor_threshold_seconds": v}))
	}
	require.Error(t, ValidateFirstTokenMonitorCredentials(map[string]any{"first_token_monitor_enabled": "true"}))
}
