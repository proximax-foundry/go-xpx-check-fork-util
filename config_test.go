package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	config, err := LoadConfig("sample.config.json")
	require.NoError(t, err)

	assert.Equal(t, 6, len(config.Nodes))
	assert.Equal(t, 2, len(config.ApiUrls))
	assert.Equal(t, true, config.Discover)
	assert.Equal(t, uint64(1), config.HeightCheckInterval)
	assert.Equal(t, true, config.Notify)
	assert.Equal(t, 5, config.AlertConfig.OutOfSyncBlocksThreshold)
	assert.Equal(t, 5, config.AlertConfig.OutOfSyncCriticalNodesThreshold)
	assert.Equal(t, time.Duration(2*time.Hour), config.AlertConfig.getOfflineAlertRepeatInterval())
	assert.Equal(t, time.Duration(5*time.Minute), config.AlertConfig.getOfflineDurationThreshold())
	assert.Equal(t, time.Duration(2*time.Hour), config.AlertConfig.getSyncAlertRepeatInterval())
	assert.Equal(t, time.Duration(10*time.Minute), config.AlertConfig.getStuckDurationThreshold())
}