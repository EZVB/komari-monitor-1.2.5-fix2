#!/usr/bin/env bash

set -Eeuo pipefail

PROJECT_DIR="${PROJECT_DIR:-/root/docker/komari}"
KOMARI_IMAGE="${KOMARI_IMAGE:-ghcr.io/ezvb/komari:1.2.5-fix2}"
KOMARI_PORT="${KOMARI_PORT:-25774}"
DEFAULT_ADMIN_USER="${ADMIN_USERNAME:-USA}"

log() {
  printf '\n==== %s ====\n' "$1"
}

die() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

quote_env_value() {
  local value="$1"

  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || \
    die "Environment values must not contain line breaks."

  value="${value//\'/\\\'}"
  printf "'%s'" "$value"
}

[[ "${EUID}" -eq 0 ]] || die "Run this installer as root."
command -v docker >/dev/null 2>&1 || die "Docker is not installed."
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is not installed."

case "$(uname -m)" in
  x86_64 | amd64 | aarch64 | arm64) ;;
  *) die "This image supports only amd64 and arm64 hosts." ;;
esac

[[ "$KOMARI_PORT" =~ ^[0-9]+$ ]] || die "KOMARI_PORT must be a number."
((KOMARI_PORT >= 1 && KOMARI_PORT <= 65535)) || \
  die "KOMARI_PORT must be between 1 and 65535."

if [[ -z "${ADMIN_USERNAME:-}" ]]; then
  read -r -p "Initial administrator username [${DEFAULT_ADMIN_USER}]: " ADMIN_USERNAME
  ADMIN_USERNAME="${ADMIN_USERNAME:-$DEFAULT_ADMIN_USER}"
fi

if [[ -z "${ADMIN_PASSWORD:-}" ]]; then
  read -r -s -p "Initial administrator password: " ADMIN_PASSWORD
  printf '\n'
fi

[[ -n "$ADMIN_USERNAME" ]] || die "Administrator username must not be empty."
[[ -n "$ADMIN_PASSWORD" ]] || die "Administrator password must not be empty."

log "Preparing project directory"
install -d -m 0750 "$PROJECT_DIR" "$PROJECT_DIR/data"
cd "$PROJECT_DIR"

if [[ -s data/komari.db ]]; then
  printf '%s\n' "Existing database detected. It will be preserved."
  printf '%s\n' "ADMIN_USERNAME and ADMIN_PASSWORD initialize only a new database."
fi

umask 077

log "Writing Docker Compose configuration"
{
  printf 'KOMARI_IMAGE=%s\n' "$(quote_env_value "$KOMARI_IMAGE")"
  printf 'KOMARI_PORT=%s\n' "$(quote_env_value "$KOMARI_PORT")"
  printf 'ADMIN_USERNAME=%s\n' "$(quote_env_value "$ADMIN_USERNAME")"
  printf 'ADMIN_PASSWORD=%s\n' "$(quote_env_value "$ADMIN_PASSWORD")"
} > .env
chmod 0600 .env

cat > docker-compose.yml <<'COMPOSE_EOF'
services:
  komari:
    image: ${KOMARI_IMAGE}
    container_name: komari
    restart: unless-stopped
    ports:
      - "${KOMARI_PORT}:25774"
    volumes:
      - ./data:/app/data
    environment:
      ADMIN_USERNAME: ${ADMIN_USERNAME}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:25774/ >/dev/null || exit 1"]
      interval: 5s
      timeout: 5s
      retries: 24
      start_period: 10s
COMPOSE_EOF
chmod 0600 docker-compose.yml

docker compose config --quiet

log "Pulling ${KOMARI_IMAGE}"
docker compose pull

log "Starting Komari"
docker compose up -d --remove-orphans

container_id="$(docker compose ps -q komari)"
[[ -n "$container_id" ]] || die "Komari container was not created."

log "Waiting for Komari to become healthy"
for _ in $(seq 1 60); do
  state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id" 2>/dev/null || true)"

  if [[ "$health" == "healthy" ]]; then
    break
  fi

  if [[ "$state" == "exited" || "$state" == "dead" ]]; then
    docker compose logs --tail=100 komari >&2 || true
    die "Komari stopped before becoming healthy."
  fi

  sleep 2
done

health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
if [[ "$health" != "healthy" ]]; then
  docker compose logs --tail=100 komari >&2 || true
  die "Komari did not become healthy within 120 seconds."
fi

host_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
host_ip="${host_ip:-127.0.0.1}"

log "Deployment complete"
docker compose ps
printf 'URL: http://%s:%s\n' "$host_ip" "$KOMARI_PORT"
printf 'Administrator username: %s\n' "$ADMIN_USERNAME"
printf '%s\n' "Administrator password was not printed or stored in shell history."
printf '%s\n' "Data directory: $PROJECT_DIR/data"
