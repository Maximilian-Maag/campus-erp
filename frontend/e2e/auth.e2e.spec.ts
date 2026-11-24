import { test, expect, type Page, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * The interface behind a login.
 *
 * The IdP is stubbed at the network edge: the discovery document, the authorize endpoint (which
 * bounces straight back with a code, the way a signed-in user's does) and the token endpoint.
 * What is under test is the interface — that it redirects, that it completes the exchange, that
 * it sends the bearer token to the API, and that it can be signed out of.
 */

const ISSUER = 'https://idp.uni.ac.at/realms/campus';
const TOKEN_ENDPOINT = `${ISSUER}/token`;

function b64url(value: object): string {
  return Buffer.from(JSON.stringify(value)).toString('base64url');
}

/** A token with the claims the interface greets by; the API would verify the signature. */
const ACCESS_TOKEN = [
  b64url({ alg: 'RS256', typ: 'JWT' }),
  b64url({ sub: 'max.maag', name: 'Max Maag', email: 'max.maag@uni.ac.at', roles: ['erp-kasse'] }),
  'signature',
].join('.');

/** Everything a cross-origin answer needs for the browser to hand it to the page. */
const CORS = { 'access-control-allow-origin': '*' };

async function stubIdp(page: Page, config: object = { issuer: ISSUER, clientId: 'campus-erp' }): Promise<void> {
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

test('a visitor without a session is sent to the login screen, and signing in brings them back', async ({
  page,
}) => {
  await stubIdp(page);
  const seen: Record<string, string | undefined> = {};
  await page.route('**/api/**', (route: Route) => {
    seen[new URL(route.request().url()).pathname] = route.request().headers()['authorization'];
    return route.fulfill({ json: [] });
  });

  // The screen they asked for is a screen behind the login.
  await page.goto('/kasse/anordnungen');
  await expect(page).toHaveURL(/\/login\?returnUrl=%2Fkasse%2Fanordnungen/);
  await expect(page.getByRole('heading', { name: 'Anmelden' })).toBeVisible();

  // One action, and it leaves for the university IdP.
  await page.getByTestId('sign-in').click();

  // Back on the screen they asked for, greeted by name.
  await expect(page).toHaveURL(/\/kasse\/anordnungen$/);
  await expect(page.getByTestId('principal')).toHaveText('Max Maag');

  // And the API call carried the token: that is the point of the whole round trip.
  await expect
    .poll(() => seen['/api/payment-orders'])
    .toBe(`Bearer ${ACCESS_TOKEN}`);
});

test('the login screen passes axe', async ({ page }) => {
  await stubIdp(page);
  await page.goto('/login');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('signing out forgets the session and asks again', async ({ page }) => {
  await stubIdp(page);
  await page.route('**/api/**', (route: Route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/anordnungen');
  await page.getByTestId('sign-in').click();
  await expect(page.getByTestId('principal')).toHaveText('Max Maag');

  await page.getByTestId('sign-out').click();

  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByTestId('principal')).toHaveCount(0);
  // A reload must not resurrect it: the session lives in the tab, and the tab forgot it.
  await page.reload();
  await expect(page.getByTestId('sign-in')).toBeVisible();
});

test('a deployment without an issuer runs without a login screen', async ({ page }) => {
  // The local stack and the tests above it: the API is open, so there is nothing to sign in to.
  await page.route('**/config.json', (route: Route) => route.fulfill({ json: { auth: {} } }));
  await page.route('**/api/**', (route: Route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/anordnungen');

  await expect(page).toHaveURL(/\/kasse\/anordnungen$/);
  await expect(page.locator('nav')).toBeVisible();
  await expect(page.getByTestId('sign-in')).toHaveCount(0);
  await expect(page.getByTestId('sign-out')).toHaveCount(0);
});