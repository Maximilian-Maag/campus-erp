# 33 — Signing in to the interface

The interface has no password of its own (ADR-0003). A clerk signs in with the university
account, the IdP issues a token, and the API decides from that token's roles what the person
may do. This document describes the client half: what the browser does, what it stores, and
what a deployment has to configure.

## The flow

Authorization Code with PKCE (RFC 7636), because the interface is a public client: it cannot
keep a secret, so the code it receives is bound to a verifier that only that tab ever saw.

1. A screen behind the login is opened without a session → `authGuard` redirects to
   `/login?returnUrl=<where they were going>`.
2. **Anmelden** asks the IdP for its discovery document (`<issuer>/.well-known/openid-configuration`),
   generates a state and a verifier, remembers both in `sessionStorage`, and navigates to the
   authorize endpoint with `code_challenge_method=S256`.
3. The IdP sends the browser back to `/auth/callback?code=…&state=…`.
4. The callback consumes the pending sign-in — a state that does not match is refused, and the
   pending value is deleted on first use, so a replayed callback finds nothing — and exchanges
   the code for a token.
5. The session is written to `sessionStorage` and the user lands on the screen they asked for.

`sessionStorage`, not `localStorage`: two tabs are two sessions, closing the tab ends it, and a
shared machine does not keep the last clerk's token for the next person.

## Sending the token

`authInterceptor` refreshes a token that is within sixty seconds of expiry before it uses it,
then adds `Authorization: Bearer …` to every request under `/api`. A 401 on a request that
carried a token means the token is no longer accepted, whatever it claims — the session is
forgotten and the user is sent back to the login screen with `returnUrl` set.

The interface only decodes the token to greet the user and to know when to refresh. It does
not verify the signature and never decides what is allowed: that is the API's role table.

## A deployment without an issuer

An empty `ERP_OIDC_ISSUER` writes a `config.json` with sign-in off. Every screen then works
without a session, because the API is open — which is how `make run` starts, and what the
compose stack defaults to for local work. The guard and the interceptor are inert in that mode
rather than pretending: with an issuer configured and no session, they redirect.

This is a *development* mode, not a deployment mode: it makes the API answer without a token,
so it is only safe where nobody else can reach it. `make run-idp` and `make dev-setup` start
the stack behind the local IdP for that reason, and the Helm chart is where a deployment
declares its issuer (`config.oidc`).

## The end-to-end suite runs signed in

`frontend/e2e/session.ts` is the only `test` the specs import: an automatic fixture signs the
tab in before the spec body, by walking the real round trip — the guard's redirect, the login
screen's one action, the IdP's answer, the callback's exchange — against an IdP stubbed at the
network edge. It does not seed `sessionStorage`: seeding tests that the application *reads* a
session, the round trip tests that it *obtains* one, and every screen then renders for a
signed-in clerk with a bearer token on each request.

`auth.e2e.spec.ts` opts out and drives the states that exist without a session (the redirect,
the callback, sign-out) on its own, including the one assertion about a deployment without an
issuer.

## Signing in locally

`make run-idp` starts the same stack plus a Keycloak realm
(`deploy/local/keycloak/realm.json`, compose profile `idp`) and configures both halves
against it, so the login screen is live and the whole round trip can be walked:

```
make run-idp        # http://localhost:8081, Keycloak on http://localhost:9080
```

Sign in as **max.maag / kasse**. The realm declares the six `erp-*` roles and gives that user
all of them, so every screen is reachable; it also declares the public client `campus-erp`
with `S256` PKCE, `http://localhost:8081/*` (and `:4200` for `ng serve`) as the redirect
address, and the two mappers that matter to the API:

- **realm roles → `roles`** — the API reads `roles`, Keycloak's own claim is
  `realm_access.roles`, so without this mapper every request is authenticated and forbidden.
- **audience `campus-erp`** — so `ERP_OIDC_AUDIENCE` can be, and is, set: a token minted for
  another client is refused.

Two addresses, on purpose: the browser reaches the issuer at `http://localhost:9080/...`, and
the token's `iss` is that value, so `ERP_OIDC_ISSUER` must be it — but the API fetches the
signing keys from inside the compose network (`ERP_OIDC_JWKS_URL=http://keycloak:8080/...`).
Setting the four `ERP_OIDC_*` values in `.env` instead of passing them makes a plain
`make run` sit behind the login as well.

The realm is imported from the file on every start; `make stop` also stops the IdP
(`--profile idp`). Keycloak's `start-dev` keeps an in-memory database and serves plain HTTP —
this is a development fixture, not a deployment.

## Configuration

The image is built once and deployed many times, so the values are read at container start by
`frontend/docker/auth-config.sh`, which writes `/usr/share/nginx/html/config.json` (served
with `no-store`):

| Variable | Meaning |
| --- | --- |
| `ERP_OIDC_ISSUER` | OIDC issuer; endpoints come from its discovery document |
| `ERP_OIDC_CLIENT_ID` | public client registered at the IdP |
| `ERP_OIDC_SCOPE` | defaults to `openid profile email` |
| `ERP_OIDC_REDIRECT_URI` | empty means `<origin>/auth/callback` |

The IdP has to allow that redirect address for the client, and the token endpoint has to
answer the browser with CORS headers (it is called from the page, not from the server).

## What this deliberately does not do

- **Hide controls by role.** The role table lives in the API; a hidden button would be
  decoration, and the API answers 403 where it matters. Role-aware navigation is a separate
  decision about the product, not a security measure.
- **Log out of the IdP.** Signing out of Campus ERP ends this product's session only; ending
  the university session is the IdP's business.
- **Silent single sign-on.** If the IdP session is still alive, the next sign-in is a redirect
  that completes without a password — but it is still a redirect.
