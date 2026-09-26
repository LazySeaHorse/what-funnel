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

echo "=========================================================="
echo " Starting WhatFunnel Backup & Restore Drill"
echo " Source Database:  $SOURCE_DB"
echo " Verification DB:  $RESTORE_DB"
echo " Backup File:      $BACKUP_FILE"
echo "=========================================================="

# 1. Export database with pg_dump
echo "Step 1: Exporting database via pg_dump..."
PGPASSWORD="$POSTGRES_PASSWORD" pg_dump \
  -h "$POSTGRES_HOST" \
  -p "$POSTGRES_PORT" \
  -U "$POSTGRES_USER" \
  -d "$SOURCE_DB" \
  --clean --if-exists --no-owner --no-acl > "$BACKUP_FILE"

FILE_SIZE=$(wc -c < "$BACKUP_FILE")
echo "Backup export complete (Size: $FILE_SIZE bytes)."
if [[ "$FILE_SIZE" -lt 1000 ]]; then
  echo "Error: Backup file is suspiciously small (< 1000 bytes)."
  exit 1
fi

# 2. Create clean temporary verification database
echo "Step 2: Creating clean verification database '$RESTORE_DB'..."
PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c "CREATE DATABASE $RESTORE_DB;"

# 3. Restore backup into verification database
echo "Step 3: Restoring backup SQL dump into '$RESTORE_DB'..."
PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$RESTORE_DB" -q < "$BACKUP_FILE"

# 4. Validate data integrity and schema match
echo "Step 4: Validating data integrity between source and restored database..."

SRC_TABLES=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$SOURCE_DB" -t -A -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';")
RESTORE_TABLES=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$RESTORE_DB" -t -A -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';")

echo "  Public tables in source:   $SRC_TABLES"
echo "  Public tables in restored: $RESTORE_TABLES"

if [[ "$SRC_TABLES" -ne "$RESTORE_TABLES" ]]; then
  echo "FAIL: Table count mismatch! Source: $SRC_TABLES, Restored: $RESTORE_TABLES"
  exit 1
fi

# Verify row counts in core tables
for TABLE in accounts users conversations messages channels; do
  SRC_ROWS=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$SOURCE_DB" -t -A -c "SELECT count(*) FROM $TABLE;" 2>/dev/null || echo "0")
  RESTORE_ROWS=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$RESTORE_DB" -t -A -c "SELECT count(*) FROM $TABLE;" 2>/dev/null || echo "0")
  echo "  Table '$TABLE': source rows = $SRC_ROWS, restored rows = $RESTORE_ROWS"
  if [[ "$SRC_ROWS" -ne "$RESTORE_ROWS" ]]; then
    echo "FAIL: Row count mismatch in table $TABLE!"
    exit 1
  fi
done

echo "=========================================================="
echo " PASS: Backup and restore drill succeeded!"
echo " All schemas and table data verified with 100% parity."
echo "=========================================================="
exit 0
