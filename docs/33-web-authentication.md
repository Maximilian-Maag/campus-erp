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

## There is no unauthenticated mode

The API is a web application on a host, so it does not run without a way to identify its
caller: `config.Load` refuses the `api` role unless `ERP_OIDC_ISSUER` and `ERP_OIDC_JWKS_URL`
are set, and the process exits before it opens a listener. A deployment that forgets them fails
at start rather than answering `/api/*` to anyone who can reach it, and the Helm chart refuses
to render without them for the same reason.

The interface still degrades when a `config.json` carries no issuer — it drops the guard and
the interceptor instead of pretending to be signed in — because a misconfigured web container
must not blank the screen. With no api to talk to that is an error state, not a mode, and
`auth.e2e.spec.ts` keeps a test for it so the fallback stays tested.

In development the stack ships its own IdP, so there is nothing to switch on: `make run` starts
Keycloak, the api and the interface against the same realm.

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

`make run` (and `make dev-setup`) starts the stack with a Keycloak realm imported from
`deploy/local/keycloak/realm.json`, and configures both halves against it, so the login screen
is live and the whole round trip can be walked:

```
make run            # http://localhost:8081, Keycloak on http://localhost:9080
```

Sign in as **max.maag / kasse** — the clerk with all six `erp-*` roles, so every screen is
reachable. **leser / lesen** is the second account: `erp-reader` only, which reads every module
and is refused (403) on any write. The pair exists so the role gate can be seen working rather
than described: `GET /api/payment-orders` answers 200 for both, `POST` answers 403 for the
reader. The realm also declares the public client `campus-erp`
with `S256` PKCE, `http://localhost:8081/*` (and `:4200` for `ng serve`) as the redirect
address, and the two mappers that matter to the API:

- **realm roles → `roles`** — the API reads `roles`, Keycloak's own claim is
  `realm_access.roles`, so without this mapper every request is authenticated and forbidden.
- **audience `campus-erp`** — so `ERP_OIDC_AUDIENCE` can be, and is, set: a token minted for
  another client is refused.

Two addresses, on purpose: the browser reaches the issuer at `http://localhost:9080/...`, and
the token's `iss` is that value, so `ERP_OIDC_ISSUER` must be it — but the API fetches the
signing keys from inside the compose network (`ERP_OIDC_JWKS_URL=http://keycloak:8080/...`).
Both defaults live in `docker-compose.yml`; `.env` overrides them for another IdP.

The realm is imported from the file on every start, and `make stop` stops the IdP with the rest
of the stack. Keycloak's `start-dev` keeps an in-memory database and serves plain HTTP — this is
a development fixture. A deployment brings its own IdP and its own `config.oidc` values.

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
