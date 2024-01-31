package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health/packets"
	crypto "github.com/proximax-storage/go-xpx-crypto"

	"github.com/fatih/color"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var redColorFormatter = color.New(color.FgRed).SprintfFunc()

const (
	DefaultRollbackBlocks  = uint64(360)
	WaitHeightTimeout      = time.Duration(10 * time.Minute)
	PeersDiscoveryInterval = time.Duration(1 * time.Hour)
)

type Config struct {
	Nodes []struct {
		Endpoint    string `json:"endpoint"`
		IdentityKey string `json:"IdentityKey"`
	} `json:"nodes"`
	ApiUrls             []string `json:"apiUrls"`
	HeightCheckInterval uint64   `json:"heightCheckInterval"`
	AlarmInterval       uint64   `json:"alarmInterval"`
	BotAPIKey           string   `json:"botApiKey"`
	ChatID              int64    `json:"chatID"`
	Notify              bool     `json:"notify"`
	lastHashAlertTime   *time.Time
	lastSyncAlertTime   *time.Time
}

func main() {
	fileName := flag.String("file", "config.json", "Name of file to load config from")
	flag.Parse()

	config, err := LoadConfig(*fileName)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	nodeInfos, err := config.ParseNodes()
	if err != nil {
		log.Fatal(err)
	}

	client, err := config.initClient()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := initPool(nodeInfos)
	if err != nil {
		log.Fatalf("Error initializing connection pool: %v", err)
	}

	currentHeight, err := client.Blockchain.GetBlockchainHeight(context.Background())
	if err != nil {
		log.Fatalf("Error getting blockchain height: %v", err)
	}

	checkpoint := uint64(currentHeight)
	alarmInterval := time.Duration(config.AlarmInterval) * time.Hour
	heightCheckInterval := time.Duration(config.HeightCheckInterval) * 15 * time.Second
	executionTime := time.Now()

	for {
		err := config.AlertOnPoolWaitHeightFailure(pool, checkpoint, WaitHeightTimeout, alarmInterval)
		if err != nil {
			log.Printf("Error alerting telegram: %v", err)
		}

		// Check the block hash of last confirmed block
		lastConfirmedBlockHeight := checkpoint - RollbackDurationFromNetworkConfig(client, sdk.Height(checkpoint))
		log.Printf("Checking block hash at %d height", lastConfirmedBlockHeight)

		err = config.AlertOnInconsistentHashes(pool, lastConfirmedBlockHeight, alarmInterval)
		if err != nil {
			log.Printf("Error alerting telegram: %v", err)
		}

		// Periodically seeks new peers to add to the connection pool at specified intervals
		if time.Since(executionTime) >= PeersDiscoveryInterval {
			pool.CollectConnectedNodes()
		}

		checkpoint += config.HeightCheckInterval
		time.Sleep(heightCheckInterval)
	}
}

func initPool(nodeInfos []*health.NodeInfo) (*health.NodeHealthCheckerPool, error) {
	clientKeyPair, err := crypto.NewRandomKeyPair()
	if err != nil {
		log.Fatalf("Error generating random keypair: %s", err)
	}

	return health.NewNodeHealthCheckerPool(clientKeyPair, nodeInfos, packets.NoneConnectionSecurity, true, math.MaxInt)
}

