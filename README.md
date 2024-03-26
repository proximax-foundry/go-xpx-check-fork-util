# go-xpx-check-fork-util

The check fork util script is a tool to detect forked chain by comparing block hashes at a certain height among the provided api nodes, peer nodes, and their connected nodes. The script runs at regular intervals and sends notifications through a Telegram bot when it detects any forks. 

Notifications are triggered in the following scenarios:
- When it identifies an inconsistent block hash on the last confirmed block, indicating a fork has occurred.
- When it detects nodes that are out of sync, potentially leading to a fork.

<br/>

## Getting started
### Prerequisites
* [Golang](https://golang.org/
) is required (tested on Go 1.20)
* [Telegram Bots](https://core.telegram.org/bots
) API Key and Chat Id 

<br/>

## Configurations

Configurations can be made to the script by changing the values to the fields in config.json.

```json
{
    "nodes": [
        {
            "endpoint": "localhost:7900",
            "IdentityKey": "4F7A80E9D6C2A4F5B46B90A1D16E95D4C1B8A3E8D5D1479D7C802C475D70A2E"
        },
        {
            "endpoint": "localhost:7901",
            "IdentityKey": "DA6B8ECFEBDDAA49CA26DEB8AC2F6346DBC9C8DD96B4584A01410190DAB4A45A"
        }     
    ],
    "apiUrls": [
        "https://localhost:3000",
        "https://localhost:3001"
    ],
    "discover": true,
    "checkpoint": 0,
    "heightCheckInterval": 5,
    "alarmInterval": 1,
    "botApiKey": "<TELEGRAM_BOT_API_KEY>",
    "chatID": 1234567,
    "notify": true
}
```

* `nodes`: List of nodes (API and PEER) to compare block hashes.
    * `Endpoint`: Node's host and port.
    * `IdentityKey` Node's public key.
* `apiUrls`: URLs of the REST servers.
* `discover`: Option to enable or disable peer discovery.
* `checkpoint`:  Specifies the initial chain height for conducting health checks. When set to 0, the script sets this checkpoint based on the current chain height obtained from the REST server.
* `heightCheckInterval`: Number of blocks between each block hash check.
* `alarmInterval`: Time interval (*in hours*) the telegram bot will send notification if a fork is detected.
* `botApiKey`: Telegram bot's API key.
* `chatID`: Telegram chat ID where notifications will be received.
* `notify`: Option to enable or disable telegram notification (`false` by default).
  
<br/>

## Usage
```bash
go build -o check-fork-util

# Running with default configuration file: "config.json"
./check-fork-util

# Running with specific configuration file using the `-file` flag
./check-fork-util -file "specific-config.json"
```
