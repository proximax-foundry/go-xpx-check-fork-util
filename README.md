# go-xpx-check-fork-util

The check fork util script is a tool to detect forked chain by comparing block hashes of provided api nodes and peer nodes at a certain height. The script will repeat at a given time interval and notify any fork detected though a telegram bot notification.

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
        }         
    ],
    "connectionRetryInterval": 1,
    "alarmInterval": 1,
    "heightCheckInterval": 10,
    "pruneHeight": 360, 
    "botApiKey": "<TELEGRAM_BOT_API_KEY>",
    "chatID": 1234567,
    "notify": true
}
```

* `nodes`: List of nodes (API and PEER) to compare block hashes.
    * `Endpoint`: Node's host and port.
    * `IdentityKey` Node's public key.
* `connectionRetryInterval`: Time interval (*in hours*) to reconnect to nodes.
* `alarmInterval`: Time interval (*in hours*) the telegram bot will send notification if a fork is detected.
* `heightCheckInterval`: Number of blocks between each block hash check.
* `pruneHeight`: Block height interval of last confirmed block.
* `botApiKey`: Telegram bot's API key.
* `chatID`: Telegram chat ID where notifications will be received.
* `notify`: Option to enable or disable telegram notification (`false` by default).
  
<br/>

## Usage
```go
go build -o check-fork-util

// Running with default configuration file: "config.json"
./check-fork-util

// Running with specific configuration file using the `-file` flag
./check-fork-util -file "specific-config.json"
```
