#!/bin/bash
# Regenerate every artefact under schema/ from the restored sandbox database.
#
# schema/ is git-ignored on purpose: these artefacts are derived from a proprietary
# third-party database dump, so they are produced locally and never committed.
# Requires the sandbox from scripts/restore.sh to be running.
#
# All outputs are written atomically (temp file + rename) so a failed command
# never leaves a truncated artefact behind.
set -euo pipefail

CONTAINER=hisrm_sandbox
DB=hisrm_muster
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/schema"
mkdir -p "$OUT"

psql() { docker exec "$CONTAINER" psql -U postgres -d "$DB" "$@"; }

# write <target> <cmd...> — replace <target> only if <cmd...> succeeds
write() {
  local target=$1
  shift
  "$@" > "$target.tmp" && mv "$target.tmp" "$target" || { rm -f "$target.tmp"; return 1; }
}

echo "==> checking sandbox"
docker exec "$CONTAINER" pg_isready -U postgres >/dev/null || {
  echo "sandbox $CONTAINER not ready - run scripts/restore.sh first" >&2; exit 1; }

echo "==> schema_full.sql (DDL)"
write "$OUT/schema_full.sql" \
  docker exec "$CONTAINER" pg_dump -U postgres -d "$DB" --schema-only --no-owner --no-acl
# pg_dump 16.15+ injects a random \restrict/\unrestrict nonce (an injection
# guard); strip it so the artefact is byte-reproducible run to run.
grep -vE '^\\(un)?restrict ' "$OUT/schema_full.sql" > "$OUT/schema_full.sql.tmp" \
  && mv "$OUT/schema_full.sql.tmp" "$OUT/schema_full.sql"

echo "==> inventory.tsv"
write "$OUT/inventory.tsv" psql -tAF$'\t' -c "
  select n.nspname, c.relname, coalesce(d.description,'')
  from pg_class c join pg_namespace n on n.oid=c.relnamespace
  left join pg_description d on d.objoid=c.oid and d.objsubid=0
  where c.relkind='r' and n.nspname not in ('pg_catalog','information_schema')
  order by 1,2;"

echo "==> columns.tsv"
write "$OUT/columns.tsv" psql -tAF$'\t' -c "
  select table_schema, table_name, ordinal_position, column_name, data_type,
         coalesce(character_maximum_length::text,''), is_nullable
  from information_schema.columns
  where table_schema not in ('pg_catalog','information_schema')
  order by 1,2,3;"

echo "==> pks.tsv"
write "$OUT/pks.tsv" psql -tAF$'\t' -c "
  select ns.nspname, cl.relname, string_agg(a.attname,',' order by k.ord)
  from pg_constraint c
  join pg_class cl on cl.oid=c.conrelid
  join pg_namespace ns on ns.oid=cl.relnamespace
  join unnest(c.conkey) with ordinality k(attnum,ord) on true
  join pg_attribute a on a.attrelid=c.conrelid and a.attnum=k.attnum
  where c.contype='p' group by 1,2 order by 1,2;"

echo "==> fks.tsv (expected empty - the schema declares no foreign keys)"
write "$OUT/fks.tsv" psql -tAF$'\t' -c "
  select ns.nspname, cl.relname, a.attname, pns.nspname, pcl.relname, pa.attname
  from pg_constraint c
  join pg_class cl on cl.oid=c.conrelid join pg_namespace ns on ns.oid=cl.relnamespace
  join pg_class pcl on pcl.oid=c.confrelid join pg_namespace pns on pns.oid=pcl.relnamespace
  join unnest(c.conkey) with ordinality k(attnum,ord) on true
  join pg_attribute a on a.attrelid=c.conrelid and a.attnum=k.attnum
  join unnest(c.confkey) with ordinality kp(attnum,ord) on kp.ord=k.ord
  join pg_attribute pa on pa.attrelid=c.confrelid and pa.attnum=kp.attnum
  where c.contype='f' order by 1,2;"

echo "==> per-schema table lists"
for s in sva4 mbs cob kbs hisrm; do
  awk -F'\t' -v s="$s" '$1==s{print $2}' "$OUT/inventory.tsv" > "$OUT/tables_$s.txt"
done

echo "==> done: $(wc -l < "$OUT/inventory.tsv") tables, $(wc -l < "$OUT/columns.tsv") columns"
