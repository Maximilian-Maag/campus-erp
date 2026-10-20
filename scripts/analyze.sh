#!/bin/bash
# Regenerate every artefact under schema/ from the restored sandbox database.
# Requires the sandbox from scripts/restore.sh to be running.
set -euo pipefail

CONTAINER=hisrm_sandbox
DB=hisrm_muster
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/schema"
mkdir -p "$OUT"

psql() { docker exec "$CONTAINER" psql -U postgres -d "$DB" "$@"; }

echo "==> checking sandbox"
docker exec "$CONTAINER" pg_isready -U postgres >/dev/null || {
  echo "sandbox $CONTAINER not ready - run scripts/restore.sh first" >&2; exit 1; }

echo "==> schema_full.sql (DDL)"
docker exec "$CONTAINER" pg_dump -U postgres -d "$DB" --schema-only --no-owner --no-acl \
  > "$OUT/schema_full.sql"

echo "==> inventory.tsv"
psql -tAF$'\t' -c "
  select n.nspname, c.relname, coalesce(d.description,'')
  from pg_class c join pg_namespace n on n.oid=c.relnamespace
  left join pg_description d on d.objoid=c.oid and d.objsubid=0
  where c.relkind='r' and n.nspname not in ('pg_catalog','information_schema')
  order by 1,2;" > "$OUT/inventory.tsv"

echo "==> columns.tsv"
psql -tAF$'\t' -c "
  select table_schema, table_name, ordinal_position, column_name, data_type,
         coalesce(character_maximum_length::text,''), is_nullable
  from information_schema.columns
  where table_schema not in ('pg_catalog','information_schema')
  order by 1,2,3;" > "$OUT/columns.tsv"

echo "==> pks.tsv"
psql -tAF$'\t' -c "
  select ns.nspname, cl.relname, string_agg(a.attname,',' order by k.ord)
  from pg_constraint c
  join pg_class cl on cl.oid=c.conrelid
  join pg_namespace ns on ns.oid=cl.relnamespace
  join unnest(c.conkey) with ordinality k(attnum,ord) on true
  join pg_attribute a on a.attrelid=c.conrelid and a.attnum=k.attnum
  where c.contype='p' group by 1,2 order by 1,2;" > "$OUT/pks.tsv"

echo "==> fks.tsv (expected empty - the schema declares no foreign keys)"
psql -tAF$'\t' -c "
  select ns.nspname, cl.relname, a.attname, pns.nspname, pcl.relname, pa.attname
  from pg_constraint c
  join pg_class cl on cl.oid=c.conrelid join pg_namespace ns on ns.oid=cl.relnamespace
  join pg_class pcl on pcl.oid=c.confrelid join pg_namespace pns on pns.oid=pcl.relnamespace
  join unnest(c.conkey) with ordinality k(attnum,ord) on true
  join pg_attribute a on a.attrelid=c.conrelid and a.attnum=k.attnum
  join unnest(c.confkey) with ordinality kp(attnum,ord) on kp.ord=k.ord
  join pg_attribute pa on pa.attrelid=c.confrelid and pa.attnum=kp.attnum
  where c.contype='f' order by 1,2;" > "$OUT/fks.tsv"

echo "==> per-schema table lists"
for s in sva4 mbs cob kbs hisrm; do
  awk -F'\t' -v s="$s" '$1==s{print $2}' "$OUT/inventory.tsv" > "$OUT/tables_$s.txt"
done

echo "==> done:"
wc -l "$OUT"/*.tsv | tail -1
