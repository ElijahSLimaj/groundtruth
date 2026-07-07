#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export DATABASE_URL="${DATABASE_URL:-postgresql://postgres:postgres@127.0.0.1:54322/postgres}"
export PAYLOAD_ROOT="${PAYLOAD_ROOT:-$PWD/.payloads}"
mkdir -p "$PAYLOAD_ROOT"

supabase start >/dev/null
supabase migration up

pids=()
cleanup() {
  for pid in "${pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

(cd services/ingestion && go run ./cmd/ingestion) &
pids+=($!)

(cd services/embedding && go run ./cmd/embedding) &
pids+=($!)

(cd services/serving && SCHEDULER_ENABLED=1 pnpm start:dev) &
pids+=($!)

(cd apps/web && pnpm dev) &
pids+=($!)

echo "brain is running: web http://localhost:3000, serving http://localhost:3001"
wait
