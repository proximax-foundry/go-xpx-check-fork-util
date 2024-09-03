package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
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
		OfflineAlertRepeatInterval        int `json:"offlineAlertRepeatInterval"`
		OfflineConsecutiveBlocksThreshold int `json:"offlineConsecutiveBlocksThreshold"`
		SyncAlertRepeatInterval           int `json:"syncAlertRepeatInterval"`
		StuckDurationThreshold            int `json:"stuckDurationThreshold"`
		OutOfSyncBlocksThreshold          int `json:"outOfSyncBlocksThreshold"`
		OutOfSyncCriticalNodesThreshold   int `json:"outOfSyncCriticalNodesThreshold"`
	}
)

var (
	ErrEmptyNodes  = errors.New("nodes cannot be empty")
	ErrEmptyApiUrl = errors.New("API url cannot be empty")
	ErrEmptyBotKey = errors.New("BotAPIKey cannot be empty")
	ErrEmptyChatId = errors.New("ChatID cannot be empty")
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
	return time.Duration(a.OfflineAlertRepeatInterval) * time.Minute
}

func (a *AlertConfig) getSyncAlertRepeatInterval() time.Duration {
	return time.Duration(a.SyncAlertRepeatInterval) * time.Minute
}

func (a *AlertConfig) getStuckDurationThreshold() time.Duration {
	return time.Duration(a.StuckDurationThreshold) * time.Minute
}
