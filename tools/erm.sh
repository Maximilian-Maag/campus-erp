#!/bin/bash
# Render the ER (entity-relationship) models of the campus-erp databases to PNG.
#
#   c4/erm/*.puml  --(gen_erm.py)-->  PlantUML  --(PlantUML, docker)-->  c4/export/*.png
#
# The .puml sources are produced by tools/gen_erm.py:
#   * target_canonical — read straight from the SQL migrations (real FKs)
#   * legacy_<schema>  — sva4 / mbs / cob / kbs, reconstructed (no FKs exist)
#
# Usage:
#   tools/erm.sh               # generate .puml + render PNGs
#   tools/erm.sh --gen-only    # only (re)generate the .puml sources
#   tools/erm.sh --no-gen      # reuse existing .puml, only render PNGs
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ERM="$ROOT/c4/erm"
EXPORT="$ROOT/c4/export"
PLANTUML_IMAGE="${PLANTUML_IMAGE:-plantuml/plantuml:latest}"

do_gen=1
do_render=1
case "${1:-}" in
  --gen-only) do_render=0 ;;
  --no-gen)   do_gen=0 ;;
  "")         ;;
  *) echo "usage: $0 [--gen-only|--no-gen]" >&2; exit 2 ;;
esac

say() { printf '%s\n' "$*"; }
die() { say "error: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

mkdir -p "$EXPORT"

if (( do_gen )); then
  say "== generating ER models (tools/gen_erm.py) =="
  python3 "$ROOT/tools/gen_erm.py" --root "$ROOT" --out "$ERM" || die "gen_erm.py failed"
fi

(( do_render )) || { say "ok: ER sources regenerated (PNG render skipped)"; exit 0; }

shopt -s nullglob
pumls=("$ERM"/*.puml)
(( ${#pumls[@]} )) || die "no .puml files in $ERM"
have docker || die "docker required to render PlantUML"

say "== rendering ER PlantUML -> PNG (PlantUML) =="
for puml in "${pumls[@]}"; do
  say "   $(basename "$puml")"
  docker run --rm --user "$(id -u):$(id -g)" -v "$ERM":/in:ro -v "$EXPORT":/out \
    "$PLANTUML_IMAGE" -tpng -o /out "/in/$(basename "$puml")" \
    || die "plantuml render failed for $(basename "$puml")"
done

say "ok: ${#pumls[@]} ER diagram(s) in $EXPORT"
