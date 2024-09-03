package main

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/proximax-storage/go-xpx-chain-sdk/sdk"
	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tablewriter "github.com/olekukonko/tablewriter"
)

type (
	AlertManager struct {
		config           AlertConfig
		lastAlertTimes   map[AlertType]time.Time
		lastStuckHeight  uint64
		lastStuckTime    time.Time
		offlineNodeStats map[string]NodeStatus
		nodeInfos        []*health.NodeInfo
		notifier         *Notifier
	}

	Notifier struct {
		bot     *tgbotapi.BotAPI
		chatID  int64
		enabled bool
	}

	Alert interface {
		createMessage() string
		getType() AlertType
	}

	SyncAlert struct {
		Height     uint64
		NotReached map[health.NodeInfo]uint64
		Reached    map[health.NodeInfo]uint64
	}

	HashAlert struct {
		Height uint64
		Hashes map[string]sdk.Hash
	}

	OfflineAlert struct {
		NotConnected map[string]*health.NodeInfo
	}

	AlertType int

	NodeStatus struct {
		consecutiveOfflineCount int
		lastOfflineAlertTime    time.Time
	}
)

const (
	OfflineAlertType AlertType = iota
	SyncAlertType
	HashAlertType
)

func (a SyncAlert) getType() AlertType {
	return SyncAlertType
}

func (a HashAlert) getType() AlertType {
	return HashAlertType
}

func (a OfflineAlert) getType() AlertType {
	return OfflineAlertType
}

func (a SyncAlert) writeSynced(buf *bytes.Buffer) {
	fmt.Fprintf(buf, "\n\nSynced at <b>%d</b> (%d):", a.Height, len(a.Reached))

	if len(a.Reached) == 0 {
		return
	}

	var nodesStr [][]string
	for node := range a.Reached {
		nodeStr := make([]string, 0, 1)
		host := abbreviateIfDNSName(node.Endpoint)

		if node.FriendlyName != "" && strings.TrimSpace(node.FriendlyName) != strings.TrimSpace(host) {
			nodeStr = append(nodeStr, fmt.Sprintf("%s(%s)", node.FriendlyName, host))
		} else {
			nodeStr = append(nodeStr, host)
		}

		nodesStr = append(nodesStr, nodeStr)
	}

	sort.Slice(nodesStr, func(i, j int) bool {
		return nodesStr[i][0] < nodesStr[j][0]
	})

	fmt.Fprintf(buf, "<pre>")

	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetBorder(false)
	table.SetAutoWrapText(true)
	table.SetNoWhiteSpace(true)
	table.SetTablePadding(" ")
	table.AppendBulk(nodesStr)
	table.Render()

	fmt.Fprintf(buf, "</pre>")
}

func (a SyncAlert) writeOutOfSync(buf *bytes.Buffer) {
	fmt.Fprintf(buf, "\n\nOut-of-sync (%d):", len(a.NotReached))

	if len(a.NotReached) == 0 {
		return
	}

	const (
		nodeCount = 5
		minWidth  = 23
		maxWidth  = 28
	)

	var nodeWidth int
	if len(a.NotReached) <= nodeCount {
		nodeWidth = minWidth
	} else {
		nodeWidth = maxWidth
	}

	var nodesStr [][]string
	for node, h := range a.NotReached {
		nodeStr := make([]string, 0, 2)
		host := abbreviateIfDNSName(node.Endpoint)

		if node.FriendlyName != "" && strings.TrimSpace(node.FriendlyName) != strings.TrimSpace(host) {
			nodeStr = append(nodeStr, insertSpaceIfExceedsLength(fmt.Sprintf("%s(%s)", node.FriendlyName, host), nodeWidth))
		} else {
			nodeStr = append(nodeStr, host)
		}

		nodeStr = append(nodeStr, fmt.Sprintf("%8s", strconv.FormatUint(h, 10)))
		nodesStr = append(nodesStr, nodeStr)
	}

	sort.Slice(nodesStr, func(i, j int) bool {
		return nodesStr[i][0] < nodesStr[j][0]
	})

	fmt.Fprintf(buf, "<pre>")

	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetBorder(false)
	table.SetAutoWrapText(true)
	table.SetNoWhiteSpace(true)
	table.SetTablePadding(" ")
	table.SetColWidth(nodeWidth)
	table.AppendBulk(nodesStr)
	table.Render()

	fmt.Fprintf(buf, "</pre>")
}

func (a SyncAlert) createMessage() string {
	var buf bytes.Buffer

	if len(a.Reached) == 0 {
		fmt.Fprintf(&buf, "<b>❗ Stuck Alert </b>")
	} else {
		fmt.Fprintf(&buf, "<b>⚠️ Warning </b>")
	}

	a.writeSynced(&buf)
	a.writeOutOfSync(&buf)

	return buf.String()
}

func (a HashAlert) createMessage() string {
	hashesGroup := make(map[sdk.Hash][]string)
	for endpoint, hash := range a.Hashes {
		hashesGroup[hash] = append(hashesGroup[hash], endpoint)
	}

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "<b>❗Fork Alert </b>\n\n")
	fmt.Fprintf(&buf, "Inconsistent block hash at:  <b>%d</b>\n", a.Height)

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

