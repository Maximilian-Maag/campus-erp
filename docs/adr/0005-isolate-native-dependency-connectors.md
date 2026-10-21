# ADR-0005 — Isolate native-dependency integrations in a separate connectors service

- Status: Accepted
- Date: 2026-10-20

## Context

Several required integrations depend on **native (non-Go) code**:
- ELSTER via the **ERiC** C library (`mbs.elster_webserver` in the dump).
- **EBICS** banking clients, which typically need native crypto/ISO 20022 tooling.
The rest of the backend is intended to be pure Go with a clean, portable image.

Mixing cgo and native libraries into the main API/worker image complicates builds,
reproducibility and supply-chain review — and these integrations are exactly the
ones with the highest compliance weight.

## Decision

Put all native-dependency integrations in a **separate `connectors` service**
(same Go codebase, separate image with the native deps and a separate k8s
Deployment). The core `api`/`worker` stay pure Go.

`connectors` owns: ERiC/ELSTER, EBICS/bank transport, and any other native-dep
adapter. It communicates over an internal contract (queue + object storage), so the
core never links the native code.

## Consequences

- The core image stays small, portable and easy to build/reproduce.
- `connectors` is a distinct, hardened, separately-scaled deployment — appropriate
  for systems touching money and tax.
- An internal contract (message schema + storage layout) must be defined and
  versioned; it is a real interface to maintain.
- Local development needs the connectors image only when working on those paths.

## Alternatives considered

- **cgo in the main binary.** Simplest to write, worst to build/deploy/review.
  Rejected.
- **Shell out to CLI tools.** Fragile and hard to test; rejected in favour of a
  dedicated service with a testable contract.
