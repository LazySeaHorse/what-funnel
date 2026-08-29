#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${repo_root}/scripts/codex-env.sh"

# Populate Go dependencies while setup-script network access and the writable
# container filesystem are available.
cd "${repo_root}"
go mod download all

cd "${repo_root}/apps/web"
npm ci
npx --no-install playwright install chromium

echo "Codex test environment is ready."
