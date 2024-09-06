package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Notifier) sendToTelegram(msg string) error {
	msgConfig := tgbotapi.NewMessage(n.chatID, msg)
	msgConfig.ParseMode = "HTML"

	_, err := n.bot.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("failed to send message to telegram: %v", err)
	}

	log.Printf("Alerted Telegram!")
	return nil
}
