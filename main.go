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
	"strconv"
	"text/template"
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
		cfg            Config
		notifier       *Notifier
		catapultClient *sdk.Client
		nodePool       *health.NodeHealthCheckerPool
		executionTime  time.Time
		checkpoint     uint64
	}
)

var (
	ErrEmptyNodes  = errors.New("Nodes cannot be empty")
	ErrEmptyApiUrl = errors.New("API url cannot be empty")
	ErrEmptyBotKey = errors.New("BotAPIKey cannot be empty")
	ErrEmptyChatId = errors.New("ChatID cannot be empty")
)

const (
	DefaultRollbackBlocks  = uint64(360)
	PeersDiscoveryInterval = time.Hour
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

	f.Start()
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

func (n *Notifier) AlertOnPoolWaitHeightFailure(height uint64, notReached map[string]uint64) {
	if n.canAlert(n.lastSyncAlertTime) {
		msg, err := HeightAlertString(height, notReached)
		if err != nil {
			log.Printf("Error creating height alert message: %v", err)
			return
		}

		if err := n.SendAlert(msg, &n.lastSyncAlertTime); err != nil {
			log.Printf("Error sending alert on pool wait height failure: %v", err)
			return
		}
	}
}

func (n *Notifier) AlertOnInconsistentHashes(height uint64, hashes map[string]sdk.Hash) {
	if n.canAlert(n.lastHashAlertTime) {
		msg, err := HashAlertString(height, hashes)
		if err != nil {
			log.Printf("Error creating hash alert message: %v", err)
			return
		}

		if err := n.SendAlert(msg, &n.lastHashAlertTime); err != nil {
			log.Printf("Error sending alert on inconsistent block hashes: %v", err)
			return
		}
	}
}

func HeightAlertString(height uint64, notReached map[string]uint64) (string, error) {
	tmplFile := "height-alert.html"
	tmpl, err := template.ParseFiles(tmplFile)
	if err != nil {
		return "", fmt.Errorf("error parsing template from '%s': %v", tmplFile, err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Height     uint64
		NotReached map[string]uint64
	}{
		height,
		notReached,
	})
	if err != nil {
		return "", fmt.Errorf("error execute template: %v", err)
	}

	return buf.String(), nil
}

func HashAlertString(height uint64, hashes map[string]sdk.Hash) (string, error) {
	hashesGroup := make(map[sdk.Hash][]string)
	for endpoint, hash := range hashes {
		hashesGroup[hash] = append(hashesGroup[hash], endpoint)
	}

	tmplFile := "hash-alert.html"
	tmpl, err := template.ParseFiles(tmplFile)
	if err != nil {
		return "", fmt.Errorf("error parsing template from '%s': %v", tmplFile, err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Height      uint64
		HashesGroup map[sdk.Hash][]string
	}{
		height,
		hashesGroup,
	})
	if err != nil {
		return "", fmt.Errorf("error execute template: %v", err)
	}

	return buf.String(), nil
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
		time.Duration(f.cfg.AlarmInterval) * time.Hour,
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

	healthCheckerPool, err := health.NewNodeHealthCheckerPool(
		clientKeyPair,
		nodeInfos,
		packets.NoneConnectionSecurity,
		f.cfg.Discover,
		math.MaxInt,
	)
	if err != nil {
		return err
	}

	f.nodePool = healthCheckerPool
	return nil
}

func (f *ForkChecker) RollbackDurationFromNetworkConfig() uint64 {
	config, err := f.catapultClient.Network.GetNetworkConfigAtHeight(context.Background(), sdk.Height(f.checkpoint))
	if err != nil {
		return DefaultRollbackBlocks
	}

	val, ok := config.NetworkConfig.Sections["chain"].Fields["maxRollbackBlocks"]
	if !ok {
		return DefaultRollbackBlocks
	}

	i, err := strconv.ParseUint(val.Value, 10, 64)
	if err != nil {
		return DefaultRollbackBlocks
	}

	return i
}

func (f *ForkChecker) initCheckpoint() error {
	height, err := f.catapultClient.Blockchain.GetBlockchainHeight(context.Background())
	if err != nil {
		return fmt.Errorf("error getting blockchain height: %v", err)
	}

	f.checkpoint = uint64(height)

	return nil
}

func (f *ForkChecker) Start() {
	f.executionTime = time.Now()

	for {
		log.Println("Checkpoint:", f.checkpoint)

		// Wait for nodes to reach checkpoint height
		notReached := f.nodePool.WaitHeight(f.checkpoint)
		if len(notReached) != 0 {
			f.notifier.AlertOnPoolWaitHeightFailure(f.checkpoint, notReached)
		}

		// Check the block hash of last confirmed block
		lastConfirmedBlockHeight := f.checkpoint - f.RollbackDurationFromNetworkConfig()
		log.Printf("Checking block hash at %d height", lastConfirmedBlockHeight)

		success, hashes := f.nodePool.CheckHashes(lastConfirmedBlockHeight)
		if !success {
			f.notifier.AlertOnInconsistentHashes(lastConfirmedBlockHeight, hashes)
		}

		// Periodically discover new peers in the network
		if f.cfg.Discover && time.Since(f.executionTime) >= PeersDiscoveryInterval {
			f.nodePool.CollectConnectedNodes()
			f.executionTime = time.Now()
		}

		// Update checkpoint and sleep until the next checkpoint
		f.checkpoint += f.cfg.HeightCheckInterval
		time.Sleep(health.AvgSecondsPerBlock * time.Duration(f.cfg.HeightCheckInterval))
	}
}