func (a OfflineAlert) createMessage() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<b>⚠️ Warning - Offline nodes </b>")
	fmt.Fprintf(&buf, "\n\nFailed connection  (%d):", len(a.NotConnected))

	fmt.Fprintf(&buf, "<pre>")
	var nodeStrings []string
	for _, node := range a.NotConnected {
		abbreviatedNode := abbreviateIfDNSName(node.Endpoint)
		nodeStr := abbreviatedNode
		if node.FriendlyName != "" && strings.TrimSpace(node.FriendlyName) != strings.TrimSpace(abbreviatedNode) {
			nodeStr = fmt.Sprintf("%s(%s)", node.FriendlyName, abbreviatedNode)
		}
		nodeStrings = append(nodeStrings, nodeStr)
	}
	sort.Strings(nodeStrings)

	for _, str := range nodeStrings {
		fmt.Fprintf(&buf, "%-37s\n", str)
	}
	fmt.Fprintf(&buf, "</pre>")

	return buf.String()
}

func (am *AlertManager) sendToTelegram(alert Alert) {
	if !am.notifier.enabled {
		return
	}

	msg := alert.createMessage()

	if err := am.notifier.sendToTelegram(msg); err != nil {
		log.Println(err)
		return
	}

	am.lastAlertTimes[alert.getType()] = time.Now()

	if alert.getType() == OfflineAlertType {
		am.updateNodeStatusLastOfflineAlertTime(alert)
	}
}

func (am *AlertManager) handleSyncAlert(checkpoint uint64, notReached, reached map[health.NodeInfo]uint64) {
	if am.shouldSendSyncAlert(checkpoint, notReached, reached) && time.Since(am.lastAlertTimes[SyncAlertType]) > am.config.getSyncAlertRepeatInterval(){
		am.sendToTelegram(SyncAlert{
			Height:     checkpoint,
			NotReached: notReached,
			Reached:    reached,
		})
	}
}

func (am *AlertManager) shouldSendSyncAlert(checkpoint uint64, notReached, reached map[health.NodeInfo]uint64) bool {
	if len(notReached) == 0 {
		return false
	}

	if len(reached) == 0 {
		return am.isStuckDurationReached(checkpoint)
	}

	criticalNodesCount := 0
	for _, info := range am.nodeInfos {
		if height, exists := notReached[*info]; exists {
			if int(checkpoint-height) >= am.config.OutOfSyncBlocksThreshold {
				criticalNodesCount++
				// fmt.Println("criticalNodesCount:", criticalNodesCount)
				if criticalNodesCount >= am.config.OutOfSyncCriticalNodesThreshold {
					return true
				}
			}
		}
	}

	return false
}

func (am *AlertManager) isStuckDurationReached(checkpoint uint64) bool {
	if am.lastStuckHeight == checkpoint {
		return time.Since(am.lastStuckTime) > am.config.getStuckDurationThreshold()
	}

	am.lastStuckHeight = checkpoint
	am.lastStuckTime = time.Now()

	return false
}

func (am *AlertManager) handleOfflineAlert(failedConnectionsNodes map[string]*health.NodeInfo) {
	if am.shouldSendOfflineAlert(failedConnectionsNodes){
		am.sendToTelegram(OfflineAlert{
			NotConnected: failedConnectionsNodes,
		})
	}
}

func (am *AlertManager) shouldSendOfflineAlert(failedConnectionsNodes map[string]*health.NodeInfo) bool {
	shouldAlert := false
	
	for _, info := range am.nodeInfos {
		identityKey := info.IdentityKey.String()
		if _, exists := failedConnectionsNodes[identityKey]; exists {

			status, exists := am.offlineNodeStats[identityKey]
			if !exists {
				status = NodeStatus{consecutiveOfflineCount: 1}
			} else {
				status.consecutiveOfflineCount++
			}

			am.updateNodeStatus(identityKey, status)

			if status.consecutiveOfflineCount > am.config.OfflineConsecutiveBlocksThreshold && time.Since(status.lastOfflineAlertTime) > am.config.getOfflineAlertRepeatInterval() {
				shouldAlert = true
			}
		} else {
			delete(am.offlineNodeStats, info.IdentityKey.String())
		}
	}

	return shouldAlert
}

func (am *AlertManager) updateNodeStatusLastOfflineAlertTime(alert Alert) {
	for key := range alert.(OfflineAlert).NotConnected {
		if status, exists := am.offlineNodeStats[key]; exists {
			status.lastOfflineAlertTime = time.Now()
			am.updateNodeStatus(key, status)
		}
	}
}

func (am *AlertManager) updateNodeStatus(key string, status NodeStatus) {
	am.offlineNodeStats[key] = status
}

func (am *AlertManager) handleHashAlert(checkpoint uint64, hashes map[string]sdk.Hash) {
	am.sendToTelegram(HashAlert{
		Height: checkpoint,
		Hashes: hashes,
	})
}
