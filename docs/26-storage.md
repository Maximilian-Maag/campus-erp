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
- **S3 store: verified end to end.** `storage.TestS3RoundTrip` runs on every build against
  an in-process S3-compatible server (gofakes3): bucket creation (idempotent), `Save` and
  `Get` round trips including nested keys, a missing key failing, and an escaping key being
  refused. Set `ERP_S3_TEST_ENDPOINT` (plus `ERP_S3_BUCKET`, `ERP_S3_ACCESS_KEY`,
  `ERP_S3_SECRET_KEY`) to run the same test against a real endpoint.
- MinIO's image cannot be pulled in this environment, so the endpoint under test is an
  S3-compatible server rather than MinIO itself; the client is minio-go and speaks the same
  protocol.
