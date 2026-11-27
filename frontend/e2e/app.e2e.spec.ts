import { readFileSync } from 'node:fs';
import { test, expect } from './session';
import AxeBuilder from '@axe-core/playwright';

test('shell renders and is accessible', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('app-root')).toBeVisible();

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('every screen is reachable from the navigation', async ({ page }) => {
  // A screen nobody can navigate to is not finished: the shell's links and the route table have
  // to agree, and this is what keeps the next screen from being added to only one of them.
  //
  // The route table is read as text rather than imported: a Playwright spec runs in Node, and
  // importing it would pull the Angular components in with it.
  const source = readFileSync('src/app/app.routes.ts', 'utf8');
  // The empty path is the root redirect and `**` the not-found catch-all: neither is a screen
  // and neither carries a navigation entry. The sign-in screen and its callback are reached by
  // a redirect from the IdP, not from the menu.
  const notInTheMenu = new Set(['/', '/**', '/login', '/auth/callback']);
  const declared = [...source.matchAll(/path:\s*'([^']+)'/g)]
    .map((m) => '/' + m[1])
    .filter((path) => !notInTheMenu.has(path))
    .sort();
  expect(declared.length).toBeGreaterThan(0);

  await page.goto('/');
  const linked = (await page.locator('nav a').evaluateAll((links) =>
    links.map((l) => l.getAttribute('href'))
  )).sort();

  expect(declared).toEqual(linked);
});

test('the navigation links resolve to a screen', async ({ page }) => {
  // Every link leads somewhere that renders, not to a blank outlet.
  await page.route('**/api/**', (route) => route.fulfill({ json: [] }));
  await page.goto('/');
  // The shell renders after the deployment's sign-in configuration has been read, so the
  // navigation is one round trip away from being in the document.
  await page.locator('nav a').first().waitFor();
  const hrefs = await page.locator('nav a').evaluateAll((links) =>
    links.map((l) => l.getAttribute('href') ?? '')
  );
  expect(hrefs.length).toBeGreaterThan(0);

  for (const href of hrefs) {
    await page.goto(href);
    const title = page.getByTestId('page-title');
    await expect(title).toBeVisible();
    // A screen whose heading is its own message key is a screen nobody translated: the
    // dictionaries are typed, but a key that names no entry renders as itself.
    await expect(title).not.toHaveText(/^[a-z]+\.[a-zA-Z]/);
  }
});

test('the language picker switches the interface and remembers the choice', async ({ page }) => {
  // The product is published in German and English: the picker has to change the whole
  // interface, tell the document, and still be in force after a reload.
  await page.route('**/api/**', (route) => route.fulfill({ json: [] }));
  await page.goto('/kasse/anordnungen');
  await expect(page.getByTestId('page-title')).toHaveText('Ausgabeanordnungen');

  await page.getByTestId('language-picker').click();
  await page.getByTestId('language-option-en').click();

  await expect(page.getByTestId('page-title')).toHaveText('Payment orders');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');

  await page.reload();
  await expect(page.getByTestId('page-title')).toHaveText('Payment orders');
});

test('an unknown address says so instead of rendering a blank outlet', async ({ page }) => {
  await page.goto('/gibt-es-nicht');
  await expect(page.getByTestId('page-title')).toHaveText('Seite nicht gefunden');
  await expect(page.getByTestId('not-found-back')).toHaveAttribute('href', '/kasse/tag');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('the browser tab is named after the screen, in the chosen language', async ({ page }) => {
  await page.route('**/api/**', (route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/anordnungen');
  await expect(page).toHaveTitle('Ausgabeanordnungen · Campus ERP');

  await page.getByTestId('language-picker').click();
  await page.getByTestId('language-option-en').click();
  await expect(page).toHaveTitle('Payment orders · Campus ERP');
});
