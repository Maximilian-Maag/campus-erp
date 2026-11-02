# 0012 — Per-state statistics formats as data

Status: Accepted (2026-11-02). Supersedes nothing. Related: 0010 (reports are definitions
plus rows), 0011 (artefacts in an object store), `docs/27`.

## Context

The statistics offices are requirement IR-4 / FR-P9. Every federal state wants the period's
figures in its own file — its own column set, order, headings and field separator — and each
office changes its layout on its own schedule. The offices' layouts are not published for a
replacement build (`open-questions.md` Q8), so they cannot be transcribed; they can only be
approximated now and corrected when obtained.

The volatile part of this interface is a *column list*. Anything that puts a column list behind
a code change, a test run and a deploy makes every office's formatting quirk a software change.

## Decision

The per-state layout is **data**, not code.

- A **`Format`** (state key, name, delimiter, ordered `[]Column`, optional caption, optional
  totals row) describes one state's file.
- A **`Registry`** (`statistics.Standard()`) holds one `Format` per state. Adding a state is a
  new entry; changing one is editing its entry.
- The rendering pipeline (`Group` + `Render`) is written once against the *set of canonical
  fields* and is format-agnostic: it projects records through whatever columns a format names.
- The **only code surface** is the field set: a value a state wants that does not exist yet is
  a new `Field` constant and a `value()` case. Layouts themselves are never code.

Formats are validated on construction (state, name, permitted delimiter, non-empty columns,
known fields, unique fields), so a malformed layout fails at startup rather than producing a
wrong file. The job kind (`statistics-export`) carries the state key as its argument and is
otherwise identical in shape to the other export kinds (a func-seam worker that validates and
hands the I/O to the role).

## Consequences

- A new or changed state layout is a one-entry edit with no pipeline change; the copy in the
  registry is the single source of truth for what a state's file looks like.
- The registry's seeded layouts are **project-defined placeholders**, named `(Vorlage)` and
  marked as such. This is the honest state: only the mechanism is real today, not the offices'
  formats. Obtaining a real layout replaces its placeholder.
- A layout that needs a genuinely new datum is still a small code change (one field), which is
  the right boundary: the *shape of the data* is code, the *arrangement of it* is data.
- Because the format carries its delimiter and column order, a layout can be exercised and
  mutation-tested without any state's real spec — which is what makes the placeholders useful
  rather than decorative.

## Alternatives

- **One Go package (or one function) per state.** Rejected: turns every column reorder into a
  deploy, and multiplies the code to review for zero behavioural difference.
- **A configuration file (YAML/JSON) per state.** Rejected as the primary mechanism: it moves
  validation and the "known field" set out of the type system and into a parser, and the
  repository's other formats live in Go. The registry is the same idea with compile-time
  checking of the field references. A file-based overlay remains a possible future extension
  if operators must edit layouts without a rebuild.
- **A single "lowest common denominator" format for all states.** Rejected: the requirement is
  that each office gets *its* file; a common format satisfies none of them.
