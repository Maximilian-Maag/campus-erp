# 26 — Storage

Artefacts — documents, exports — go through one keyed store (ADR-0011).

## Shape

```go
storage.Store{ Save(ctx, key string, content []byte) error ; Get(ctx, key string) ([]byte, error) }
```

- **`storage.Dir`** — a directory; used locally and in tests.
- **`storage.S3`** — an S3-compatible bucket (minio-go); `EnsureBucket` is idempotent, so
  a role can call it on every start.
- **`storage.FromEnv(ctx, root)`** — `ERP_S3_ENDPOINT` set → S3 (`ERP_S3_BUCKET`,
  `ERP_S3_ACCESS_KEY`, `ERP_S3_SECRET_KEY`); otherwise a directory at `root`.

Keys are checked once (`CheckKey`): empty, absolute, and escaping keys are refused.

## Who writes what

| Writer | Key |
|---|---|
| Dunning notices (`cmd/api`) | `dunning/<notice id>.pdf` |
| Ledger export (`cmd/worker`) | `ledger/ledger-postings-<timestamp>.csv` |
| SEPA export (`cmd/worker`) | `sepa/sepa-<run id>.xml` |

`documents.Store` is the one-method subset the document workflow needs, so a
`storage.Store` is passed to it directly.

## Verification status

- Directory store and key handling: unit-tested.
- **S3 store: not verified end to end here.** `storage.TestS3RoundTrip` round-trips a
  value when `ERP_S3_TEST_ENDPOINT` is set; the MinIO image could not be pulled in this
  environment, so that test has not been run. Treat the S3 path as written-but-unproven
  until it is run against a real endpoint.
