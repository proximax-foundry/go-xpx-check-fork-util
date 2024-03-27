package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health/packets"
	crypto "github.com/proximax-storage/go-xpx-crypto"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type (
	Nodes []struct {
		Endpoint    string `json:"endpoint"`
		IdentityKey string `json:"IdentityKey"`
	}

	Config struct {
		Nodes               Nodes    `json:"nodes"`
		ApiUrls             []string `json:"apiUrls"`
		Discover            bool     `json:"discover"`
		Checkpoint          uint64   `json:"checkpoint"`
		HeightCheckInterval uint64   `json:"heightCheckInterval"`
		AlarmInterval       uint64   `json:"alarmInterval"`
		BotAPIKey           string   `json:"botApiKey"`
		ChatID              int64    `json:"chatID"`
		Notify              bool     `json:"notify"`
	}

	Notifier struct {
		bot               *tgbotapi.BotAPI
		chatID            int64
		alarmInterval     time.Duration
		lastHashAlertTime time.Time
		lastSyncAlertTime time.Time
		enabled           bool
	}

	ForkChecker struct {
		cfg                    Config
		notifier               *Notifier
		catapultClient         *sdk.Client
		nodeInfos              []*health.NodeInfo
		nodePool               *health.NodeHealthCheckerPool
		failedConnectionsNodes []*health.NodeInfo
		executionTime          time.Time
		checkpoint             uint64
	}
)

var (
	ErrEmptyNodes  = errors.New("Nodes cannot be empty")
	ErrEmptyApiUrl = errors.New("API url cannot be empty")
	ErrEmptyBotKey = errors.New("BotAPIKey cannot be empty")
	ErrEmptyChatId = errors.New("ChatID cannot be empty")
)

const (
	PeersDiscoveryInterval = time.Hour
	AlarmInterval          = time.Hour
)

func main() {
	fileName := flag.String("file", "config.json", "Name of file to load config from")
	flag.Parse()

	config, err := LoadConfig(*fileName)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	f, err := NewForkChecker(*config)
	if err != nil {
		log.Fatalf("Failed to setup fork checker: %v", err)
	}

	err = f.Start()
	if err != nil {
		log.Fatalf("Error running fork checker: %v", err)
	}
}

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

func ParseNodes(nodes Nodes) ([]*health.NodeInfo, error) {
	nodeInfos := make([]*health.NodeInfo, 0, len(nodes))

	for _, node := range nodes {
		ni, err := health.NewNodeInfo(node.IdentityKey, node.Endpoint)
		if err != nil {
			return nil, err
		}

		nodeInfos = append(nodeInfos, ni)
	}

	return nodeInfos, nil
}

func (n *Notifier) IsEnabled() bool {
	return n.enabled
}

func (n *Notifier) canAlert(lastAlertTime time.Time) bool {
	return n.IsEnabled() && (lastAlertTime.IsZero() || time.Since(lastAlertTime) >= n.alarmInterval)
}

func (n *Notifier) SendAlert(text string, alarmTime *time.Time) error {
	msgConfig := tgbotapi.NewMessage(n.chatID, text)
	msgConfig.ParseMode = "HTML"

	_, err := n.bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("failed to send message to telegram: %v", err)
	}

	log.Println("Alerted telegram!")
	*alarmTime = time.Now()

	return nil
}

func (n *Notifier) AlertOnPoolWaitHeightFailure(height uint64, notReached map[string]uint64, reached map[string]uint64, notConnected []*health.NodeInfo) {
	if n.canAlert(n.lastSyncAlertTime) {
		msg := HeightAlertMsg(height, notReached, reached, notConnected)
		if err := n.SendAlert(msg, &n.lastSyncAlertTime); err != nil {
			log.Printf("Error sending alert on pool wait height failure: %v", err)
			return
		}
	}
}

func (n *Notifier) AlertOnInconsistentHashes(height uint64, hashes map[string]sdk.Hash) {
	// if n.canAlert(n.lastHashAlertTime) {
		msg := HashAlertMsg(height, hashes)
		if err := n.SendAlert(msg, &n.lastHashAlertTime); err != nil {
			log.Printf("Error sending alert on inconsistent block hashes: %v", err)
			return
		}
	// }
}

func HeightAlertMsg(height uint64, notReached map[string]uint64, reached map[string]uint64, notConnected []*health.NodeInfo) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "<b>⚠️ Fork Alert </b>\n\n")
	fmt.Fprintf(&buf, "Expected network height:  <b>%d</b>", height)

	if len(notReached) != 0 {
		fmt.Fprintf(&buf, "\n\nOut-of-sync:")
		fmt.Fprintf(&buf, "<pre>")
		for node, height := range notReached {
			fmt.Fprintf(&buf, "%-35v %-8v\n", node, height)
		}
		fmt.Fprintf(&buf, "</pre>")
	}

	if len(reached) != 0 {
		fmt.Fprintf(&buf, "\n\nSynced:")
		fmt.Fprintf(&buf, "<pre>")
		for node, height := range reached {
			fmt.Fprintf(&buf, "%-35v %-8v\n", node, height)
		}
		fmt.Fprintf(&buf, "</pre>")
	}

	if len(notConnected) != 0 {
		fmt.Fprintf(&buf, "\n\nFailed connections:")
		fmt.Fprintf(&buf, "<pre>")
		for _, node := range notConnected {
			fmt.Fprintf(&buf, "%s\n", strings.TrimSpace(node.Endpoint))
		}
		fmt.Fprintf(&buf, "</pre>")
	}

	return buf.String()
}

