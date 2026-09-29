#!/usr/bin/env bash
# resource-watchdog.sh
# Monitors RAM and root disk usage every 3 seconds and LOGS when either reaches
# the threshold (default 90%).
#
# By default this script never kills anything. To opt in to killing runaway
# processes on memory pressure, set WATCHDOG_TARGET_PATTERN to an extended regex
# that matches the command line (`ps -o args`) of processes that are safe to
# kill, e.g.:
#   WATCHDOG_TARGET_PATTERN='go test|playwright|chromium' ./scripts/resource-watchdog.sh
# Only the highest-memory process matching that pattern is sent SIGKILL.
# Disk pressure is only ever logged.

LOG_FILE="${WATCHDOG_LOG_FILE:-/tmp/resource-watchdog.log}"
THRESHOLD="${WATCHDOG_THRESHOLD_PCT:-90}"
TARGET_PATTERN="${WATCHDOG_TARGET_PATTERN:-}"

if [ -n "$TARGET_PATTERN" ]; then
    mode="kill processes matching /${TARGET_PATTERN}/ on RAM pressure"
else
    mode="log only (set WATCHDOG_TARGET_PATTERN to enable killing)"
fi
echo "[$(date -u)] Resource watchdog started. Thresholds: RAM/Disk >= ${THRESHOLD}%. Mode: ${mode}" >> "${LOG_FILE}"

while true; do
    mem_total=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
    mem_avail=$(awk '/MemAvailable/ {print $2}' /proc/meminfo)

    if [ -n "$mem_total" ] && [ -n "$mem_avail" ] && [ "$mem_total" -gt 0 ]; then
        mem_used=$((mem_total - mem_avail))
        mem_pct=$(( (mem_used * 100) / mem_total ))
    else
        mem_pct=0
    fi

    disk_pct=$(df / --output=pcent | tail -n 1 | tr -d ' %')

    if [ "$mem_pct" -ge "$THRESHOLD" ]; then
        echo "[$(date -u)] ALERT: High Memory Usage detected: ${mem_pct}%!" >> "${LOG_FILE}"
        if [ -n "$TARGET_PATTERN" ]; then
            # Highest-memory process whose command line matches the allowlist
            # (excluding this script itself).
            top_pid=$(ps -eo pid,args --sort=-%mem --no-headers \
                | awk -v self="$$" -v pat="$TARGET_PATTERN" '$1 != self && $0 ~ pat {print $1; exit}')
            if [ -n "$top_pid" ]; then
                proc_info=$(ps -p "$top_pid" -o pid,%mem,comm,args --no-headers)
                echo "[$(date -u)] Terminating allowlisted memory hog (PID $top_pid): $proc_info" >> "${LOG_FILE}"
                kill -9 "$top_pid" 2>/dev/null || true
            else
                echo "[$(date -u)] No process matches WATCHDOG_TARGET_PATTERN; nothing killed." >> "${LOG_FILE}"
            fi
        fi
    fi

    if [ "$disk_pct" -ge "$THRESHOLD" ]; then
        echo "[$(date -u)] ALERT: High Disk Usage detected: ${disk_pct}% (log only)." >> "${LOG_FILE}"
    fi

    sleep 3
done
