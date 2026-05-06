# Requirements

A Telegram bot in Go that polls the Alteg platform for available activity slots and notifies a specific Telegram chat when availability changes.

---

## API

**Endpoint:** `GET https://n1338118.alteg.io/api/v1/activity/777474/search`

**Query parameters:**

| Parameter       | Value                                                        |
|-----------------|--------------------------------------------------------------|
| `from`          | Today's date (`YYYY-MM-DD`)                                  |
| `till`          | 1st day of next month + 2 months ahead (`YYYY-MM-DD`)        |
| `service_ids[]` | `12995896`                                                   |
| `staff_ids[]`   | `2811495`                                                    |

The date window is recalculated on every poll — the bot always looks ~3 months ahead from the current date.

Response example: [search-response-example.json](../requests/search-response-example.json)

---

## Business Logic

### Polling

- Poll the API every **5 minutes** continuously.
- First poll runs immediately on startup.

### Filtering

Keep only activities where `capacity > records_count` (i.e. at least one free slot exists).

### Diff & notification

Compare the current result with the last known state. A notification is sent when:

- A new activity appeared (was not in the previous result).
- An existing activity disappeared (no longer available).
- An existing activity's `capacity` changed.

If the result is identical to the previous poll, no message is sent.

### Notification format

Each Telegram message lists all currently available activities. For each activity:

| Field             | Source                                                              |
|-------------------|---------------------------------------------------------------------|
| Date              | `activity.date` (date part)                                         |
| Time              | `activity.date` (time part)                                         |
| Title             | `activity.service.title`                                            |
| Coach             | `activity.staff.name`                                               |
| Available places  | `capacity − records_count`                                          |
| Booking link      | `https://n1338118.alteg.io/company/777474/activity/info/<id>`       |

### Token renewal

If the API returns **HTTP 401**, the bot:
1. Sends a Telegram message asking the user to paste a new bearer token.
2. Waits up to **10 minutes** for a reply in the same chat.
3. On receiving the token, resumes polling immediately.
4. If the timeout expires, logs the event and retries on the next scheduled poll.

### Error handling

- On any non-401 API error, send a short error message to the Telegram chat.
- Do **not** repeat the same error on consecutive failures.
- Resend only after the error has been resolved and reappears.

---

## Storage

Activity state is persisted in a **SQLite database** (`tgbot.db`):

- On every successful poll, available activities are upserted (inserted or updated by `id`).
- Historical records are **not deleted** — only updated when capacity changes.
- On startup, previously saved activities for the current date window are loaded to seed the diff state, avoiding false-positive notifications after a restart.

---

## Configuration

All configuration is provided via environment variables:

| Variable       | Required | Default          | Description                            |
|----------------|----------|------------------|----------------------------------------|
| `BOT_TOKEN`    | ✅       | —                | Telegram bot token from @BotFather     |
| `CHAT_ID`      | ✅       | —                | Telegram chat ID for notifications     |
| `BEARER_TOKEN` | ✅       | —                | Alteg API bearer token                 |
| `STORAGE_PATH` | ❌       | `/data/tgbot.db` | Path to the SQLite database file       |

---

## Deployment

- Docker image built with a **multi-stage `Dockerfile`** (minimal Alpine-based final image).
- Supports **multi-arch** builds: `linux/amd64` and `linux/arm64`.
- `docker-compose.yml` is included for running on a server.
- The `/data` directory inside the container is a named volume — mount it to a host directory to persist the database across updates.
- See [deploy.md](../deploy.md) for build and deployment instructions.