func HashAlertMsg(height uint64, hashes map[string]sdk.Hash) string {
	hashesGroup := make(map[sdk.Hash][]string)
	for endpoint, hash := range hashes {
		hashesGroup[hash] = append(hashesGroup[hash], endpoint)
	}

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "<b>❗Fork Alert </b>\n\n")
	fmt.Fprintf(&buf, "Inconsistent block hash at height:  <b>%d</b>\n", height)

	fmt.Fprintf(&buf, "<pre>")
	for hash, endpoints := range hashesGroup {
		fmt.Fprintf(&buf, "%s:\n\n", hash)
		for _, endpoint := range endpoints {
			fmt.Fprintln(&buf, endpoint)
		}
		fmt.Fprintf(&buf, "\n\n")
	}
	fmt.Fprintf(&buf, "</pre>")

	return buf.String()
}

func NewForkChecker(config Config) (*ForkChecker, error) {
	f := &ForkChecker{cfg: config}

	if err := f.initClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize catapult client: %v", err)
	}

	if err := f.initNotifier(); err != nil {
		return nil, fmt.Errorf("failed to initialize notifier: %v", err)
	}

	if err := f.initPool(); err != nil {
		return nil, fmt.Errorf("failed to initialize node health checker pool: %v", err)
	}

	if err := f.initCheckpoint(); err != nil {
		return nil, fmt.Errorf("failed to initialize checkpoint: %v", err)
	}

	return f, nil
}

func (f *ForkChecker) initClient() error {
	var conf *sdk.Config
	var err error

	for _, url := range f.cfg.ApiUrls {
		conf, err = sdk.NewConfig(context.Background(), []string{url})
		if err == nil {
			log.Printf("Initialized client on URL: %s", url)
			break
		}
	}

	if err != nil {
		return fmt.Errorf("all provided URLs failed: %v", err)
	}

	f.catapultClient = sdk.NewClient(nil, conf)

	return nil
}

func (f *ForkChecker) initNotifier() error {
	bot, err := tgbotapi.NewBotAPI(f.cfg.BotAPIKey)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram bot: %v", err)
	}

	bot.Debug = false

	f.notifier = &Notifier{
		bot,
		f.cfg.ChatID,
		time.Duration(f.cfg.AlarmInterval) * AlarmInterval,
		time.Time{},
		time.Time{},
		f.cfg.Notify,
	}

	return nil
}

func (f *ForkChecker) initPool() error {
	clientKeyPair, err := crypto.NewRandomKeyPair()
	if err != nil {
		return fmt.Errorf("error generating random keypair: %s", err)
	}

	nodeInfos, err := ParseNodes(f.cfg.Nodes)
	if err != nil {
		return fmt.Errorf("error parsing node info: %v", err)
	}

	healthCheckerPool := health.NewNodeHealthCheckerPool(
		clientKeyPair,
		packets.NoneConnectionSecurity,
		math.MaxInt,
	)

	failedConnectionsNodes, err := healthCheckerPool.ConnectToNodes(nodeInfos, f.cfg.Discover)
	if err != nil {
		return err
	}

	f.executionTime = time.Now()
	f.nodeInfos = nodeInfos
	f.nodePool = healthCheckerPool
	f.failedConnectionsNodes = failedConnectionsNodes
	return nil
}

func (f *ForkChecker) initCheckpoint() error {
	if f.cfg.Checkpoint != 0 {
		f.checkpoint = f.cfg.Checkpoint
	} else {
		height, err := f.catapultClient.Blockchain.GetBlockchainHeight(context.Background())
		if err != nil {
			return fmt.Errorf("error getting blockchain height: %v", err)
		}

		f.checkpoint = uint64(height)
	}

	log.Println("Initialized checkpoint: ", f.checkpoint)

	return nil
}

func (f *ForkChecker) Start() error {
	var err error

	for {
		// Periodically discovers new peers in the network
		if time.Since(f.executionTime) >= PeersDiscoveryInterval {
			f.failedConnectionsNodes, err = f.nodePool.ConnectToNodes(f.nodeInfos, f.cfg.Discover)
			f.executionTime = time.Now()
		} else if len(f.failedConnectionsNodes) != 0 {
			f.failedConnectionsNodes, err = f.nodePool.ConnectToNodes(f.nodeInfos, false)
		}

		if err != nil {
			return fmt.Errorf("error connecting to nodes: %w", err)
		}
		log.Println("Failed connections nodes:", f.failedConnectionsNodes)

		// Wait for nodes to reach checkpoint height
		log.Println("Checkpoint:", f.checkpoint)
		notReached, reached, err := f.nodePool.WaitHeight(f.checkpoint)
		if err != nil {
			return fmt.Errorf("error waiting for nodes to reach height %d: %w", f.checkpoint, err)
		} else if len(notReached) != 0 || len(f.failedConnectionsNodes) != 0 {
			f.notifier.AlertOnPoolWaitHeightFailure(f.checkpoint, notReached, reached, f.failedConnectionsNodes)
		}

		// Check the block hash of last confirmed block
		lastConfirmedBlockHeight := f.checkpoint
		log.Printf("Checking block hash at %d height", lastConfirmedBlockHeight)

		success, hashes, err := f.nodePool.CheckHashes(lastConfirmedBlockHeight)
		if err != nil {
			return fmt.Errorf("error checking hashes at height %d: %w", lastConfirmedBlockHeight, err)
		} else if !success {
			f.notifier.AlertOnInconsistentHashes(lastConfirmedBlockHeight, hashes)
		}

		// Update checkpoint and sleep until the next checkpoint
		f.checkpoint += f.cfg.HeightCheckInterval
		time.Sleep(health.AvgSecondsPerBlock * time.Duration(f.cfg.HeightCheckInterval))
	}
}
