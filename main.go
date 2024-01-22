package main

import (
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
	alarmTime time.Time
	config    Config
)

type Config struct {
	Nodes []struct {
		Endpoint    string `json:"endpoint"`
		IdentityKey string `json:"IdentityKey"`
	} `json:"nodes"`
	HeightCheckInterval     uint64 `json:"heightCheckInterval"`
	ConnectionRetryInterval uint64 `json:"connectionRetryInterval"`
	AlarmInterval           uint64 `json:"alarmInterval"`
	PruneHeight             uint64 `json:"pruneHeight"`
	BotAPIKey               string `json:"botApiKey"`
	ChatID                  int64  `json:"chatID"`
	Notify                  bool   `json:"notify"`
}

func main() {
	fileName := flag.String("file", "config.json", "Name of file to load config from")
	flag.Parse()

	if err := config.Load(*fileName); err != nil {
		log.Fatal("Error:", err)
	}

	if err := config.Validate(); err != nil {
		log.Fatal("Validation Error:", err)
	}

	nodeInfos, err := config.ParseNodes()
	if err != nil {
		log.Fatal(err)
	}

	client, err := crypto.NewRandomKeyPair()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := health.NewNodeHealthCheckerPool(client, nodeInfos, packets.NoneConnectionSecurity, false, math.MaxInt)
	if err != nil {
		log.Fatal(err)
	}

	heightCheckInterval := time.Duration(config.HeightCheckInterval) * 15 * time.Second
	alarmInterval := time.Duration(config.AlarmInterval) * time.Hour
	connectionRetryInterval := time.Duration(config.ConnectionRetryInterval) * time.Minute
	executionTime := time.Now()

	red := color.New(color.FgRed).SprintfFunc()

	for {
		maxHeight := pool.MaxHeight()

		if err := pool.WaitHeightAll(maxHeight); err != nil {
			log.Fatal(err)
		}

		checkingHeight := maxHeight - config.PruneHeight
		log.Printf("Checking block hash at %d height", checkingHeight)

		if hashes := pool.FindInconsistentHashesAtHeight(checkingHeight); hashes != nil {
			log.Print(red("Fork Detected! Block Height: %d", checkingHeight))
			for endpoint, hash := range hashes {
				log.Print(red("%s: %s", endpoint, hash))
			}

			formattedAlert := ConstructFormattedAlert(checkingHeight, hashes)
			if err := config.SendAlert(formattedAlert, alarmInterval); err != nil {
				log.Print(err)
			}
		}

		if time.Since(executionTime) >= connectionRetryInterval {
			pool = ReconnectAll(pool, nodeInfos, client)
			executionTime = time.Now()
		}

		time.Sleep(heightCheckInterval)
	}
}

func ReconnectAll(pool *health.NodeHealthCheckerPool, nodeInfos []*health.NodeInfo, client *crypto.KeyPair) *health.NodeHealthCheckerPool {
	log.Println("Resetting connection pool")

	if err := pool.Close(); err != nil {
		log.Fatal(err)
	}

	pool, err := health.NewNodeHealthCheckerPool(client, nodeInfos, packets.NoneConnectionSecurity, false, math.MaxInt)
	if err != nil {
		log.Fatal(err)
	}

	return pool
}

func (c *Config) Load(fileName string) error {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("error reading config file '%s': %v", fileName, err)
	}

	err = json.Unmarshal(content, c)
	if err != nil {
		return fmt.Errorf("error un-marshalling config file '%s': %v", fileName, err)
	}

	return nil
}

func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return errors.New("Nodes cannot be empty")
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
			return nil, fmt.Errorf("error parsing node info: %v", err)
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
		return fmt.Errorf("failed to initialize telegram bot: %v", err)
	}

	bot.Debug = false

	msgConfig := tgbotapi.NewMessage(c.ChatID, msg)
	msgConfig.ParseMode = "HTML"

	_, err = bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("failed to alert telegram: %v", err)
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
