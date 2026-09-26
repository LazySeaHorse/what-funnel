#!/usr/bin/env bash
# ==============================================================================
# WhatFunnel Idle Memory Baseline Verification
# Inspects running WhatFunnel service containers via docker stats and asserts
# that each container consumes less than 256 MiB at idle.
# ==============================================================================
set -euo pipefail

MAX_ALLOWED_MIB=256
FAILED=0

echo "=========================================================="
echo " Checking WhatFunnel Idle Container Memory Usage"
echo " Threshold: < ${MAX_ALLOWED_MIB} MiB per container"
echo "=========================================================="
printf "%-32s %-16s %-12s %-8s\n" "CONTAINER" "MEMORY USAGE" "MIB USAGE" "STATUS"
printf "%-32s %-16s %-12s %-8s\n" "--------------------------------" "----------------" "------------" "------"

# Get docker stats for running whatfunnel containers
while IFS=$'\t' read -r name mem_usage; do
  # Filter only whatfunnel containers
  if [[ ! "$name" =~ ^whatfunnel- ]]; then
    continue
  fi

  # Skip migrate container if exited/stopped
  if [[ "$name" == "whatfunnel-migrate" ]]; then
    continue
  fi

  # Extract the used portion before the slash, e.g. "12.82MiB" or "0.15GiB" or "840KiB"
  raw_used=$(echo "$mem_usage" | awk -F'/' '{print $1}' | tr -d ' ')

  # Convert to MiB
  mib_val="0"
  if [[ "$raw_used" =~ GiB$ ]]; then
    val=${raw_used%GiB}
    mib_val=$(awk "BEGIN {printf \"%.2f\", $val * 1024}")
  elif [[ "$raw_used" =~ MiB$ ]]; then
    val=${raw_used%MiB}
    mib_val=$(awk "BEGIN {printf \"%.2f\", $val}")
  elif [[ "$raw_used" =~ KiB$ ]]; then
    val=${raw_used%KiB}
    mib_val=$(awk "BEGIN {printf \"%.2f\", $val / 1024}")
  elif [[ "$raw_used" =~ B$ ]]; then
    val=${raw_used%B}
    mib_val=$(awk "BEGIN {printf \"%.2f\", $val / 1048576}")
  fi

  status="PASS"
  exceeded=$(awk "BEGIN {print ($mib_val >= $MAX_ALLOWED_MIB) ? 1 : 0}")
  if [[ "$exceeded" -eq 1 ]]; then
    status="FAIL"
    FAILED=1
  fi

  printf "%-32s %-16s %-12s %-8s\n" "$name" "$raw_used" "${mib_val} MiB" "$status"
done < <(docker stats --no-stream --format "{{.Name}}\t{{.MemUsage}}")

echo "=========================================================="
if [[ "$FAILED" -eq 1 ]]; then
  echo "FAIL: One or more containers exceeded the 256 MiB idle threshold!"
  exit 1
else
  echo "PASS: All WhatFunnel containers consume < 256 MiB at idle."
  exit 0
fi
