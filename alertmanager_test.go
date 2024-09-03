package main

import (
	"testing"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crypto "github.com/proximax-storage/go-xpx-crypto"
)

func TestShouldSendOfflineAlert(t *testing.T) {

	t.Run("First Occurrence", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)

		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)

		nodeInfo := health.NodeInfo{
			IdentityKey:  getPublicKey(config.Nodes[0].IdentityKey),
			Endpoint:     config.Nodes[0].Endpoint,
			FriendlyName: config.Nodes[0].FriendlyName,
		}

		failedConnectionsNodes := map[string]*health.NodeInfo{
			nodeInfo.IdentityKey.String(): &nodeInfo,
		}

		// Check that alert should not be sent before exceeding the threshold
		shouldAlert := fc.alertManager.shouldSendOfflineAlert(failedConnectionsNodes)
		assert.Equal(t, false, shouldAlert)
	})

	t.Run("Exceed Threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)

		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)

		nodeInfo := health.NodeInfo{
			IdentityKey:  getPublicKey(config.Nodes[0].IdentityKey),
			Endpoint:     config.Nodes[0].Endpoint,
			FriendlyName: config.Nodes[0].FriendlyName,
		}

		failedConnectionsNodes := map[string]*health.NodeInfo{
			nodeInfo.IdentityKey.String(): &nodeInfo,
		}

		// Check that alert should be sent after exceeding the threshold
		blocksCount := fc.cfg.AlertConfig.OfflineConsecutiveBlocksThreshold
		shouldAlert := false
		for i := 0; i < blocksCount+1; i++ {
			shouldAlert = fc.alertManager.shouldSendOfflineAlert(failedConnectionsNodes)
		}
		assert.Equal(t, true, shouldAlert)

		// Check that alert should not be sent if repeat interval has not passed
		fc.alertManager.updateNodeStatusLastOfflineAlertTime(OfflineAlert{
			NotConnected: failedConnectionsNodes,
		})
		shouldAlert = fc.alertManager.shouldSendOfflineAlert(failedConnectionsNodes)
		assert.Equal(t, false, shouldAlert)

		// Check that alert should be sent again after the repeat interval has passed
		if status, exists := fc.alertManager.offlineNodeStats[nodeInfo.IdentityKey.String()]; exists {
			status.lastOfflineAlertTime = time.Now().Add(-fc.alertManager.config.getOfflineAlertRepeatInterval() - time.Hour)
			fc.alertManager.updateNodeStatus(nodeInfo.IdentityKey.String(), status)
		}
		shouldAlert = fc.alertManager.shouldSendOfflineAlert(failedConnectionsNodes)
		assert.Equal(t, true, shouldAlert)
	})
}

func TestShouldSendSyncAlert(t *testing.T) {

	t.Run("Exceed stuck duration threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)
	
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)
	
		checkpoint := uint64(1000)

		notReached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[0]: 950,
			*fc.alertManager.nodeInfos[1]: 951,
			*fc.alertManager.nodeInfos[2]: 952,
			*fc.alertManager.nodeInfos[3]: 953,
			*fc.alertManager.nodeInfos[4]: 954,
			*fc.alertManager.nodeInfos[5]: 955,
		}
	
		reached := map[health.NodeInfo]uint64{}

		shouldAlert := fc.alertManager.shouldSendSyncAlert(checkpoint, notReached, reached)
		assert.Equal(t, false, shouldAlert)
	})

	t.Run("Not exceed stuck duration threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)
	
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)
	
		checkpoint := uint64(1000)

		notReached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[0]: 950,
			*fc.alertManager.nodeInfos[1]: 951,
			*fc.alertManager.nodeInfos[2]: 952,
			*fc.alertManager.nodeInfos[3]: 953,
			*fc.alertManager.nodeInfos[4]: 954,
			*fc.alertManager.nodeInfos[5]: 955,
		}
	
		reached := map[health.NodeInfo]uint64{}
	
		fc.alertManager.lastStuckHeight = checkpoint
		fc.alertManager.lastStuckTime = time.Now().Add(-fc.alertManager.config.getStuckDurationThreshold()*2)
		
		shouldAlert := fc.alertManager.shouldSendSyncAlert(checkpoint, notReached, reached)
		assert.Equal(t, true, shouldAlert)
	})

	t.Run("Exceed critical nodes threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)
	
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)
	
		checkpoint := uint64(1000)

		notReached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[0]: 950,
			*fc.alertManager.nodeInfos[1]: 951,
			*fc.alertManager.nodeInfos[2]: 952,
			*fc.alertManager.nodeInfos[3]: 953,
			*fc.alertManager.nodeInfos[4]: 954,
		}
	
		reached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[5]: 1000,
		}
	
		shouldAlert := fc.alertManager.shouldSendSyncAlert(checkpoint, notReached, reached)
		assert.Equal(t, true, shouldAlert)
	})

	t.Run("Not exceed critical nodes threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)
	
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)
	
		checkpoint := uint64(1000)

		notReached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[0]: 950,
			*fc.alertManager.nodeInfos[1]: 951,
			*fc.alertManager.nodeInfos[2]: 952,
			*fc.alertManager.nodeInfos[3]: 953,
		}
	
		reached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[4]: 1000,
			*fc.alertManager.nodeInfos[5]: 1000,
		}
	
		shouldAlert := fc.alertManager.shouldSendSyncAlert(checkpoint, notReached, reached)
		assert.Equal(t, false, shouldAlert)
	})

	t.Run("Not exceed blocks threshold", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)
	
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.NoError(t, err)
	
		checkpoint := uint64(1000)

		notReached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[0]: 999,
			*fc.alertManager.nodeInfos[1]: 999,
			*fc.alertManager.nodeInfos[2]: 999,
			*fc.alertManager.nodeInfos[3]: 999,
			*fc.alertManager.nodeInfos[4]: 999,
		}
	
		reached := map[health.NodeInfo]uint64{
			*fc.alertManager.nodeInfos[5]: 1000,
		}
	
		shouldAlert := fc.alertManager.shouldSendSyncAlert(checkpoint, notReached, reached)
		assert.Equal(t, false, shouldAlert)
	})
}

func getPublicKey(key string) *crypto.PublicKey {
	publicKey, _ := crypto.NewPublicKeyfromHex(key)
	return publicKey
}
