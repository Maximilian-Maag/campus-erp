#!/bin/bash
# Restore the HIS-ERP dump into a disposable Postgres sandbox and (optionally)
# regenerate the schema artefacts. Reproduces every file under schema/.
#
# Usage: scripts/restore.sh /path/to/hisrm_muster_30_09_26.backup [--analyze]
#
# Notes
#  - The dump is PostgreSQL custom-archive format v1.16 (pg_dump 17.x). The
#    postgres:16 image's pg_restore REFUSES v1.16 ("unsupported version").
#    We therefore stream the dump through the HOST pg_restore (>=17) into the
#    container's psql, which avoids needing a 17 image locally.
#  - The sandbox listens on 127.0.0.1:5434 to avoid clashing with a local 5432/5433.
set -euo pipefail

DUMP="${1:?usage: restore.sh <dump-file> [--analyze]}"
[[ -f $DUMP ]] || { echo "no such file: $DUMP" >&2; exit 1; }

CONTAINER=hisrm_sandbox
PORT=5434
DB=hisrm_muster

command -v docker >/dev/null || { echo "docker required" >&2; exit 1; }
command -v pg_restore >/dev/null || { echo "host pg_restore (>=17) required" >&2; exit 1; }

ver="$(pg_restore --version | grep -oE '[0-9]+' | head -1)"
if (( ver < 17 )); then
  echo "host pg_restore is v$ver; need >=17 to read archive v1.16" >&2
  exit 1
fi

echo "==> starting sandbox $CONTAINER on 127.0.0.1:$PORT"
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER" \
  -e POSTGRES_PASSWORD=hisrm -e POSTGRES_USER=postgres \
  -p "127.0.0.1:$PORT:5432" postgres:16-alpine >/dev/null

echo "==> waiting for readiness"
for _ in {1..30}; do
  if docker exec "$CONTAINER" pg_isready -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$CONTAINER" pg_isready -U postgres >/dev/null

echo "==> creating database $DB"
docker exec "$CONTAINER" psql -U postgres -qc "DROP DATABASE IF EXISTS $DB;"
docker exec "$CONTAINER" psql -U postgres -qc \
  "CREATE DATABASE $DB ENCODING 'UTF8' LC_COLLATE 'C' LC_CTYPE 'C' TEMPLATE template0;"

echo "==> restoring (host pg_restore v$ver -> container psql)"
pg_restore --no-owner --no-acl -f - "$DUMP" \
  | docker exec -i "$CONTAINER" psql -U postgres -d "$DB" -q \
      > /tmp/hisrm_restore.out 2> /tmp/hisrm_restore.err || true

echo "==> done. tables:"
docker exec "$CONTAINER" psql -U postgres -d "$DB" -tAc \
  "select count(*) from information_schema.tables
   where table_schema not in ('pg_catalog','information_schema');"
echo "    (non-fatal errors, if any, are in /tmp/hisrm_restore.err)"

if [[ ${2:-} == --analyze ]]; then
  exec "$(dirname "$0")/analyze.sh"
fi
