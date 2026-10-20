# Changelog

All notable changes to this repository. Format: Keep a Changelog.

## [0.1.0] - 2026-10-20

### Added
- Initial reverse-engineering of the HIS-ERP Muster dump
  (`hisrm_muster_30_09_26.backup`).
- Module catalogue: HISinOne (campus) vs Hochschul-ERP, with module ids from
  `db_version` (`HISSVA-GX`, `HISMBS-UNIX`/`HISFSV-GX`, `HISCOB-GX`, `HISKBS`).
- Database overview: 1,536 tables / 374 views / 38,453 columns across 4 module
  schemas; framework layer; no foreign keys.
- Schema analysis by module and domain cluster.
- Glossary of German domain terms.
- C4 model (context, container, component, code).
- Requirements baseline and open-questions list.
- Reproducible tooling: `scripts/restore.sh`, `scripts/analyze.sh`.
- Generated artefacts under `schema/` (DDL, inventories, keys).
- Policy as code: `tools/policy_check.py`, `policy.json`, pre-commit hook, CI.
