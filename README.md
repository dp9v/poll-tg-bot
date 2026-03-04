# poll-tg-bot

A Telegram bot that polls the [Alteg](https://alteg.io) platform every 5 minutes and notifies a Telegram chat when available activity slots change.

## Features

- Polls Alteg activity API on a configurable interval (default: 5 min)
- Sends a Telegram message only when availability changes (new slots or capacity updates)
- Stores activity history in a local SQLite database that survives restarts
- Requests a new Alteg bearer token via Telegram when the current one expires
- Multi-arch Docker image (`linux/amd64`, `linux/arm64`)

## Project structure

```
.
├── main.go
├── Dockerfile
├── docker-compose.yml
├── internal/
│   ├── alteg/       # Alteg API client
│   ├── bot/         # Telegram bot (sender + token dialog)
│   ├── config/      # Environment-based config
│   ├── notifier/    # Polling loop & diff logic
│   └── storage/     # SQLite persistence
└── docs/
    ├── deploy.md    # Deploy & push instructions
    └── requirements/
        ├── requirements.md
        ├── ai-agent-instructions.md
        └── search-response-example.json
```

## Quick start (local)

1. Copy `.env.example` to `.env` and fill in the values.
2. Run:
   ```bash
   docker compose up -d
   ```

## Documentation

- [Deploy instructions](docs/deploy.md) — how to push a new image and run it on a server

## Environment variables

| Variable       | Description                              |
|----------------|------------------------------------------|
| `BOT_TOKEN`    | Telegram bot token from @BotFather       |
| `CHAT_ID`      | Telegram chat ID to send notifications   |
| `BEARER_TOKEN` | Alteg bearer token (can be updated live) |
| `STORAGE_PATH` | Path to the SQLite database file         |

