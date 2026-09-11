#!/usr/bin/env bash

# Source this file before running tests inside Codex. The sandbox can read the
# image's global tools, but its user-global Go cache may be read-only.

codex_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
	if [[ -x /usr/local/go/bin/go ]]; then
		export PATH="/usr/local/go/bin:${PATH}"
	else
		echo "Go is not installed at /usr/local/go/bin/go" >&2
		return 1 2>/dev/null || exit 1
	fi
fi

export GOCACHE="${GOCACHE:-${codex_repo_root}/.cache/go-build}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="${PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH:-/usr/local/bin/chromium}"
export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD="${PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD:-1}"

mkdir -p "${GOCACHE}"

unset codex_repo_root
