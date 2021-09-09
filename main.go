package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
)

type Config struct {
	ApiNodes      []string `json:"apiNodes"`
	Sleep         int      `json:"sleep"`
	BotApiKey     string   `json:"botApiKey"`
	ChatID        int64    `json:"chatID"`
	AlarmInterval int      `json:"alarmInterval"`
	PruneHeight   int      `json:"pruneHeight"`
}

var clients []*sdk.Client
var conf *sdk.Config

var alarmTime time.Time

func main() {
	var err error
	config, err := readConfig("config.json")
	errHandling(err)

	for {
		clients = nil
		fmt.Println()
		for _, apiNode := range config.ApiNodes {
			conf, err = sdk.NewConfig(context.Background(), []string{apiNode})
			if err != nil {
				fmt.Println(apiNode + " is Offline")
			} else {
				clients = append(clients, sdk.NewClient(nil, conf))

			}
		}
		checkHash(clients)
		time.Sleep(time.Duration(config.Sleep) * time.Second)

	}
}

func checkHash(clients []*sdk.Client) {
	config, err := readConfig("config.json")
	errHandling(err)
	alarmInterval := time.Duration(config.AlarmInterval) * time.Hour
	Red := "\033[31m"
	Reset := "\033[0m"

	pruneHeight := sdk.Height(config.PruneHeight)
	for _, checkingNode := range clients {

		height, err := checkingNode.Blockchain.GetBlockchainHeight(context.Background())
		errHandling(err)
		checkingHeight := height - pruneHeight

		checkBlock, err := checkingNode.Blockchain.GetBlockByHeight(context.Background(), checkingHeight)
		errHandling(err)

		checkNode, err := checkingNode.Node.GetNodeInfo(context.Background())

		errHandling(err)
		fmt.Print("\nCHECKING NODE: ", checkNode.Host)
		fmt.Println(" at height :", checkingHeight)

		for _, currentNode := range clients {
			curHeight, err := currentNode.Blockchain.GetBlockchainHeight(context.Background())
			errHandling(err)
			if (curHeight >= checkingHeight) && (checkingNode != currentNode) {
				curBlock, err := currentNode.Blockchain.GetBlockByHeight(context.Background(), checkingHeight)
				errHandling(err)
				curNode, err := currentNode.Node.GetNodeInfo(context.Background())
				errHandling(err)
				fmt.Print("comparing to node: ", curNode.Host)
				if curBlock.BlockHash.String() != checkBlock.BlockHash.String() {

					fmt.Println(string(Red), "\n\nFork Detected ! Block Height: ", checkingHeight, string(Reset))
					forkInfo := fmt.Sprintf("%s: %s\n%s: %s\n", curNode.Host, curBlock.BlockHash, checkNode.Host, checkBlock.BlockHash)
					fmt.Println(forkInfo)

					if time.Since(alarmTime) > alarmInterval {
						msgString := "Fork Detected ! Block Height: " + checkingHeight.String()
						sendAlert(msgString + forkInfo)
						alarmTime = time.Now()
					}

				} else {
					fmt.Println(" (Hash identical)")
				}

			}
		}
	}
}

func errHandling(err error) {
	if err != nil {
		panic(err)
	}
}

func readConfig(fileName string) (Config, error) {
	configFile, err := os.Open(fileName)
	var config Config
	if err != nil {
		return config, err
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	err = jsonParser.Decode(&config)
	return config, err
}

func sendAlert(msgString string) {
	config, _ := readConfig("config.json")

	bot, err := tgbotapi.NewBotAPI(config.BotApiKey)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	//log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	msg := tgbotapi.NewMessage(config.ChatID, msgString)

	bot.Send(msg)

}
