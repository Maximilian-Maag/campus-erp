# 16 — HTTP transport layer: dependencies, middleware, error envelope

`docs/15` made the API *contract* real. This note covers the transport layer
that serves it: how the server is wired, what every request passes through, and
the one error shape the contract now promises.

## 1. Dependencies are a struct, not thirteen positional arguments

`httpapi.New` took thirteen positional service interfaces. Adding a module meant
editing the signature and every call site, and a test had to pass ten stubs it
never used. It now takes a `Deps` struct (`backend/internal/httpapi/server.go`):

```go
type Deps struct {
	Kasse           cashBook
	Orders          orderService
	// … one field per collaborator …
	Audit           auditVerifier
}

func New(d Deps) *Server { … }
```

A test supplies only the collaborators it exercises; the rest are nil interfaces
that panic only if that route is hit. Call sites (`cmd/api`, the two test files)
name their dependencies instead of matching a position.

## 2. Middleware: correlation id, recovery, access log

`internal/httpapi/middleware.go` wraps the router once:

- **Correlation id** — an incoming `X-Request-ID` is kept, otherwise one is
  generated; it is echoed on the response and carried in the request context
  (`httpapi.RequestID`). The client, the access log and (later) the audit event
  share one id (NFR-5).
- **Panic recovery** — a panic becomes a `500` with the JSON error envelope and
  is logged; one bad request cannot take the process down.
- **Structured access log** — method, path, status, duration, request id via
  `log/slog`, so ops logs stay structured and separate from the audit stream
  (NFR-5).

Handlers stay thin: they decode, delegate and map errors.

## 3. One error envelope

Every handler used to write `http.Error(...)` — a plain-text body. Now they all
write the shared JSON envelope defined in the contract
(`components/schemas/Problem`):

```json
{ "error": "kasse: day is closed", "code": "unprocessable", "request_id": "a1b2…" }
```

`writeProblem(w, status, msg)` is the single writer; the per-module
`statusFor…` mappers still decide the status code (they encode the domain
meaning), but the *body* is uniform, so the generated client and every future
consumer can parse errors the same way.

- **`code`** is a stable machine code derived from the status (`not_found`,
  `unprocessable`, …), so a client branches on it instead of matching the message text.
  Where a client should branch on something the status cannot express, the handler passes
  an explicit code through `writeProblemCode` (e.g. `invalid_artefact`,
  `artefact_not_found`, `state_required`, `export_queue_unconfigured`).
- **`request_id`** is the same correlation id the response carries in `X-Request-ID`, so an
  error report maps to a log line and an audit event (NFR-5). It rides on the response
  writer the middleware installs, which is why no handler has to thread the request
  through to write an error.

## 4. What this changes in the contract

Nothing in the request/response *shapes* — but the spec is now also our error
contract: `Problem` is referenced by every operation's `400/404/409/422/500`
responses. Nullable Go fields (`*int64`) are emitted with `x-nullable` and the
generated TypeScript is `number | null`, so the client no longer lies about a
field that can be absent-as-null.

## 5. Verified

- Go: `go vet`, `go build`, `go test ./...`; gremlins `internal/httpapi` 100 %.
- Web: `ng build`, `ng test` (23 specs), stryker 100 %.
- Contract: `tools/api_contract.py check` (51 ↔ 51), redocly lint (valid).

## Not yet done

- ~~Auth is still the `X-Actor` header~~ — done since: OIDC + the coarse role model
  (ADR-0003) shipped (docs/17), and the authenticated subject is the audit `actor`,
  qualified with the resolved tenant.
- ~~The error envelope has no stable machine `code`~~ — done since: the envelope carries a
  status-derived code, and `writeProblemCode` sets a domain code where a client branches on
  it. What remains is a *systematic* mapping from domain errors to codes rather than the
  per-call-site ones.
