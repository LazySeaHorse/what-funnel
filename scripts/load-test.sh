#!/usr/bin/env bash
# ==============================================================================
# WhatFunnel HTTP Load Testing Script
# Simulates virtual users over a duration and verifies strict latency (<500ms p95)
# and error rate limits (<0.1%).
# ==============================================================================
set -euo pipefail

CONCURRENCY=500
DURATION="60s"
TARGET_URL="http://localhost:18080/healthz"
MAX_P95_SECS="0.5" # 500ms
MAX_ERROR_RATE="0.001" # 0.1%

while [[ $# -gt 0 ]]; do
  case "$1" in
    -c|--concurrency)
      CONCURRENCY="$2"
      shift 2
      ;;
    -z|--duration)
      DURATION="$2"
      shift 2
      ;;
    -u|--url)
      TARGET_URL="$2"
      shift 2
      ;;
    --max-p95)
      MAX_P95_SECS="$2"
      shift 2
      ;;
    --max-error-rate)
      MAX_ERROR_RATE="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

HEY_BIN=""
if command -v hey &>/dev/null; then
  HEY_BIN="$(command -v hey)"
elif [[ -x "/home/ubuntu/.local/bin/hey" ]]; then
  HEY_BIN="/home/ubuntu/.local/bin/hey"
elif [[ -x "$(pwd)/.cache/bin/hey" ]]; then
  HEY_BIN="$(pwd)/.cache/bin/hey"
else
  echo "Error: 'hey' load testing tool not found in PATH or .cache/bin/hey."
  echo "Install with: go install github.com/rakyll/hey@latest"
  exit 1
fi

echo "=========================================================="
echo " Starting WhatFunnel HTTP Load Test"
echo " Target:       $TARGET_URL"
echo " Concurrency:  $CONCURRENCY virtual users"
echo " Duration:     $DURATION"
echo " Max p95:      ${MAX_P95_SECS}s (500ms)"
echo " Max err rate: 0.1%"
echo " Runner:       $HEY_BIN"
echo "=========================================================="

TEMP_OUTPUT=$(mktemp)
trap 'rm -f "$TEMP_OUTPUT"' EXIT

"$HEY_BIN" -c "$CONCURRENCY" -z "$DURATION" "$TARGET_URL" | tee "$TEMP_OUTPUT"

# Extract 95% latency in seconds
P95_VAL=$(grep "95%% in" "$TEMP_OUTPUT" | head -n1 | awk '{print $3}')
if [[ -z "$P95_VAL" ]]; then
  echo "Error: Could not extract 95% latency from hey output"
  exit 1
fi

# Extract 200 response count and total response count
SUCCESS_COUNT=$(grep -E '^\s*\[200\]' "$TEMP_OUTPUT" | awk '{print $2}' || echo "0")
if [[ -z "$SUCCESS_COUNT" ]]; then SUCCESS_COUNT=0; fi

TOTAL_REQS=$(grep -E '^\s*\[[0-9]{3}\]' "$TEMP_OUTPUT" | awk '{sum += $2} END {print sum+0}')
if [[ "$TOTAL_REQS" -eq 0 ]]; then
  echo "Error: Zero HTTP responses received during load test"
  exit 1
fi

ERRORS=$(( TOTAL_REQS - SUCCESS_COUNT ))

ERROR_RATE=$(awk "BEGIN {printf \"%.6f\", $ERRORS / $TOTAL_REQS}")

echo ""
echo "=========================================================="
echo " Load Test Results & Assertions"
echo "=========================================================="
echo " Total Requests: $TOTAL_REQS"
echo " Successful 200: $SUCCESS_COUNT"
echo " Failed/Errors:  $ERRORS"
echo " Error Rate:     $(awk "BEGIN {printf \"%.4f%%\", $ERROR_RATE * 100}") (Threshold: <= 0.1%)"
echo " 95th Percentile: ${P95_VAL}s (Threshold: <= ${MAX_P95_SECS}s)"

FAILED=0

# Compare P95 using awk
P95_EXCEEDED=$(awk "BEGIN {print ($P95_VAL > $MAX_P95_SECS) ? 1 : 0}")
if [[ "$P95_EXCEEDED" -eq 1 ]]; then
  echo "FAIL: 95th percentile latency (${P95_VAL}s) exceeded limit (${MAX_P95_SECS}s)!"
  FAILED=1
else
  echo "PASS: 95th percentile latency (${P95_VAL}s) <= ${MAX_P95_SECS}s"
fi

# Compare Error rate using awk
ERR_EXCEEDED=$(awk "BEGIN {print ($ERROR_RATE > $MAX_ERROR_RATE) ? 1 : 0}")
if [[ "$ERR_EXCEEDED" -eq 1 ]]; then
  echo "FAIL: Error rate ($(awk "BEGIN {printf \"%.4f%%\", $ERROR_RATE * 100}")) exceeded limit (0.1%)!"
  FAILED=1
else
  echo "PASS: Error rate ($(awk "BEGIN {printf \"%.4f%%\", $ERROR_RATE * 100}")) <= 0.1%"
fi

echo "=========================================================="
if [[ "$FAILED" -eq 1 ]]; then
  echo "Load test FAILED"
  exit 1
else
  echo "Load test PASSED"
  exit 0
fi
