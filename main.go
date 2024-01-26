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

var (
	config              *Config
	alarmTime           time.Time
	heightCheckInterval time.Duration
	alarmInterval       time.Duration
	redColorFormatter   func(format string, a ...interface{}) string
)

const (
	defaultRollbackDuration = uint64(360)
	timeoutDuration         = time.Duration(10 * time.Minute)
)

type Config struct {
	Nodes []struct {
		Endpoint    string `json:"endpoint"`
		IdentityKey string `json:"IdentityKey"`
	} `json:"nodes"`
	ApiUrl              string `json:"apiUrl"`
	HeightCheckInterval uint64 `json:"heightCheckInterval"`
	AlarmInterval       uint64 `json:"alarmInterval"`
	BotAPIKey           string `json:"botApiKey"`
	ChatID              int64  `json:"chatID"`
	Notify              bool   `json:"notify"`
}

func init() {
	fileName := flag.String("file", "config.json", "Name of file to load config from")
	flag.Parse()

	config = &Config{}
	if err := config.Load(*fileName); err != nil {
		log.Fatal("Error:", err)
	}

	if err := config.Validate(); err != nil {
		log.Fatal("Validation Error:", err)
	}

	heightCheckInterval = time.Duration(config.HeightCheckInterval) * 15 * time.Second
	alarmInterval = time.Duration(config.AlarmInterval) * time.Hour
	redColorFormatter = color.New(color.FgRed).SprintfFunc()
}

func main() {
	nodeInfos, err := config.ParseNodes()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := initPool(nodeInfos)
	if err != nil {
		log.Fatal(err)
	}

	client := config.initClient()

	currentHeight, err := client.Blockchain.GetBlockchainHeight(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	checkpoint := uint64(currentHeight)

	err = pool.WaitHeightAll(checkpoint, timeoutDuration)
	if err != nil {
		log.Fatal(err)
	}

	err = pool.WaitAllHashesEqual(checkpoint)
	if err != nil {
		log.Fatal(err)
	}

	executionTime := time.Now()

	for {
		pool.WaitHeightAll(checkpoint, timeoutDuration)

		checkingHeight := checkpoint - RollbackDurationFromNetworkConfig(client, sdk.Height(checkpoint))

		log.Printf("Checking block hash at %d height", checkingHeight)
		hashes := pool.FindInconsistentHashesAtHeight(checkingHeight)
		if hashes != nil {
			log.Print(redColorFormatter("Fork Detected! Block Height: %d", checkingHeight))
			for endpoint, hash := range hashes {
				log.Print(redColorFormatter("%s: %s", endpoint, hash))
			}

			formattedAlert := ConstructFormattedAlert(checkingHeight, hashes)
			if err := config.SendAlert(formattedAlert, alarmInterval); err != nil {
				log.Print(err)
			}
		}

		// Periodically search for new peers to add to the connection pool
		if time.Since(executionTime) >= (2 * time.Hour) {
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

func (c *Config) initClient() *sdk.Client {
	conf, err := sdk.NewConfig(context.Background(), []string{c.ApiUrl})
	if err != nil {
		log.Fatalf("Error initializing config from url %s:", c.ApiUrl, err)
	}

	return sdk.NewClient(nil, conf)
}

func RollbackDurationFromNetworkConfig(client *sdk.Client, height sdk.Height) uint64 {
	config, err := client.Network.GetNetworkConfigAtHeight(context.Background(), height)
	if err != nil {
		return defaultRollbackDuration
	}

	val, ok := config.NetworkConfig.Sections["chain"].Fields["maxRollbackBlocks"]
	if !ok {
		return defaultRollbackDuration
	}

	i, err := strconv.ParseUint(val.Value, 10, 64)
	if err != nil {
		return defaultRollbackDuration
	}

	return i
}

func (c *Config) Load(fileName string) error {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("Error reading config file '%s': %v", fileName, err)
	}

	err = json.Unmarshal(content, c)
	if err != nil {
		return fmt.Errorf("Error unmarshalling config file '%s': %v", fileName, err)
	}

	return nil
}

func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return errors.New("Nodes cannot be empty")
	}

	if c.ApiUrl == "" {
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

func (c *Config) SendAlert(msg string, alarmInterval time.Duration) error {
	if !(c.Notify && time.Since(alarmTime) >= alarmInterval) {
		return nil
	}

	bot, err := tgbotapi.NewBotAPI(c.BotAPIKey)
	if err != nil {
		return fmt.Errorf("Failed to initialize telegram bot: %v", err)
	}

	bot.Debug = false

	msgConfig := tgbotapi.NewMessage(c.ChatID, msg)
	msgConfig.ParseMode = "HTML"

	_, err = bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("Failed to send message to telegram: %v", err)
	}

	log.Println("Alerted telegram!")
	alarmTime = time.Now()

	return nil
}

func ConstructFormattedAlert(height uint64, hashes map[string]sdk.Hash) string {
	var builder strings.Builder
	builder.WriteString("❗Fork Detected\n\n")
	builder.WriteString("<i><b>Block Height: </b>")
	builder.WriteString(strconv.Itoa(int(height)))
	builder.WriteString("</i>\n")

	var hashesMsg strings.Builder
	for endpoint, hash := range hashes {
		hashesMsg.WriteString(endpoint)
		hashesMsg.WriteString(":\n")
		hashesMsg.WriteString(hash.String())
		hashesMsg.WriteString("\n\n")
	}

	builder.WriteString("<pre>")
	builder.WriteString(hashesMsg.String())
	builder.WriteString("</pre>")

	return builder.String()
}
