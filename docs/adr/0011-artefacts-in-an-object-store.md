# 0011 — Artefacts go through one keyed store

Status: accepted (2026-11-02)

## Context

Documents (dunning PDFs, reports) and exports (ledger postings, SEPA files) were written
to directories, each role choosing its own. That does not survive more than one replica,
and a container filesystem is not a place a bank file may live.

## Decision

One seam: `storage.Store` — `Save(ctx, key, content)` and `Get(ctx, key)`. Two
implementations: `storage.Dir` (local, tests, single-host) and `storage.S3` (an
S3-compatible bucket via minio-go). `storage.FromEnv` picks: `ERP_S3_ENDPOINT` set → S3
(the bucket is created if missing), otherwise a directory.

`documents.Store` is the one-method subset the document workflow needs, so a
`storage.Store` satisfies it without an adapter.

## Consequences

- Replicas can produce documents and exports: the artefact is not tied to a pod.
- One dependency (minio-go) instead of a client per role; it speaks the S3 protocol, so
  RustFS on-prem and a university S3 both work.
- Keys are validated in one place (`CheckKey`): empty, absolute and escaping keys are
  refused, so a caller cannot write outside the store.
- The S3 path is verified end to end by `storage.TestS3RoundTrip`: it serves an in-process
  S3-compatible server (gofakes3) so the path is exercised on every build and cannot rot,
  and it has also been run green against **RustFS**, the S3-compatible endpoint the local
  stack ships. `ERP_S3_TEST_ENDPOINT` points it at any other real endpoint.
- The endpoint is deliberately not tied to one vendor: RustFS is the default in the compose
  file, and a university S3 works unchanged. The client speaks the S3 protocol either way.