func (c *Config) initClient() (*sdk.Client, error) {
	var conf *sdk.Config
	var err error

	for _, url := range c.ApiUrls {
		conf, err = sdk.NewConfig(context.Background(), []string{url})
		if err == nil {
			log.Printf("Initialized client on URL: %s", url)
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("Failed to initialize client. All provided URLs failed: %v", err)
	}

	return sdk.NewClient(nil, conf), nil
}

func RollbackDurationFromNetworkConfig(client *sdk.Client, height sdk.Height) uint64 {
	config, err := client.Network.GetNetworkConfigAtHeight(context.Background(), height)
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

	config.lastHashAlertTime = new(time.Time)
	config.lastSyncAlertTime = new(time.Time)

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

func (c *Config) ParseNodes() ([]*health.NodeInfo, error) {
	nodeInfos := make([]*health.NodeInfo, 0, len(c.Nodes))

	for _, node := range c.Nodes {
		ni, err := health.NewNodeInfo(node.IdentityKey, node.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("Error parsing node info: %v", err)
		}

		nodeInfos = append(nodeInfos, ni)
	}

	return nodeInfos, nil
}

func (c *Config) SendAlert(msg string, alarmTime *time.Time, alarmInterval time.Duration) error {
	if !(c.Notify && (time.Since(*alarmTime) >= alarmInterval)) {
		return nil
	}

	bot, err := tgbotapi.NewBotAPI(c.BotAPIKey)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram bot: %v", err)
	}

	bot.Debug = false

	msgConfig := tgbotapi.NewMessage(c.ChatID, msg)
	msgConfig.ParseMode = "HTML"

	_, err = bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("failed to send message to telegram: %v", err)
	}

	log.Println("Alerted telegram!")
	*alarmTime = time.Now()

	return nil
}

// Sends an alert if the pool fails to wait for nodes reaching certain height
func (c *Config) AlertOnPoolWaitHeightFailure(pool *health.NodeHealthCheckerPool, height uint64, timeout time.Duration, alarmInterval time.Duration) error {
	err := pool.WaitHeightAll(height, timeout)
	if err != nil {
		msg := NetworkSyncAlertString(err.Error())
		return c.SendAlert(msg, c.lastSyncAlertTime, alarmInterval)
	}

	return nil
}

// Sends an alert if the pool finds inconsistent hashes at a certain height
func (c *Config) AlertOnInconsistentHashes(pool *health.NodeHealthCheckerPool, height uint64, alarmInterval time.Duration) error {
	hashes := pool.FindInconsistentHashesAtHeight(height)
	if hashes != nil {
		log.Print(redColorFormatter("Fork Detected! Block Height: %d", height))
		for endpoint, hash := range hashes {
			log.Print(redColorFormatter("%s: %s", endpoint, hash))
		}

		msg := BlockHashAlertString(height, hashes)
		return c.SendAlert(msg, c.lastHashAlertTime, alarmInterval)
	}

	return nil
}

func BlockHashAlertString(height uint64, hashes map[string]sdk.Hash) string {
	var builder strings.Builder
	builder.WriteString("<b>❗Fork Alert</b>\n\n")
	builder.WriteString("<i>Inconsistent block hash at height:  ")
	builder.WriteString(strconv.Itoa(int(height)))
	builder.WriteString("</i>\n")

	groups := make(map[sdk.Hash][]string)
	for endpoint, hash := range hashes {
		groups[hash] = append(groups[hash], endpoint)
	}

	var hashesMsg strings.Builder
	for hash, endpoints := range groups {
		for _, endpoint := range endpoints {
			hashesMsg.WriteString(endpoint)
			hashesMsg.WriteString(":\n")
			hashesMsg.WriteString(hash.String())
			hashesMsg.WriteString("\n\n")
		}

		hashesMsg.WriteString("\n\n")
	}

	builder.WriteString("<pre>")
	builder.WriteString(hashesMsg.String())
	builder.WriteString("</pre>")

	return builder.String()
}

func NetworkSyncAlertString(message string) string {
	msg := strings.SplitN(message, "\n", 2)

	var builder strings.Builder
	builder.WriteString("⚠️ <b>Fork Alert</b>\n\n")
	builder.WriteString("<i>")
	builder.WriteString(msg[0])
	builder.WriteString("</i>")
	builder.WriteString("\n\nOut-of-sync nodes:\n")

	builder.WriteString("<pre>")
	builder.WriteString(msg[1])
	builder.WriteString("</pre>")

	return builder.String()
}
