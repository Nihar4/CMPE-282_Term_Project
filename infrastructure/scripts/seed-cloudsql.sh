#!/usr/bin/env bash
# ============================================================
# Load seed data into Cloud SQL via Cloud SQL Proxy.
# Run ONCE after Cloud SQL is created.
# Usage: ./infrastructure/scripts/seed-cloudsql.sh
# ============================================================
set -euo pipefail

PROJECT_ID="enterprise-portal-48689"
REGION="us-central1"
INSTANCE="enterprise-portal-pg"
DB_NAME="enterprise_portal"
DB_USER="portal_user"
DB_PASS="${DB_PASSWORD:-portal_P@ssw0rd_Secure_2024!}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Start Cloud SQL Proxy
echo "Starting Cloud SQL Proxy..."
cloud-sql-proxy "${PROJECT_ID}:${REGION}:${INSTANCE}" --port=5433 &
PROXY_PID=$!
sleep 5

# Run migrations + seed
echo "Running migrations..."
PGPASSWORD="${DB_PASS}" psql \
  -h 127.0.0.1 -p 5433 \
  -U "${DB_USER}" -d "${DB_NAME}" \
  -f "${ROOT}/database/migrations/001_init.sql"

echo "Running seed (test_db data)..."
PGPASSWORD="${DB_PASS}" psql \
  -h 127.0.0.1 -p 5433 \
  -U "${DB_USER}" -d "${DB_NAME}" \
  -f "${ROOT}/database/seeds/002_testdb_sample.sql"

kill "${PROXY_PID}" 2>/dev/null || true
echo "Done! Cloud SQL seeded with test_db data."
