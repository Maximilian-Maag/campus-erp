import { test as base, expect, type Page, type Route } from '@playwright/test';

/**
 * The end-to-end suite runs **signed in**.
 *
 * Every screen is behind `authGuard` the moment a deployment names an issuer, so a suite that
 * runs with sign-in off is testing a shape of the product nobody deploys: a visitor with no
 * session reaching the cashbook. `test` exported here extends Playwright's so that each spec
 * starts with a session, and it gets there the way a clerk does — the guard redirects, the
 * login screen's one action leaves for the IdP, the callback exchanges the code — rather than
 * by seeding `sessionStorage`. Seeding would test that the app *reads* a session; the round
 * trip tests that it *obtains* one.
 *
 * The IdP is stubbed at the network edge: the discovery document, the authorize endpoint
 * (which bounces straight back with a code, the way a signed-in user's does) and the token
 * endpoint. What is under test stays the interface: the guard, the PKCE exchange, the
 * interceptor that attaches the token, and the screens that then render.
 *
 * The API is stubbed too, with an empty answer, because a spec that has not stubbed it yet
 * still has to render — and, more importantly, a request that reached a real API would come
 * back 401 and the interceptor would (correctly) throw the session away half-way through the
 * fixture. A spec that registers its own route for the API does so later, and the later
 * registration wins.
 */

export const ISSUER = 'https://idp.uni.ac.at/realms/campus';
export const TOKEN_ENDPOINT = `${ISSUER}/token`;

function b64url(value: object): string {
  return Buffer.from(JSON.stringify(value)).toString('base64url');
}

/** A token with the claims the interface greets by; the API would verify the signature. */
export const ACCESS_TOKEN = [
  b64url({ alg: 'RS256', typ: 'JWT' }),
  b64url({ sub: 'max.maag', name: 'Max Maag', email: 'max.maag@uni.ac.at', roles: ['erp-admin'] }),
  'signature',
].join('.');

/** Everything a cross-origin answer needs for the browser to hand it to the page. */
const CORS = { 'access-control-allow-origin': '*' };

/** The deployment's sign-in configuration and the IdP behind it, as the network sees them. */
export async function stubIdp(
  page: Page,
  config: object = { issuer: ISSUER, clientId: 'campus-erp' },
): Promise<void> {
  await page.route('**/config.json', (route: Route) => route.fulfill({ json: { auth: config } }));
  await page.route(`${ISSUER}/.well-known/openid-configuration`, (route: Route) =>
    route.fulfill({
      json: { authorization_endpoint: `${ISSUER}/auth`, token_endpoint: TOKEN_ENDPOINT },
      headers: CORS,
    }),
  );
  // The user has already signed in at the university: the authorize endpoint answers at once,
  // with the state the interface sent it.
  await page.route(`${ISSUER}/auth**`, (route: Route) => {
    const state = new URL(route.request().url()).searchParams.get('state') ?? '';
    // Absolute, or the browser resolves it against the IdP: the answer has to land on the
    // application's own origin.
    const origin = new URL(page.url()).origin;
    return route.fulfill({
      status: 302,
      headers: { location: `${origin}/auth/callback?code=code-1&state=${state}` },
    });
  });
  await page.route(TOKEN_ENDPOINT, (route: Route) =>
    route.fulfill({ json: { access_token: ACCESS_TOKEN, expires_in: 300 }, headers: CORS }),
  );
}

/** Walks the sign-in round trip and leaves the tab signed in on `/kasse/tag`. */
export async function signIn(page: Page): Promise<void> {
  await page.route('**/api/**', (route: Route) => route.fulfill({ json: [] }));
  await stubIdp(page);
  await page.goto('/login');
  await page.getByTestId('sign-in').click();
  await expect(page.getByTestId('principal')).toBeVisible();
}

/** `test`, with the tab already signed in when the spec body starts. */
export const test = base.extend<{ signedIn: void }>({
  signedIn: [
    async ({ page }, use) => {
      await signIn(page);
      await use();
    },
    { auto: true },
  ],
});

export { expect } from '@playwright/test';
