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
| Statistics export (`cmd/worker`) | `statistics/<state>-<from>-<to>.csv` |

`documents.Store` is the one-method subset the document workflow needs, so a
`storage.Store` is passed to it directly.

## Reaching what is stored

`GET /api/artefacts/{kind}/{name}` serves stored artefacts (role `kasse`); the kinds are
`dunning`, `reports`, `ledger`, `sepa` and `statistics` (docs/24). The route is an allowlist, not a
passthrough to the store: a key outside those kinds is a 400, whatever the store holds.

## Verification status

- Directory store and key handling: unit-tested.
- **S3 store: verified end to end, both against a synthetic server and a real one.**
  `storage.TestS3RoundTrip` runs on every build against an in-process S3-compatible server
  (gofakes3): bucket creation (idempotent), `Save` and `Get` round trips including nested
  keys, a missing key failing, and an escaping key being refused. It has also been run
  against **RustFS**, the S3-compatible endpoint the local stack ships
  (`docker compose --profile s3 up rustfs`), and passes.
- Point the same test at any endpoint with `ERP_S3_ENDPOINT`/`ERP_S3_TEST_ENDPOINT` plus
  `ERP_S3_BUCKET`, `ERP_S3_ACCESS_KEY`, `ERP_S3_SECRET_KEY`. The client is minio-go; the
  server can be RustFS, a university S3, or anything else that speaks S3.
