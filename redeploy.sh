#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_NAME="poll-tg-bot"

echo "==> Pulling latest image..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" pull

echo "==> Stopping and removing container '$CONTAINER_NAME'..."
docker stop "$CONTAINER_NAME" 2>/dev/null && docker rm "$CONTAINER_NAME" 2>/dev/null || echo "    Container not running, skipping."

echo "==> Starting services..."
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d

echo "==> Done. Container status:"
docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"

echo "==> Image digest:"
docker inspect "$CONTAINER_NAME" --format '{{.Image}}'

