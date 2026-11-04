import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';
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
  const declared = [...source.matchAll(/path:\s*'([^']+)'/g)].map((m) => '/' + m[1]).sort();
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
  const hrefs = await page.locator('nav a').evaluateAll((links) =>
    links.map((l) => l.getAttribute('href') ?? '')
  );
  expect(hrefs.length).toBeGreaterThan(0);

  for (const href of hrefs) {
    await page.goto(href);
    await expect(page.locator('h2')).toBeVisible();
  }
});
