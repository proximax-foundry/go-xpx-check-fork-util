# go-xpx-check-fork-util

The check fork utility script is a tool to detect chain forks by comparing block hashes at specific heights across the provided nodes and their connected nodes. The script runs at regular intervals and sends notifications through a Telegram bot if any forks are detected.

**Notifications will be triggered in the following scenarios:**
- **Hash Alert**: Triggered when an inconsistent block hash is detected on the last confirmed block, indicating a fork.
- **Out-of-Sync Alert**: Triggered if more than a specified number of nodes (from those listed in the config file) are out of sync, based on a block count difference threshold
- **Stuck Alert**: Triggered when no nodes have reached the checkpoint height within a specified duration, indicating that the blockchain is stuck.
- **Offline Alert**: Triggered when any nodes (from those listed in the config file) are detected as offline.

<br/>

## Getting started
### Prerequisites
* [Golang](https://golang.org/) is required (tested on Go 1.20)
* [Telegram Bots](https://core.telegram.org/bots) API Key and Chat Id 

<br/>

## Configurations

Configure the script by modifying the values in config.json.

```json
{
    "nodes": [
        {
            "endpoint": "127.0.0.1:7900",
            "IdentityKey": "4F7A80E9D6C2A4F5B46B90A1D16E95D4C1B8A3E8D5D1479D7C802C475D70A2E",
            "friendlyName": "nodeA"
        },
        {
            "endpoint": "127.0.0.2:7900",
            "IdentityKey": "DA6B8ECFEBDDAA49CA26DEB8AC2F6346DBC9C8DD96B4584A01410190DAB4A45A",
            "friendlyName": "nodeB"
        }     
    ],
    "apiUrls": [
        "http://127.0.0.1:3000",
        "http://127.0.0.2:3000"
    ],
    "discover": true,
    "checkpoint": 0,
    "heightCheckInterval": 1,
    "botApiKey": "<TELEGRAM_BOT_API_KEY>",
    "chatID": -1234567,
    "notify": true,
    "alertConfig": {
        "offlineAlertRepeatInterval": "2h",
        "offlineDurationThreshold": "5m",
        "syncAlertRepeatInterval": "2h",
        "stuckDurationThreshold": "10m",
        "outOfSyncBlocksThreshold": 5,
        "outOfSyncCriticalNodesThreshold": 5
    }
}
```

* `nodes`: List of nodes (both API and PEER) for comparing block hashes.
    * `endpoint`: Node's host and port.
    * `IdentityKey`: Node's public key.
    * `friendlyName`: Node's friendly name.
* `apiUrls`: URLs of the REST servers.
* `discover`: Option to enable or disable peer discovery.
* `checkpoint`:  Specifies the initial chain height for health checks. If set to 0, the script will determine the checkpoint based on the current chain height from the REST server.
* `heightCheckInterval`: Number of blocks between each block hash check.
* `botApiKey`:  API key for the Telegram bot.
* `chatID`: Telegram chat ID where notifications will be sent.
* `notify`: Option to enable or disable Telegram notifications.
* `alertConfig`
    * `offlineAlertRepeatInterval`: Time between repeated alerts for offline nodes.
    * `offlineDurationThreshold`: Duration that a node must remain offline before an alert is triggered.
    * `syncAlertRepeatInterval`: Time between repeated alerts for blockchain sync issues.
    * `stuckDurationThreshold`: Duration that the blockchain must remain stuck before an alert is triggered.
    * `outOfSyncBlocksThreshold`: Number of blocks difference that classifies nodes as out-of-sync.
    * `outOfSyncCriticalNodesThreshold`: Number of nodes (from those listed in the config file) that need to be classified as out of sync before an alert is triggered.
  
<br/>

## Usage
```bash
go build -o go-xpx-check-fork-util

# Running with default configuration file: "config.json"
./go-xpx-check-fork-util

# Running with specific configuration file using the `-file` flag
./go-xpx-check-fork-util -file "specific-config.json"
```
