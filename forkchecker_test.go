package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCheckpoint(t *testing.T){
	config, err := LoadConfig("sample.config.json")
	require.NoError(t, err)
	
	config.ApiUrls = append(config.ApiUrls, "https://betelgeuse.xpxsirius.io/")

	t.Run("Checkpoint equals 0", func(t *testing.T) {
		config.Checkpoint = uint64(0)

		fc := &ForkChecker{cfg: *config}

		err := fc.initCatapultClient()
		require.NoError(t, err)
		
		err = fc.initCheckpoint()
		require.NoError(t, err)
		assert.NotEqual(t, 0, fc.checkpoint)
	})

	t.Run("Checkpoint not equal 0", func(t *testing.T) {
		config.Checkpoint = uint64(9876543)
		fc := &ForkChecker{cfg: *config}

		err :=  fc.initCatapultClient()
		require.NoError(t, err)

		err = fc.initCheckpoint()
		require.NoError(t, err)
		assert.Equal(t, uint64(9876543), fc.checkpoint)
	})
}

func TestInitCatapultClient(t *testing.T){
	config, err := LoadConfig("sample.config.json")
	require.NoError(t, err)

	t.Run("Invalid URL", func(t *testing.T) {
		fc := &ForkChecker{cfg: *config}

		err := fc.initCatapultClient()
		require.Error(t, err)
	})

	t.Run("Valid URL", func(t *testing.T) {
		config.ApiUrls = append(config.ApiUrls, "https://betelgeuse.xpxsirius.io/")
		fc := &ForkChecker{cfg: *config}

		err :=  fc.initCatapultClient()
		require.NoError(t, err)
	})
}


func TestInitAlertManager(t *testing.T){
	t.Run("Invalid nodes", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)

		invalidNode := Node{
			Endpoint: "127.0.0.3",
			IdentityKey: "ABCDEFG123456",
			FriendlyName: "NodeC",
		}
		config.Nodes = append(config.Nodes, invalidNode)
		
		fc := &ForkChecker{cfg: *config}
		err = fc.initAlertManager()
		require.Error(t, err)
	})

	t.Run("Invalid telegram bot", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)

		config.BotAPIKey = "123456789:abcdefghijklmn"
		
		fc := &ForkChecker{cfg: *config}
		err =  fc.initAlertManager()
		require.Error(t, err)
	})

	t.Run("Valid config", func(t *testing.T) {
		config, err := LoadConfig("sample.config.json")
		require.NoError(t, err)

		fc := &ForkChecker{cfg: *config}
		err =  fc.initAlertManager()
		require.NoError(t, err)
	})

}

