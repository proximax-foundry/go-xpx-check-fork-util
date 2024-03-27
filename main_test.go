package main

import (
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
	"github.com/stretchr/testify/assert"
)

var (
	ValidConfigJson = `{
		"nodes": [
			{
				"endpoint": "arcturus.xpxsirius.io:7900",
				"IdentityKey": "8B1FBE2F65D4AD2EA7A1421109B76CCD13ED2D0F34FCA1F10C93BFA4CC0A5D53"
			}           
		],
		"apiUrls": [
			"https://arcturus.xpxsirius.io"
		],
		"discover": true,
		"checkpoint": 0,
		"heightCheckInterval": 5,
		"alarmInterval": 1,
		"botApiKey": "7108251290:AAHYAp0fi7leBHAD9Xtna8ay2Zm48Y5zZh0",
		"chatID": 111122223333,
		"notify": true
	}`
)

func setupForkCheckerConfig() *ForkChecker {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	return &ForkChecker{cfg: *config}
}

/* ------------------------------- config tests ------------------------------ */

func TestValidateConfig_MissingApiUrls(t *testing.T) {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	config.ApiUrls = nil
	err := config.Validate()
	assert.EqualError(t, err, ErrEmptyApiUrl.Error())
}

func TestValidateConfig_MissingNodes(t *testing.T) {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	config.Nodes = nil
	err := config.Validate()
	assert.EqualError(t, err, ErrEmptyNodes.Error())
}

func TestValidateConfig_MissingBotKey(t *testing.T) {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	config.BotAPIKey = ""
	err := config.Validate()
	assert.EqualError(t, err, ErrEmptyBotKey.Error())
}

func TestValidateConfig_MissingChatId(t *testing.T) {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	config.ChatID = 0
	err := config.Validate()
	assert.EqualError(t, err, ErrEmptyChatId.Error())
}

/* ------------------------------ notifier tests ----------------------------- */

func TestDisableNotifier(t *testing.T) {
	f := setupForkCheckerConfig()

	err := f.initNotifier()
	assert.NoError(t, err, err)

	f.notifier.enabled = false
	canAlert := f.notifier.canAlert(time.Now())
	assert.Equal(t, false, canAlert)
}

func TestEnableNotifier(t *testing.T) {
	f := setupForkCheckerConfig()

	err := f.initNotifier()
	assert.NoError(t, err, err)

	f.notifier.enabled = true
	canAlert := f.notifier.canAlert(time.Now())
	assert.Equal(t, true, canAlert)
}

func TestNotifierIsRespectingAlarmInterval(t *testing.T) {
	f := setupForkCheckerConfig()
	err := f.initNotifier()
	assert.NoError(t, err, err)

	f.notifier.alarmInterval = time.Second * 5
	testDuration := time.Second * 60
	startTime := time.Now()

	expectedAlertCount := int(testDuration / f.notifier.alarmInterval)
	actualAlertCount := 0
	for time.Since(startTime) < testDuration {
		if f.notifier.canAlert(f.notifier.lastSyncAlertTime) {
			log.Println("can alert")
			actualAlertCount++
			f.notifier.lastSyncAlertTime = time.Now()
		} else {
			log.Println("blocked")
		}

		time.Sleep(time.Second / 2)
	}

	assert.Equal(t, expectedAlertCount, actualAlertCount)
}

func TestCreateHashAlertStringFromTemplate(t *testing.T) {
	height := uint64(789)
	hash1, _ := sdk.StringToHash("DA6B8ECFEBDDAA49CA26DEB8AC2F6346DBC9C8DD96B4584A01410190DAB4A45A")
	hash2, _ := sdk.StringToHash("4F7A80E9D6C2A4F5B46B90A1D16E95D4C1B8A3E8D5D1479D7C802C475D70A2EE")
	hashes := map[string]sdk.Hash{
		"111": *hash1,
		"222": *hash2,
		"333": *hash2,
		"444": *hash1,
		"555": *hash1,
	}
	htmlContent := HashAlertMsg(height, hashes)
	assert.NotNil(t, htmlContent)
	fmt.Println(htmlContent)
}

func TestCreateHeightAlertStringFromTemplate(t *testing.T) {
	height := uint64(25000)
	notReached := map[string]uint64{
		"DA6B8ECFEBDDAA49CA26DEB8AC2F6346DBC9C8DD96B4584A01410190DAB4A45A": 10000,
		"4F7A80E9D6C2A4F5B46B90A1D16E95D4C1B8A3E8D5D1479D7C802C475D70A2EE": 12000,
	}

	reached := map[string]uint64{
		"DA6B8ECFEBDDAA49CA26DEB8AC2F6346DBC9C8DD96B4584A01410190DAB4A45A": 25000,
		"4F7A80E9D6C2A4F5B46B90A1D16E95D4C1B8A3E8D5D1479D7C802C475D70A2EE": 25000,
	}

	notConnected := []*health.NodeInfo{
		{
			IdentityKey: nil,
			Endpoint: "127.0.0.1:7900",
		},
		{
			IdentityKey: nil,
			Endpoint: "127.0.0.1:7904",
		},
	}

	htmlContent := HeightAlertMsg(height, notReached, reached, notConnected)
	assert.NotNil(t, htmlContent)
	fmt.Println(htmlContent)
}

/* --------------------------- fork checker tests --------------------------- */
func TestCreateNewForkChecker(t *testing.T) {
	config := &Config{}
	json.Unmarshal([]byte(ValidConfigJson), config)

	f, err := NewForkChecker(*config)
	assert.NoError(t, err, err)
	assert.NotNil(t, f)
	assert.NotNil(t, f.catapultClient)
	assert.NotNil(t, f.checkpoint)
	assert.NotNil(t, f.nodePool)
	assert.NotNil(t, f.notifier)
	assert.NotNil(t, f.cfg)
	assert.Equal(t, *config, f.cfg)
}

func TestInitCheckpoint(t *testing.T) {
	f := setupForkCheckerConfig()

	err := f.initClient()
	assert.NoError(t, err, err)
	assert.NotNil(t, f.catapultClient)

	err = f.initCheckpoint()
	assert.NoError(t, err, err)
	assert.NotNil(t, f.checkpoint)
	log.Println("Checkpoint", f.checkpoint)
}

func TestInitPool_DisableDiscover(t *testing.T) {
	f := setupForkCheckerConfig()
	f.cfg.Discover = false

	err := f.initPool()
	assert.NoError(t, err, err)
	assert.NotNil(t, f.nodePool)
}

func TestInitPool_EnableDiscover(t *testing.T) {
	f := setupForkCheckerConfig()
	f.cfg.Discover = true

	err := f.initPool()
	assert.NoError(t, err, err)
	assert.NotNil(t, f.nodePool)
}