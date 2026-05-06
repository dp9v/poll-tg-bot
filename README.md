# poll-tg-bot

A Telegram bot that periodically polls the [Alteg](https://alteg.io) platform,
caches every activity (and its service / category / coach metadata) in
PostgreSQL, and notifies a Telegram chat when availability changes.

## Features

- **Two independent schedulers:**
  - **Near** (default 15 min, `NEAR_INTERVAL`) — fetches the current week and
    the two following weeks. Diffed against the cache and **drives Telegram
    notifications**.
  - **Long-term** (default 60 min, `LONGTERM_INTERVAL`) — fetches the rest of
    the current month plus the next two months. **Cache-only**, no chat
    notifications.
- Transparent pagination via the API's `count`/`page` query parameters
  (`PAGE_SIZE` env var).
- Stores activities in PostgreSQL alongside denormalized lookup tables
  (`staff`, `services`, `categories`).
- Requests a new Alteg bearer token via Telegram when the current one expires.
- Schema versioning with [goose](https://github.com/pressly/goose); migrations
  embedded into the binary and applied on every start.
- Multi-arch Docker image (`linux/amd64`, `linux/arm64`).

## Project structure

```
.
├── main.go              # wires loader + notifier together
├── Dockerfile
├── docker-compose.yml
├── internal/
│   ├── alteg/           # Alteg API client (paginated search)
│   ├── bot/             # Telegram bot (sender + token dialog)
│   ├── config/          # Environment-based config
│   ├── loader/          # Two API-poll schedulers, writes to PostgreSQL
│   ├── notifier/        # Watches the cache, computes diffs, sends Telegram messages
│   ├── timewindow/      # Shared near / long-term date-range helpers
│   └── storage/         # PostgreSQL persistence
│       └── migrations/  # Embedded SQL schema migrations (goose)
└── docs/
    ├── deploy.md        # Deploy & push instructions
    └── requirements/
        ├── requirements.md
        ├── ai-agent-instructions.md
        └── search-response-example.json
```

## Architecture

```
                  ┌──────────────────┐
                  │   Alteg API      │
                  └────────┬─────────┘
                           │ paginated search
                           ▼
   ┌─────────────────────────────────────────┐
   │              Loader                     │
   │  • near scheduler   (default 15m)       │
   │  • long-term sched. (default 60m)       │
   │  • token-renewal dialog                 │
   │  • startup / error notifications        │
   └─────────────┬─────────────────┬─────────┘
                 │ Save()          │ NearLoaded() chan
                 ▼                 │
        ┌────────────────┐         │
        │  PostgreSQL    │         │
        └────────┬───────┘         │
                 │ LoadBetween()   │
                 ▼                 ▼
        ┌────────────────────────────────────┐
        │           Notifier                 │
        │  • in-memory baseline              │
        │  • diff vs cache → +added/-removed │
        │  • Telegram availability messages  │
        └────────────────────────────────────┘
```

The **loader** is the only component that talks to the Alteg API and the only
writer to the database. The **notifier** never touches the API: it reacts to
the loader's `NearLoaded` channel (or its own safety-net ticker), reads the
fresh state from PostgreSQL, diffs it against an in-memory baseline and posts
Telegram notifications. On startup the notifier seeds its baseline from the
database so a process restart does not flood the chat with stale "new"
notifications.

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

| Variable            | Required | Default | Description                                                                  |
|---------------------|----------|---------|------------------------------------------------------------------------------|
| `BOT_TOKEN`         | ✅       | —       | Telegram bot token from @BotFather                                           |
| `CHAT_ID`           | ✅       | —       | Telegram chat ID to send notifications                                       |
| `BEARER_TOKEN`      | ✅       | —       | Alteg bearer token (can be updated live via Telegram)                        |
| `DATABASE_URL`      | ✅       | —       | PostgreSQL DSN, e.g. `postgres://user:pass@host:5432/dbname?sslmode=disable` |
| `NEAR_INTERVAL`     | ❌       | `15m`   | Poll interval for the near window (Go `time.Duration` — e.g. `5m`, `1h30m`)  |
| `LONGTERM_INTERVAL` | ❌       | `60m`   | Poll interval for the long-term window                                       |
| `PAGE_SIZE`         | ❌       | `200`   | Page size for paginated `search` API requests                                |
| `POSTGRES_PASSWORD` | ❌       | `bot`   | (docker-compose only) password for the bundled `postgres` service.           |

## Data model

The PostgreSQL schema is normalized into four tables:

| Table        | Purpose                                                                                       |
|--------------|-----------------------------------------------------------------------------------------------|
| `activities` | One row per activity slot (id, date, capacity, records_count, staff_id, service_id).          |
| `staff`      | Trainer metadata (id, name, specialization, avatar URL, rating).                              |
| `services`   | Activity types (id, title, category_id, price_min, price_max).                                |
| `categories` | Service categories (id, title, parent_id) — `parent_id = 0` means a top-level category.       |

Each fetcher run upserts everything inside a single transaction, in dependency
order: `categories` → `services` → `staff` → `activities`. Historical
activities are never deleted — only updated when the API reports new data.

