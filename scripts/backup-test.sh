#!/usr/bin/env bash
# ==============================================================================
# WhatFunnel PostgreSQL Backup and Restore Verification Drill
# Automates a backup drill that exports the database with pg_dump,
# restores it to a clean database, and validates data integrity.
# ==============================================================================
set -euo pipefail

POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-whatfunnel}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-whatfunnel}"
SOURCE_DB="${POSTGRES_DB:-whatfunnel}"

RAND_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || date +%s)
RESTORE_DB="wf_drill_${RAND_ID//-/}"
BACKUP_FILE=$(mktemp /tmp/wf_backup_XXXXXX.sql)

cleanup() {
  echo "Cleaning up drill artifacts..."
  rm -f "$BACKUP_FILE"
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c "DROP DATABASE IF EXISTS $RESTORE_DB;" 2>/dev/null || true
}
trap cleanup EXIT

PSQL_BASE=(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -v ON_ERROR_STOP=1)
CORE_TABLES=(accounts users conversations messages channels)

# psql wrapper: run a scalar query against a database; any failure aborts the drill.
scalar() {
  local db="$1" query="$2" out
  if ! out=$(PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL_BASE[@]}" -d "$db" -t -A -c "$query"); then
    echo "FAIL: query failed on database '$db': $query" >&2
    exit 1
  fi
  echo "$out"
}

# Snapshot core-table row counts of the source DB as "table=count" lines.
snapshot_counts() {
  local t
  for t in "${CORE_TABLES[@]}"; do
    echo "$t=$(scalar "$SOURCE_DB" "SELECT count(*) FROM $t;")"
  done
}

echo "=========================================================="
echo " Starting WhatFunnel Backup & Restore Drill"
echo " Source Database:  $SOURCE_DB"
echo " Verification DB:  $RESTORE_DB"
echo " Backup File:      $BACKUP_FILE"
echo "=========================================================="

# 1. Export database with pg_dump.
# Row counts are captured immediately before and after the dump. The dump itself
# is a consistent snapshot, but the live source can keep changing, so the restored
# copy is compared against the pre-dump counts and the drill fails if the source
# changed while dumping (rerun against a quiescent database in that case).
echo "Step 1: Exporting database via pg_dump..."
BEFORE_COUNTS=$(snapshot_counts)
PGPASSWORD="$POSTGRES_PASSWORD" pg_dump \
  -h "$POSTGRES_HOST" \
  -p "$POSTGRES_PORT" \
  -U "$POSTGRES_USER" \
  -d "$SOURCE_DB" \
  --clean --if-exists --no-owner --no-acl > "$BACKUP_FILE"
AFTER_COUNTS=$(snapshot_counts)

if [[ "$BEFORE_COUNTS" != "$AFTER_COUNTS" ]]; then
  echo "FAIL: source row counts changed while the dump ran; cannot compare reliably." >&2
  echo "Before:"; echo "$BEFORE_COUNTS"
  echo "After:";  echo "$AFTER_COUNTS"
  exit 1
fi

FILE_SIZE=$(wc -c < "$BACKUP_FILE")
echo "Backup export complete (Size: $FILE_SIZE bytes)."
if [[ "$FILE_SIZE" -lt 1000 ]]; then
  echo "Error: Backup file is suspiciously small (< 1000 bytes)."
  exit 1
fi

# 2. Create clean temporary verification database
echo "Step 2: Creating clean verification database '$RESTORE_DB'..."
PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL_BASE[@]}" -d postgres -c "CREATE DATABASE $RESTORE_DB;"

# 3. Restore backup into verification database (abort on the first SQL error)
echo "Step 3: Restoring backup SQL dump into '$RESTORE_DB'..."
PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL_BASE[@]}" -d "$RESTORE_DB" -q < "$BACKUP_FILE"

# 4. Validate data integrity and schema match
echo "Step 4: Validating data integrity between source and restored database..."

TABLES_SQL="SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';"
SRC_TABLES=$(scalar "$SOURCE_DB" "$TABLES_SQL")
RESTORE_TABLES=$(scalar "$RESTORE_DB" "$TABLES_SQL")

echo "  Public tables in source:   $SRC_TABLES"
echo "  Public tables in restored: $RESTORE_TABLES"

if [[ "$SRC_TABLES" -ne "$RESTORE_TABLES" ]]; then
  echo "FAIL: Table count mismatch! Source: $SRC_TABLES, Restored: $RESTORE_TABLES"
  exit 1
fi

# Verify row counts in core tables against the pre-dump snapshot
while IFS='=' read -r TABLE SRC_ROWS; do
  RESTORE_ROWS=$(scalar "$RESTORE_DB" "SELECT count(*) FROM $TABLE;")
  echo "  Table '$TABLE': source rows (pre-dump) = $SRC_ROWS, restored rows = $RESTORE_ROWS"
  if [[ "$SRC_ROWS" -ne "$RESTORE_ROWS" ]]; then
    echo "FAIL: Row count mismatch in table $TABLE!"
    exit 1
  fi
done <<< "$BEFORE_COUNTS"

echo "=========================================================="
echo " PASS: Backup and restore drill succeeded!"
echo " Schema table count and core-table row counts match the pre-dump snapshot."
echo "=========================================================="
exit 0
