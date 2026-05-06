# poll-tg-bot

A Telegram bot that polls the [Alteg](https://alteg.io) platform every 5 minutes and notifies a Telegram chat when available activity slots change.

## Features

- Polls Alteg activity API on a configurable interval (default: 5 min)
- Sends a Telegram message only when availability changes (new slots or capacity updates)
- Stores activity history in a PostgreSQL database that survives restarts
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
│   └── storage/     # PostgreSQL persistence
│       └── migrations/  # Embedded SQL schema migrations (goose)
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

## Database migrations

Schema changes live in [`internal/storage/migrations/`](internal/storage/migrations) as plain
`.sql` files using the [goose](https://github.com/pressly/goose) format
(`-- +goose Up` / `-- +goose Down`). They are embedded into the binary via
`//go:embed` and applied automatically on every start in `storage.New`, so
deployments don't need any extra steps.

To add a new migration, create a file with the next sequence number, e.g.
`00002_add_user_email.sql`:

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE activities ADD COLUMN comment TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE activities DROP COLUMN comment;
-- +goose StatementEnd
```

Files are applied in lexicographic order, so always pad the numeric prefix.

## Environment variables

| Variable            | Description                                                                                  |
|---------------------|----------------------------------------------------------------------------------------------|
| `BOT_TOKEN`         | Telegram bot token from @BotFather                                                           |
| `CHAT_ID`           | Telegram chat ID to send notifications                                                       |
| `BEARER_TOKEN`      | Alteg bearer token (can be updated live)                                                     |
| `DATABASE_URL`      | PostgreSQL DSN, e.g. `postgres://user:pass@host:5432/dbname?sslmode=disable`                 |
| `POSTGRES_PASSWORD` | (docker-compose only) password for the bundled `postgres` service. Defaults to `bot`.        |

