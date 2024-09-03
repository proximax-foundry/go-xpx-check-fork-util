package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health/packets"
	crypto "github.com/proximax-storage/go-xpx-crypto"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ForkChecker struct {
	cfg            Config
	alertManager   *AlertManager
	catapultClient *sdk.Client
	nodePool       *health.NodeHealthCheckerPool
	checkpoint     uint64
}

func NewForkChecker(config Config) (*ForkChecker, error) {
	fc := &ForkChecker{cfg: config}

	if err := fc.initCatapultClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize catapult client: %v", err)
	}

	if err := fc.initAlertManager(); err != nil {
		return nil, fmt.Errorf("failed to initialize alert manager: %v", err)
	}

	if err := fc.initPool(); err != nil {
		return nil, fmt.Errorf("failed to initialize node health checker pool: %v", err)
	}

	if err := fc.initCheckpoint(); err != nil {
		return nil, fmt.Errorf("failed to initialize checkpoint: %v", err)
	}

	return fc, nil
}

func (fc *ForkChecker) initCheckpoint() error {
	if fc.cfg.Checkpoint != 0 {
		fc.checkpoint = fc.cfg.Checkpoint
	} else {
		height, err := fc.catapultClient.Blockchain.GetBlockchainHeight(context.Background())
		if err != nil {
			return fmt.Errorf("error getting blockchain height: %v", err)
		}
		fc.checkpoint = uint64(height)
	}

	log.Println("Initialized checkpoint:", fc.checkpoint)

	return nil
}

func (fc *ForkChecker) initPool() error {
	clientKeyPair, err := crypto.NewRandomKeyPair()
	if err != nil {
		return fmt.Errorf("error generating random keypair: %s", err)
	}

	fc.nodePool = health.NewNodeHealthCheckerPool(
		clientKeyPair,
		packets.NoneConnectionSecurity,
		math.MaxInt,
	)

	return nil
}

func (fc *ForkChecker) initAlertManager() error {
	nodeInfos, err := parseNodes(fc.cfg.Nodes)
	if err != nil {
		return fmt.Errorf("error parsing node info: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(fc.cfg.BotAPIKey)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	bot.Debug = false

	fc.alertManager = &AlertManager{
		config:           fc.cfg.AlertConfig,
		lastAlertTimes:   make(map[AlertType]time.Time),
		offlineNodeStats: make(map[string]NodeStatus),
		nodeInfos:        nodeInfos,
		notifier: &Notifier{
			bot:     bot,
			chatID:  fc.cfg.ChatID,
			enabled: fc.cfg.Notify,
		},
	}

	return nil
}

func (fc *ForkChecker) initCatapultClient() error {
	var conf *sdk.Config
	var err error

	for _, url := range fc.cfg.ApiUrls {
		conf, err = sdk.NewConfig(context.Background(), []string{url})
		if err == nil {
			log.Printf("Initialized client on URL: %s", url)
			fc.catapultClient = sdk.NewClient(nil, conf)
			return nil
		}
	}

	return fmt.Errorf("all provided URLs failed: %v", err)
}

func (fc *ForkChecker) Start() error {
	for {
		failedConnectionsNodes, err := fc.nodePool.ConnectToNodes(fc.alertManager.nodeInfos, fc.cfg.Discover)
		if err != nil {
			log.Printf("error connecting to nodes: %s", err)
			continue
		}
		
		// Trigger alert if offline nodes include bootstrap nodes or API nodes.
		fc.alertManager.handleOfflineAlert(failedConnectionsNodes)

		notReached, reached, err := fc.nodePool.WaitHeight(fc.checkpoint)
		if err != nil {
			log.Printf("error waiting for connected nodes to reach %d height: %s", fc.checkpoint, err)
			continue
		}
		
		// Trigger alert if the following conditions are met:
		//   - No nodes have synced to the checkpoint height for X minutes (stuck alert) 
		//   - Among the out-of-sync nodes, there are Y or more bootstrap or API nodes that are Z blocks or more behind the chain's highest height.
		// X, Y, Z values are configurable in the config.json file:
		//   X - stuckDurationThreshold
		//   Y - outOfSyncCriticalNodesThreshold
		//   Z - outOfSyncBlocksThreshold
		fc.alertManager.handleSyncAlert(fc.checkpoint, notReached, reached)

		// Skip incrementing checkpoint if the chain is stuck.
		if len(reached) == 0 {
			log.Printf("Chain is stuck! No nodes  reached height: %d", fc.checkpoint)
			continue
		}

		log.Printf("Checking block hash at %d height", fc.checkpoint)
		hashes, err := fc.nodePool.CompareHashes(fc.checkpoint)

		// Trigger alert if the hashes of the last confirmed block are not the same.
		if err != nil {
			switch err {
			case health.ErrHashesAreNotTheSame:
				log.Printf("hashes are not the same at %d height: %v", fc.checkpoint, hashes)
				fc.alertManager.handleHashAlert(fc.checkpoint, hashes)
			case health.ErrNoConnectedPeers:
				log.Printf("error comparing hashes for connected nodes at %d height: %s", fc.checkpoint, err)
				continue
			default:
				log.Printf("unexpected error when comparing hashes at %d height: %s", fc.checkpoint, err)
				continue
			}
		}

		// Update checkpoint
		fc.checkpoint += fc.cfg.HeightCheckInterval
	}
}
