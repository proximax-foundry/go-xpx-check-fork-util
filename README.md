# go-xpx-check-fork-util

The check fork util script is a tool to detect forked chain by comparing block hashes of provided api nodes at a certain height. The script will repeat at a given time interval and notify any fork detected though a telegram bot notification.

## Getting started
### Prerequisites
* [Golang](https://golang.org/
) is required (tested on Go 1.17)
* [Telegram Bots API](https://core.telegram.org/bots
) Key and Chat Id 

### Clone the repository:
```
go get github.com/proximax-foundry/go-xpx-check-fork-util
cd go-xpx-check-fork-util
```

## Configurations

Multiple configurations to the script can be done by making changes to variables in the [config.json](./config.json) file.

structure of configuration file:
```json
{
  "notif": false,
  "apiNodes": [ 
    "https://arcturus.xpxsirius.io",
    "https://aldebaran.xpxsirius.io",
    "https://bigcalvin.xpxsirius.io",
    "https://betelgeuse.xpxsirius.io",
    "https://lyrasithara.xpxsirius.io",
    "https://delphinus.xpxsirius.io"
  ],
  "sleepInterval": 60,
  "alarmInterval": 1,
  "pruneHeight": 360,
  "botApiKey": "<TELEGRAM_BOT_API_KEY>",
  "chatID": <TELEGRAM_CHAT_ID>
}
```

* `notif`: Option to enable telegram notifications (`false` by default)
* `apiNodes`: List of api nodes to be compared
* `sleepInterval`: The time interval (in seconds) where the script will perform block hash comparisons 
* `alarmInterval`: The time interval (in hours) where telegram bot will send notification where a fork is detected 
* `pruneHeight`: The block height interval of last confirmed block.
* `botApiKey`: Telegram Bot Api Key
* `chatID`: Telegram Chat Id where notifications will be received.

## Selecting configuration file
The script will use [config.json](./config.json) as the default configuration, other configuration files can be selected using the `-file` flag.

**Example:**
```go
go run main.go -file "config.json"
```

## Running the script
```go
go run main.go
```
