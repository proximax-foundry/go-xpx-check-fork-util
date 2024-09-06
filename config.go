package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
)

type (
	Config struct {
		Nodes               []Node      `json:"nodes"`
		ApiUrls             []string    `json:"apiUrls"`
		Discover            bool        `json:"discover"`
		Checkpoint          uint64      `json:"checkpoint"`
		HeightCheckInterval uint64      `json:"heightCheckInterval"`
		BotAPIKey           string      `json:"botApiKey"`
		ChatID              int64       `json:"chatID"`
		Notify              bool        `json:"notify"`
		AlertConfig         AlertConfig `json:"alertConfig"`
	}

	Node struct {
		Endpoint     string `json:"endpoint"`
		IdentityKey  string `json:"IdentityKey"`
		FriendlyName string `json:"friendlyName"`
	}

	AlertConfig struct {
		OfflineAlertRepeatInterval      string `json:"offlineAlertRepeatInterval"`
		OfflineDurationThreshold        string `json:"offlineDurationThreshold"`
		SyncAlertRepeatInterval         string `json:"syncAlertRepeatInterval"`
		StuckDurationThreshold          string `json:"stuckDurationThreshold"`
		OutOfSyncBlocksThreshold        int    `json:"outOfSyncBlocksThreshold"`
		OutOfSyncCriticalNodesThreshold int    `json:"outOfSyncCriticalNodesThreshold"`
	}
)

var (
	ErrEmptyNodes  = errors.New("nodes cannot be empty")
	ErrEmptyApiUrl = errors.New("API url cannot be empty")
	ErrEmptyBotKey = errors.New("BotAPIKey cannot be empty")
	ErrEmptyChatId = errors.New("ChatID cannot be empty")
)

const (
	DefaultOfflineAlertRepeatInterval = time.Hour * 12
	DefaultOfflineDurationThreshold   = time.Minute * 5
	DefaultSyncAlertRepeatInterval    = time.Hour * 6
	DefaultStuckDurationThreshold     = time.Minute * 10
)

func LoadConfig(fileName string) (*Config, error) {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed reading config file '%s': %w", fileName, err)
	}

	config := &Config{}
	if err := json.Unmarshal(content, config); err != nil {
		return nil, fmt.Errorf("failed unmarshalling config file '%s': %w", fileName, err)
	}

	err = config.Validate()
	if err != nil {
		return nil, fmt.Errorf("validation error in config file '%s': %w", fileName, err)
	}

	return config, nil
}

func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return ErrEmptyNodes
	}

	if len(c.ApiUrls) == 0 {
		return ErrEmptyApiUrl
	}

	if c.BotAPIKey == "" {
		return ErrEmptyBotKey
	}

	if c.ChatID == 0 {
		return ErrEmptyChatId
	}

	return nil
}

func (a *AlertConfig) getOfflineAlertRepeatInterval() time.Duration {
	duration, err := time.ParseDuration(a.OfflineAlertRepeatInterval)
	if err != nil {
		fmt.Println("Error parsing offline alert repeat interval:", err)
		return DefaultOfflineAlertRepeatInterval
	}
	return duration
}

func (a *AlertConfig) getSyncAlertRepeatInterval() time.Duration {
	duration, err := time.ParseDuration(a.SyncAlertRepeatInterval)
	if err != nil {
		fmt.Println("Error parsing sync alert repeat interval:", err)
		return DefaultSyncAlertRepeatInterval
	}
	return duration
}

func (a *AlertConfig) getStuckDurationThreshold() time.Duration {
	duration, err := time.ParseDuration(a.StuckDurationThreshold)
	if err != nil {
		fmt.Println("Error parsing stuck duration threshold:", err)
		return DefaultStuckDurationThreshold
	}
	return duration
}

func (a *AlertConfig) getOfflineDurationThreshold() time.Duration {
	duration, err := time.ParseDuration(a.OfflineDurationThreshold)
	if err != nil {
		fmt.Println("Error parsing offline duration threshold:", err)
		return DefaultOfflineDurationThreshold
	}
	return duration
}

func (a *AlertConfig) getOfflineBlocksThreshold() int {
	return int(a.getOfflineDurationThreshold() / health.DefaultAvgSecondsPerBlock)
}
