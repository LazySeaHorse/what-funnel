#!/usr/bin/env bash
# resource-watchdog.sh
# Monitors RAM and Disk usage every 3 seconds.
# If memory or root disk usage reaches >= 90%, it aggressively kills the
# highest-consuming test/subagent process to prevent system lockup.

LOG_FILE="/tmp/resource-watchdog.log"
echo "[$(date -u)] Resource watchdog started. Thresholds: RAM >= 90%, Disk >= 90%" >> "${LOG_FILE}"

while true; do
    # 1. Check Memory Usage (percentage)
    # Using /proc/meminfo or free -m
    mem_total=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
    mem_avail=$(awk '/MemAvailable/ {print $2}' /proc/meminfo)
    
    if [ -n "$mem_total" ] && [ -n "$mem_avail" ] && [ "$mem_total" -gt 0 ]; then
        mem_used=$((mem_total - mem_avail))
        mem_pct=$(( (mem_used * 100) / mem_total ))
    else
        mem_pct=0
    fi

    # 2. Check Root Disk Usage (percentage)
    disk_pct=$(df / --output=pcent | tail -n 1 | tr -d ' %')

    if [ "$mem_pct" -ge 90 ]; then
        echo "[$(date -u)] ALERT: High Memory Usage detected: ${mem_pct}%!" >> "${LOG_FILE}"
        # Find top CPU/Memory process that is not dockerd/postgres/systemd
        # Specifically target go test, python, node, agy, or test runners
        top_pid=$(ps -eo pid,%mem,comm,args --sort=-%mem | awk 'NR>1 && $3 !~ /(systemd|sshd|dockerd|containerd|postgres|redis-server)/ {print $1}' | head -n 1)
        if [ -n "$top_pid" ]; then
            proc_info=$(ps -p "$top_pid" -o pid,%mem,comm,args --no-headers)
            echo "[$(date -u)] Terminating top memory process (PID $top_pid): $proc_info" >> "${LOG_FILE}"
            kill -9 "$top_pid" 2>/dev/null || true
        fi
    fi

    if [ "$disk_pct" -ge 90 ]; then
        echo "[$(date -u)] ALERT: High Disk Usage detected: ${disk_pct}%!" >> "${LOG_FILE}"
        # Target runaway test processes
        top_pid=$(ps -eo pid,%cpu,%mem,comm,args --sort=-%cpu | awk 'NR>1 && $3 !~ /(systemd|sshd|dockerd|containerd|postgres|redis-server)/ {print $1}' | head -n 1)
        if [ -n "$top_pid" ]; then
            proc_info=$(ps -p "$top_pid" -o pid,%mem,comm,args --no-headers)
            echo "[$(date -u)] Terminating process due to disk exhaustion (PID $top_pid): $proc_info" >> "${LOG_FILE}"
            kill -9 "$top_pid" 2>/dev/null || true
        fi
    fi

    sleep 3
done
