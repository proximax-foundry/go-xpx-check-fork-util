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
		Discover       bool     `json:"discover"`
		HeightCheckInterval uint64   `json:"heightCheckInterval"`
		AlarmInterval       uint64   `json:"alarmInterval"`
		BotAPIKey           string   `json:"botApiKey"`
		ChatID              int64    `json:"chatID"`
		Notify              bool     `json:"notify"`
	}

	Notifier struct {
		bot               *tgbotapi.BotAPI
		lastHashAlertTime time.Time
		lastSyncAlertTime time.Time
	}

	ForkChecker struct {
		cfg            Config
		notifier       *Notifier
		catapultClient *sdk.Client
		nodePool       *health.NodeHealthCheckerPool
	}
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
		return errors.New("Nodes cannot be empty")
	}

	if len(c.ApiUrls) == 0 {
		return errors.New("API url cannot be empty")
	}

	if c.BotAPIKey == "" {
		return errors.New("BotAPIKey cannot be empty")
	}

	if c.ChatID == 0 {
		return errors.New("ChatID cannot be empty")
	}

	return nil
}

func ParseNodes(nodes Nodes) ([]*health.NodeInfo, error) {
	nodeInfos := make([]*health.NodeInfo, 0, len(nodes))

	for _, node := range nodes {
		ni, err := health.NewNodeInfo(node.IdentityKey, node.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("Error parsing node info: %v", err)
		}

		nodeInfos = append(nodeInfos, ni)
	}

	return nodeInfos, nil
}

func (n *Notifier) SendAlert(chatID int64, text string, alarmTime *time.Time) error {
	msgConfig := tgbotapi.NewMessage(chatID, text)
	msgConfig.ParseMode = "HTML"

	_, err := n.bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("failed to send message to telegram: %v", err)
	}

	log.Println("Alerted telegram!")
	*alarmTime = time.Now()

	return nil
}

func (n *Notifier) AlertOnPoolWaitHeightFailure(chatID int64, height uint64, notReached map[string]uint64, interval time.Duration) {
	msg := HeightAlertString(height, notReached)
	if n.lastSyncAlertTime.IsZero() || n.lastSyncAlertTime.Before(time.Now().Add(-interval)) {
		if err := n.SendAlert(chatID, msg, &n.lastSyncAlertTime); err != nil {
			log.Printf("Error alerting on pool wait height failure: %v", err)
		}
	}
}

func (n *Notifier) AlertOnInconsistentHashes(chatID int64, height uint64, hashes map[string]sdk.Hash, interval time.Duration) {
	msg := HashAlertString(height, hashes)
	if n.lastHashAlertTime.IsZero() || n.lastHashAlertTime.Before(time.Now().Add(-interval)) {
		if err := n.SendAlert(chatID, msg, &n.lastHashAlertTime); err != nil {
			log.Printf("Error alerting on inconsistent block hashes: %v", err)
		}
	}
}

func HeightAlertString(height uint64, notReached map[string]uint64) string {
	tmpl, err := template.ParseFiles("height-alert.html")
	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Height     uint64
		NotReached map[string]uint64
	}{
		height,
		notReached,
	})

	return buf.String()
}

func HashAlertString(height uint64, hashes map[string]sdk.Hash) string {
	hashesGroup := make(map[sdk.Hash][]string)
	for endpoint, hash := range hashes {
		hashesGroup[hash] = append(hashesGroup[hash], endpoint)
	}

	tmpl, err := template.ParseFiles("hash-alert.html")
	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Height      uint64
		HashesGroup map[sdk.Hash][]string
	}{
		height,
		hashesGroup,
	})

	return buf.String()
}

func NewForkChecker(config Config) (*ForkChecker, error) {
	f := &ForkChecker{cfg: config}

	client, err := f.initClient()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize catapult client: %v", err)
	}

	notifier, err := f.initNotifier()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize telegram bot: %v", err)
	}

	pool, err := f.initPool()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize node health checker pool: %v", err)
	}

	f.catapultClient = client
	f.notifier = notifier
	f.nodePool = pool

	return f, nil
}

func (f *ForkChecker) initClient() (*sdk.Client, error) {
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
		return nil, fmt.Errorf("All provided URLs failed: %v", err)
	}

	return sdk.NewClient(nil, conf), nil
}

func (f *ForkChecker) initNotifier() (*Notifier, error) {
	bot, err := tgbotapi.NewBotAPI(f.cfg.BotAPIKey)
	if err != nil {
		return nil, err
	}

	bot.Debug = false

	return &Notifier{bot, time.Time{}, time.Time{}}, nil
}

func (f *ForkChecker) initPool() (*health.NodeHealthCheckerPool, error) {
	clientKeyPair, err := crypto.NewRandomKeyPair()
	if err != nil {
		return nil, fmt.Errorf("Error generating random keypair: %s", err)
	}

	nodeInfos, err := ParseNodes(f.cfg.Nodes)
	if err != nil {
		return nil, err
	}

	healthCheckerPool, err := health.NewNodeHealthCheckerPool(
		clientKeyPair,
		nodeInfos,
		packets.NoneConnectionSecurity,
		f.cfg.Discover,
		math.MaxInt,
	)
	if err != nil {
		return nil, err
	}

	return healthCheckerPool, nil
}

func (f *ForkChecker) RollbackDurationFromNetworkConfig(height uint64) uint64 {
	config, err := f.catapultClient.Network.GetNetworkConfigAtHeight(context.Background(), sdk.Height(height))
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

func (f *ForkChecker) CurrentBlockchainHeight(ctx context.Context) (uint64, error) {
	height, err := f.catapultClient.Blockchain.GetBlockchainHeight(context.Background())
	if err != nil {
		return 0, nil
	}

	return uint64(height), nil
}

func (f *ForkChecker) Start() {
	checkpoint, err := f.CurrentBlockchainHeight(context.Background())
	if err != nil {
		log.Fatalf("Error getting blockchain height: %v", err)
	}

	executionTime := time.Now()
	alarmInterval := time.Duration(f.cfg.AlarmInterval) * time.Hour

	for {
		notReached := f.nodePool.WaitHeight(checkpoint)
		if len(notReached) != 0 {
			f.notifier.AlertOnPoolWaitHeightFailure(f.cfg.ChatID, checkpoint, notReached, alarmInterval)
		}

		// Check the block hash of last confirmed block
		lastConfirmedBlockHeight := checkpoint - f.RollbackDurationFromNetworkConfig(checkpoint)
		log.Printf("Checking block hash at %d height", lastConfirmedBlockHeight)

		success, hashes := f.nodePool.CheckHashes(lastConfirmedBlockHeight)
		if !success {
			f.notifier.AlertOnInconsistentHashes(f.cfg.ChatID, lastConfirmedBlockHeight, hashes, alarmInterval)
		}

		// Periodically seeks for new peers in the network
		if f.cfg.Discover && time.Since(executionTime) >= PeersDiscoveryInterval {
			f.nodePool.CollectConnectedNodes()
		}

		checkpoint += f.cfg.HeightCheckInterval
		time.Sleep(15 * time.Second * time.Duration(f.cfg.HeightCheckInterval))
	}

}
