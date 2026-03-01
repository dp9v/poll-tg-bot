# Deploy Instructions

## Push new changes to GitHub Container Registry

Replace `DD-MM-YYYY` with the current date.

```powershell
docker login --username dp9v --password <GITHUB_TOKEN> ghcr.io
docker build . -t ghcr.io/dp9v/poll-tg-bot:latest -t ghcr.io/dp9v/poll-tg-bot:DD-MM-YYYY
docker push ghcr.io/dp9v/poll-tg-bot:DD-MM-YYYY
docker push ghcr.io/dp9v/poll-tg-bot:latest
```

