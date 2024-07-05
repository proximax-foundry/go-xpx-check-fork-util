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
	"net"
	"os"
	"sort"
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
		lastPeerDiscoveryTime  time.Time
		lastNodeConnectionTime time.Time
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
	AlarmInterval          = time.Hour
	PeersDiscoveryInterval = time.Hour
	TenMinuteInterval      = 10 * time.Minute
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

func (n *Notifier) AlertOnPoolWaitHeightFailure(height uint64, notReached map[string]uint64, reached map[string]uint64, notConnected []*health.NodeInfo, immediate bool) {
	if immediate || n.canAlert(n.lastSyncAlertTime) {
		msg := HeightAlertMsg(height, notReached, reached, notConnected)
		if err := n.SendAlert(msg, &n.lastSyncAlertTime); err != nil {
			log.Printf("Error sending alert on pool wait height failure: %v", err)
			return
		}
	}
}

func (n *Notifier) AlertOnInconsistentHashes(height uint64, hashes map[string]sdk.Hash, immediate bool) {
	if immediate || n.canAlert(n.lastHashAlertTime) {
		msg := HashAlertMsg(height, hashes)
		if err := n.SendAlert(msg, &n.lastHashAlertTime); err != nil {
			log.Printf("Error sending alert on inconsistent block hashes: %v", err)
			return
		}
	}
}

func SortMapKeys(m map[string]uint64) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Checks if the input is a DNS name and abbreviates it if so.
func AbbreviateIfDNSName(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if ip := net.ParseIP(host); ip != nil {
		if port != "" {
			return host + ":" + port
		}
		return host
	}

	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		if port != "" {
			return parts[0] + ":" + port
		}
		return parts[0]
	}

	return address
}

func HeightAlertMsg(height uint64, notReached map[string]uint64, reached map[string]uint64, notConnected []*health.NodeInfo) string {
	var buf bytes.Buffer
	totalNodes := len(reached) + len(notReached) + len(notConnected)

	fmt.Fprintf(&buf, "<b> Alert </b>\n\n")
	// fmt.Fprintf(&buf, "Expected network height:  <b>%d</b>\n", height)
	fmt.Fprintf(&buf, "Total nodes:  <b>%d</b>", totalNodes)

	if len(notReached) != 0 {
		sortedNotReached := SortMapKeys(notReached)

		fmt.Fprintf(&buf, "\n\nOut-of-sync  (%d):", len(notReached))
		fmt.Fprintf(&buf, "<pre>")
		for _, node := range sortedNotReached {
			abbreviatedNode := AbbreviateIfDNSName(node)
			buf.WriteString(fmt.Sprintf("%-22s %-7d\n", abbreviatedNode, notReached[node]))
		}
		fmt.Fprintf(&buf, "</pre>")
	}

	if len(reached) != 0 {
		sortedReached := SortMapKeys(reached)

		fmt.Fprintf(&buf, "\n\nSynced  (%d):", len(reached))
		fmt.Fprintf(&buf, "<pre>")
		for _, node := range sortedReached {
			abbreviatedNode := AbbreviateIfDNSName(node)
			buf.WriteString(fmt.Sprintf("%-24s %-7d\n", abbreviatedNode, reached[node]))
		}
		fmt.Fprintf(&buf, "</pre>")
	}

	if len(notConnected) != 0 {
		fmt.Fprintf(&buf, "\n\nFailed connections  (%d):", len(notConnected))
		fmt.Fprintf(&buf, "<pre>")
		for _, node := range notConnected {
			endpoint := AbbreviateIfDNSName(node.Endpoint)
			fmt.Fprintf(&buf, "%-24s\n", strings.TrimSpace(endpoint))
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
	fmt.Fprintf(&buf, "Inconsistent block hash:  <b>%d</b>\n", height)

	fmt.Fprintf(&buf, "<pre>")
	for hash, endpoints := range hashesGroup {
		fmt.Fprintf(&buf, "%s:\n\n", hash)
		sort.Strings(endpoints)
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

	f.lastPeerDiscoveryTime = time.Now()
	f.lastNodeConnectionTime = f.lastPeerDiscoveryTime
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

func (f *ForkChecker) discoverPeers() (err error) {
	if time.Since(f.lastPeerDiscoveryTime) >= PeersDiscoveryInterval {
		log.Printf("Discover peers in the network.")
		f.failedConnectionsNodes, err = f.nodePool.ConnectToNodes(f.nodeInfos, f.cfg.Discover)
		f.lastPeerDiscoveryTime = time.Now()
		f.lastNodeConnectionTime = f.lastPeerDiscoveryTime
		return err
	}

	return nil
}

func (f *ForkChecker) connectToMustConnectNodes() (err error) {
	if time.Since(f.lastNodeConnectionTime) >= TenMinuteInterval {
		log.Printf("Connect to must-connect nodes.")
		f.failedConnectionsNodes, err = f.nodePool.ConnectToNodes(f.nodeInfos, false)
		f.lastNodeConnectionTime = time.Now()
		log.Println("Failed connections nodes:", f.failedConnectionsNodes)
		return err
	}

	return nil
}

func (f *ForkChecker) ResetPeersDiscoveryTime() {
	f.lastPeerDiscoveryTime = time.Time{}
	time.Sleep(TenMinuteInterval / 2)
}

func (f *ForkChecker) Start() error {
	var err error

	for {
		// Periodically discovers new peers in the network
		err = f.discoverPeers()
		if err != nil {
			return fmt.Errorf("error connecting to nodes: %w", err)
		}

		// Connect to must-connect nodes
		err = f.connectToMustConnectNodes()
		if err != nil {
			return fmt.Errorf("error connecting to nodes: %w", err)
		}

		// Wait for nodes to reach checkpoint height
		log.Println("Checkpoint: ", f.checkpoint)
		notReached, reached, err := f.nodePool.WaitHeight(f.checkpoint)
		if err != nil {
			log.Printf("Error waiting for nodes to reach height %d: %s", f.checkpoint, err)
			f.ResetPeersDiscoveryTime()
			continue
		}

		if len(reached) == 0 {
			log.Println("Blockchain stuck at height: ", f.checkpoint)
			f.notifier.AlertOnPoolWaitHeightFailure(f.checkpoint, notReached, reached, f.failedConnectionsNodes, false)
			f.ResetPeersDiscoveryTime()
			continue
		}

		if len(notReached) >= 5 {
			f.notifier.AlertOnPoolWaitHeightFailure(f.checkpoint, notReached, reached, f.failedConnectionsNodes, false)
		}

		// Check the block hash of last confirmed block
		log.Printf("Checking block hash at %d height", f.checkpoint)

		success, hashes, err := f.nodePool.CheckHashes(f.checkpoint)
		if err != nil {
			log.Printf("Error checking hashes at height %d: %s", f.checkpoint, err)
			f.ResetPeersDiscoveryTime()
			continue
		}

		if !success && !containsEmptyHashWhenTwoUniques(hashes) {
			f.notifier.AlertOnInconsistentHashes(f.checkpoint, hashes, true)
		}

		// Update checkpoint
		f.checkpoint += f.cfg.HeightCheckInterval
	}
}

func containsEmptyHashWhenTwoUniques(hashes map[string]sdk.Hash) bool {
	uniqueHashes := make(map[sdk.Hash]bool)

	for _, hash := range hashes {
		uniqueHashes[hash] = true
	}

	if len(uniqueHashes) == 2 {
		for hash := range uniqueHashes {
            if hash.Empty() {
                return true
            }
        }
	}

	return false
}
