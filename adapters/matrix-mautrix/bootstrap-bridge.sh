#!/usr/bin/env bash
# Generate and configure a real mautrix bridge config + registration for the
# local WhatFunnel compose network. Generated artifacts must never be committed.
set -euo pipefail

usage() {
  echo "Usage: $0 <telegram|messenger|instagram>"
  echo "Telegram also requires TELEGRAM_API_ID and TELEGRAM_API_HASH."
}

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 64
fi

bridge="$1"
case "$bridge" in
  telegram)
    image="dock.mau.dev/mautrix/telegram:v26.07"
    service="mautrix-telegram"
    port="29317"
    appservice_id="telegram"
    bot_username="telegrambot"
    ;;
  messenger)
    image="dock.mau.dev/mautrix/meta:v26.07"
    service="mautrix-messenger"
    port="29319"
    appservice_id="messenger"
    bot_username="messengerbot"
    ;;
  instagram)
    image="dock.mau.dev/mautrix/meta:ig-v26.07"
    service="mautrix-instagram"
    port="29320"
    appservice_id="instagram"
    bot_username="instagrambot"
    ;;
  *)
    usage >&2
    exit 64
    ;;
esac

if [[ "$bridge" == "telegram" ]]; then
  : "${TELEGRAM_API_ID:?Set TELEGRAM_API_ID from https://my.telegram.org/apps}"
  : "${TELEGRAM_API_HASH:?Set TELEGRAM_API_HASH from https://my.telegram.org/apps}"
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
data_dir="$repo_root/adapters/matrix-mautrix/bridges/$bridge"
container_user="$(id -u):$(id -g)"

if [[ -e "$data_dir/config.yaml" || -e "$data_dir/registration.yaml" ]]; then
  echo "Refusing to overwrite existing $bridge bridge data at $data_dir." >&2
  echo "Remove or move it explicitly if you intend to create a new bridge identity." >&2
  exit 1
fi

mkdir -p "$data_dir"

echo "Generating the initial $bridge config from $image..."
docker run --rm --user "$container_user" -v "$data_dir:/data" "$image"

config="$data_dir/config.yaml"
if [[ ! -f "$config" ]]; then
  echo "mautrix did not create $config" >&2
  exit 1
fi

# The upstream starter config has stable placeholders for the mandatory
# Docker fields. Fail closed if an upstream release changes those shapes.
replace_required() {
  local pattern="$1"
  local replacement="$2"
  if ! grep -Eq "$pattern" "$config"; then
    echo "Expected configuration field not found: $pattern" >&2
    exit 1
  fi
  sed -Ei "s#$pattern#$replacement#" "$config"
}

# mautrix releases have used both matrix.example.com and example.localhost in
# their generated homeserver placeholder.
replace_required 'address: https?://(matrix\.example\.com|example\.localhost(:[0-9]+)?)' 'address: http://synapse:8008'
replace_required 'domain: example\.com' 'domain: localhost'
replace_required 'address: http://localhost:[0-9]+' "address: http://$service:$port"
replace_required 'hostname: 127\.0\.0\.1' 'hostname: 0.0.0.0'
replace_required 'port: [0-9]+' "port: $port"
replace_required 'id: (telegram|meta|instagram)' "id: $appservice_id"
replace_required 'username: (telegrambot|metabot|instagrambot)' "username: $bot_username"
replace_required '"example\.com": user' '"localhost": user'
replace_required '"@admin:example\.com": admin' '"@admin:localhost": admin'
replace_required '    type: postgres' '    type: sqlite3-fk-wal'
replace_required '    uri: postgres://user:password@host/database\?sslmode=disable' '    uri: file:/data/bridge.db?_txlock=immediate'

if [[ "$bridge" == "telegram" ]]; then
  replace_required 'api_id: [0-9]+' "api_id: $TELEGRAM_API_ID"
  replace_required 'api_hash: [[:alnum:]]+' "api_hash: $TELEGRAM_API_HASH"
fi
if [[ "$bridge" == "messenger" ]]; then
  replace_required '    mode:$' '    mode: facebook'
  replace_required '    allow_messenger_com_on_fb: false' '    allow_messenger_com_on_fb: true'
fi

echo "Generating the $bridge application-service registration..."
docker run --rm --user "$container_user" -v "$data_dir:/data" "$image"

if [[ ! -f "$data_dir/registration.yaml" ]]; then
  echo "mautrix did not create $data_dir/registration.yaml" >&2
  exit 1
fi

echo
echo "$bridge is bootstrapped. Start all enabled bridges with:"
echo "  make bridges-up"
echo
echo "The registration and config contain secrets and are ignored by git."
