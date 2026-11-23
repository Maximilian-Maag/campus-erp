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
without a session, because the API is open — which is how the local compose stack runs, and
what the end-to-end tests assume. The guard and the interceptor are inert in that mode rather
than pretending: with an issuer configured and no session, they redirect.

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