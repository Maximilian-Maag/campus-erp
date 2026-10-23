#!/bin/bash
# Render the C4 model (c4/workspace.dsl) to a single PDF.
#
# Pipeline:
#   DSL  --(Structurizr CLI, docker)-->  PlantUML  --(PlantUML, docker)-->  PNG
#   PNG + c4/pdf/template.tex  --(tectonic)-->  c4/export/campus-erp-c4-model.pdf
#
# Why PlantUML and not the CLI's own PNG export: the structurizr/structurizr
# image cannot render images locally ("Exporting to PNG/SVG is not supported in
# this build" — that path needs the hosted API via --url). PlantUML is
# self-contained here: the export has no `!include`, so it renders offline.
#
# Nothing produced here is committed (c4/export/ is git-ignored); the sources are
# c4/workspace.dsl and c4/pdf/template.tex.
#
# Usage:
#   tools/c4_pdf.sh                # export + render + PDF
#   tools/c4_pdf.sh --export-only  # only refresh c4/export/*.png
#   tools/c4_pdf.sh --no-export    # reuse existing PNGs, only build the PDF
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
C4="$ROOT/c4"
EXPORT="$C4/export"
TEMPLATE="$C4/pdf/template.tex"
STRUCTURIZR_IMAGE="${STRUCTURIZR_IMAGE:-structurizr/structurizr:latest}"
PLANTUML_IMAGE="${PLANTUML_IMAGE:-plantuml/plantuml:latest}"
OUT_NAME="campus-erp-c4-model"

do_export=1
do_pdf=1
case "${1:-}" in
  --export-only) do_pdf=0 ;;
  --no-export)   do_export=0 ;;
  "")            ;;
  *) echo "usage: $0 [--export-only|--no-export]" >&2; exit 2 ;;
esac

say() { printf '%s\n' "$*"; }
die() { say "error: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

[[ -f $C4/workspace.dsl ]] || die "missing $C4/workspace.dsl"
[[ -f $TEMPLATE ]] || die "missing $TEMPLATE"

# The Structurizr container reads the DSL as a non-root user; a 600 file makes the
# parser report only the bare filename. Keep it world-readable.
chmod 644 "$C4/workspace.dsl"
mkdir -p "$EXPORT"

# uid/gid mapping so files land owned by the invoking user, not root.
as_user=(--user "$(id -u):$(id -g)")

# ── 1. DSL -> PlantUML ──────────────────────────────────────────────────────
if (( do_export )); then
  have docker || die "docker required to export diagrams (or pass --no-export)"
  say "== exporting views to PlantUML (Structurizr) =="
  docker run --rm "${as_user[@]}" \
    -v "$C4":/w -w /w -v "$EXPORT":/out \
    "$STRUCTURIZR_IMAGE" export -w workspace.dsl -f plantuml -o /out \
    || die "structurizr export failed"

  say "== rendering PlantUML -> PNG (PlantUML) =="
  shopt -s nullglob
  for puml in "$EXPORT"/structurizr-*.puml; do
    [[ $puml == *-key.puml ]] && continue      # skip the standalone legend
    docker run --rm "${as_user[@]}" -v "$EXPORT":/out \
      "$PLANTUML_IMAGE" -tpng -o /out "/out/$(basename "$puml")" \
      || die "plantuml render failed for $(basename "$puml")"
  done
fi

(( do_pdf )) || { say "ok: views refreshed (PDF skipped)"; exit 0; }

mapfile -t pngs < <(find "$EXPORT" -maxdepth 1 -name 'structurizr-*.png' | sort)
(( ${#pngs[@]} )) || die "no PNG views in $EXPORT (run without --no-export)"

# ── 2. build the LaTeX figure fragment ──────────────────────────────────────
order=(AsIs-Context AsIs-Containers AsIs-SvaComponents \
       Target-Context Target-Containers Target-ApiComponents)

tex_escape() { printf '%s' "$1" | sed -e 's/\\/\\textbackslash{}/g' -e 's/[&%#_]/\\&/g'; }

nice_title() {
  case "$1" in
    AsIs-Context)         echo "As-is — System Context (level 1)";;
    AsIs-Containers)      echo "As-is — Containers (level 2)";;
    AsIs-SvaComponents)   echo "As-is — HISSVA Components (level 3)";;
    Target-Context)       echo "Target — System Context (level 1)";;
    Target-Containers)    echo "Target — Containers (level 2)";;
    Target-ApiComponents) echo "Target — API Components (level 3)";;
    *)                    printf '%s\n' "$1" | tr '_-' '  ';;
  esac
}

view_key() { local b; b="$(basename "$1" .png)"; echo "${b#structurizr-}"; }

declare -A seen=()
frag="$EXPORT/.figures.tex"
: > "$frag"

emit() { # <png path>
  local png="$1" key title
  key="$(view_key "$png")"
  seen["$key"]=1
  title="$(nice_title "$key")"
  {
    printf '\\clearpage\n'
    printf '\\section{%s}\n' "$(tex_escape "$title")"
    printf '\\begin{center}\n'
    printf '\\includegraphics[width=\\linewidth,height=0.82\\textheight,keepaspectratio]{%s}\n' "$(basename "$png")"
    printf '\\end{center}\n'
  } >> "$frag"
}

for key in "${order[@]}"; do
  for png in "${pngs[@]}"; do
    [[ -z ${seen[$(view_key "$png")]:-} ]] || continue
    [[ "$(view_key "$png")" == "$key" ]] && emit "$png"
  done
done
for png in "${pngs[@]}"; do
  [[ -z ${seen[$(view_key "$png")]:-} ]] || continue
  emit "$png"
done

# ── 3. materialise the report and compile it ────────────────────────────────
tex="$EXPORT/$OUT_NAME.tex"
sed -e "/@DIAGRAMS@/r $frag" -e "/@DIAGRAMS@/d" "$TEMPLATE" > "$tex"
rm -f "$frag"

have tectonic || die "tectonic required to build the PDF (https://tectonic-typesetting.github.io)"

say "== compiling PDF (tectonic) =="
( cd "$EXPORT" && tectonic --keep-logs --outdir "$EXPORT" "$tex" ) \
  || die "tectonic failed (see $EXPORT/${OUT_NAME}.log)"

[[ -f "$EXPORT/$OUT_NAME.pdf" ]] || die "PDF was not produced"
say "ok: $EXPORT/$OUT_NAME.pdf  (${#pngs[@]} diagram(s))"
