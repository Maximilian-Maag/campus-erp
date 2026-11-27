import { test, expect, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

import { ACCESS_TOKEN, signIn, stubIdp } from './session';

/**
 * The interface's own sign-in code, without the fixture the rest of the suite uses.
 *
 * The other specs start signed in (`./session`); this one has to see the states that exist
 * *before* a session — the redirect, the callback, the sign-out — so it drives the round trip
 * itself, and it is the one place that asserts what a deployment without an issuer does.
 */

test('a visitor without a session is sent to the login screen, and signing in brings them back', async ({
  page,
}) => {
  const seen: Record<string, string | undefined> = {};
  await page.route('**/api/**', (route: Route) => {
    seen[new URL(route.request().url()).pathname] = route.request().headers()['authorization'];
    return route.fulfill({ json: [] });
  });

  // The screen they asked for is a screen behind the login.
  await stubIdp(page);
  await page.goto('/kasse/anordnungen');
  await expect(page).toHaveURL(/\/login\?returnUrl=%2Fkasse%2Fanordnungen/);
  await expect(page.getByRole('heading', { name: 'Anmelden' })).toBeVisible();

  // One action, and it leaves for the university IdP.
  await page.getByTestId('sign-in').click();

  // Back on the screen they asked for, greeted by name.
  await expect(page).toHaveURL(/\/kasse\/anordnungen$/);
  await expect(page.getByTestId('principal')).toHaveText('Max Maag');

  // And the API call carried the token: that is the point of the whole round trip.
  await expect.poll(() => seen['/api/payment-orders']).toBe(`Bearer ${ACCESS_TOKEN}`);
});

test('the login screen passes axe', async ({ page }) => {
  await stubIdp(page);
  await page.goto('/login');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('signing out forgets the session and asks again', async ({ page }) => {
  await signIn(page);

  await page.getByTestId('sign-out').click();

  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByTestId('principal')).toHaveCount(0);
  // A reload must not resurrect it: the session lives in the tab, and the tab forgot it.
  await page.reload();
  await expect(page.getByTestId('sign-in')).toBeVisible();
});

test('a deployment without an issuer runs without a login screen', async ({ page }) => {
  // The shape `make run` has and the compose stack defaults to: the API decides for itself
  // (it is open there), so the interface must not pretend to be signed in.
  await page.route('**/config.json', (route: Route) => route.fulfill({ json: { auth: {} } }));
  await page.route('**/api/**', (route: Route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/anordnungen');

  await expect(page).toHaveURL(/\/kasse\/anordnungen$/);
  await expect(page.locator('nav')).toBeVisible();
  await expect(page.getByTestId('sign-in')).toHaveCount(0);
  await expect(page.getByTestId('sign-out')).toHaveCount(0);
});
