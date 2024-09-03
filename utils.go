package main

import (
	"net"
	"strings"

	"github.com/proximax-storage/go-xpx-chain-sdk/tools/health"
)

func insertSpaceIfExceedsLength(input string, maxLength int) string {
	if len(input) > maxLength {
		return input[:maxLength] + " " + input[maxLength:]
	}
	return input
}

// Checks if the input is a DNS name and abbreviates it if so.
func abbreviateIfDNSName(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if ip := net.ParseIP(host); ip != nil {
		return host
	}

	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		return parts[0]
	}

	return address
}

func parseNodes(nodes []Node) ([]*health.NodeInfo, error) {
	nodeInfos := make([]*health.NodeInfo, 0, len(nodes))

	for _, node := range nodes {
		ni, err := health.NewNodeInfo(node.IdentityKey, node.Endpoint, node.FriendlyName)
		if err != nil {
			return nil, err
		}

		nodeInfos = append(nodeInfos, ni)
	}

	return nodeInfos, nil
}
