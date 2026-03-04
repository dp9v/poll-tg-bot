# Deploy Instructions

## Push new changes to GitHub Container Registry

Replace `DD-MM-YYYY` with the current date.

```powershell
docker login --username dp9v --password <GITHUB_TOKEN> ghcr.io
docker buildx build --platform linux/amd64,linux/arm64 `
  -t ghcr.io/dp9v/poll-tg-bot:latest `
  -t ghcr.io/dp9v/poll-tg-bot:DD-MM-YYYY `
  --push .
```

---

## Run on server

Create a `.env` file next to `docker-compose.yml`:

```dotenv
BOT_TOKEN=<telegram bot token from @BotFather>
CHAT_ID=<telegram chat id>
BEARER_TOKEN=<alteg bearer token>
STORAGE_PATH=/data/tgbot.db
```

Create the data directory and set permissions:

```bash
mkdir -p ./data
sudo chown -R 100:101 ./data
sudo chmod -R 755 ./data
```

Then start the bot:

```bash
docker compose pull
docker compose up -d
docker compose logs -f
```

### Update to a new version

```bash
docker compose pull
docker compose up -d
```

The SQLite database is stored in the `./data` directory and survives updates.

---

## Inspect the database

```bash
sqlite3 ./data/tgbot.db
```

```sql
SELECT * FROM activities ORDER BY date DESC LIMIT 20;
```

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Container exits immediately | Check `docker compose logs` for the missing env variable |
| `401 Unauthorized` from Alteg | Bearer token expired — reply with a new one when the bot asks via Telegram |
