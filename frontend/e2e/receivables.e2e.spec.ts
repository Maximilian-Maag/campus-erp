import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const openItem = {
  ID: 1,
  MandateReference: 'MAN-1',
  AmountCents: 125000,
  Currency: 'EUR',
  DueDate: '2026-10-15T00:00:00Z',
  Purpose: 'Semesterbeitrag',
  State: 'open',
  CollectedRunID: null,
  Source: 'manual',
  CreatedBy: 'carol',
};

test('Offene Posten lists the items of a state and passes axe', async ({ page }) => {
  await page.route('**/api/receivables**', (route) => route.fulfill({ json: [openItem] }));
  await page.goto('/kasse/offene-posten');

  await expect(page.getByTestId('receivable-1')).toContainText('MAN-1');
  await expect(page.getByTestId('receivable-1')).toContainText('1.250,00');
  await expect(page.getByTestId('receivable-1')).toContainText('15.10.2026');
  await expect(page.getByTestId('receivable-1')).toContainText('offen');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Offene Posten asks the server for the chosen state and passes axe', async ({ page }) => {
  const seen: string[] = [];
  await page.route('**/api/receivables**', (route) => {
    const url = new URL(route.request().url());
    const state = url.searchParams.get('state') ?? '';
    seen.push(state);
    return route.fulfill({ json: state === 'collected' ? [{ ...openItem, State: 'collected' }] : [openItem] });
  });

  await page.goto('/kasse/offene-posten');
  await expect(page.getByTestId('receivable-1')).toContainText('offen');

  await page.getByLabel('Status', { exact: true }).selectOption('collected');
  await expect(page.getByTestId('receivable-1')).toContainText('eingezogen');

  // The filter really travelled to the server, and "alle" carries no filter.
  expect(seen).toContain('collected');
  await page.getByLabel('Status', { exact: true }).selectOption('');
  await expect(page.getByTestId('receivable-1')).toBeVisible();
  expect(seen).toContain('');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Offene Posten records an item and passes axe', async ({ page }) => {
  let stored: (typeof openItem)[] = [];
  const bodies: unknown[] = [];
  await page.route('**/api/receivables**', (route) => {
    const request = route.request();
    if (request.method() === 'POST') {
      bodies.push(request.postDataJSON());
      stored = [openItem];
      return route.fulfill({ status: 201, json: openItem });
    }
    const state = new URL(request.url()).searchParams.get('state');
    return route.fulfill({ json: state === null || state === 'open' ? stored : [] });
  });

  await page.goto('/kasse/offene-posten');
  await expect(page.getByText('Keine Posten in diesem Status.')).toBeVisible();

  await page.getByLabel('Mandat').fill('MAN-1');
  await page.getByLabel('Betrag in Euro').fill('1.250,00');
  await page.getByLabel('Fälligkeit').fill('2026-10-15');
  await page.getByLabel('Zweck').fill('Semesterbeitrag');
  await page.getByRole('button', { name: 'Erfassen' }).click();

  await expect(page.getByText('Posten erfasst.').first()).toBeVisible();
  await expect(page.getByTestId('receivable-1')).toBeVisible();
  // The amount was converted on the string, not through a float.
  expect(bodies).toEqual([
    { mandate_reference: 'MAN-1', amount_cents: 125000, currency: 'EUR', due_date: '2026-10-15', purpose: 'Semesterbeitrag', source: '' },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Offene Posten reports a refused record and passes axe', async ({ page }) => {
  await page.route('**/api/receivables**', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 404, json: { error: 'mandate not found' } });
    }
    return route.fulfill({ json: [] });
  });

  await page.goto('/kasse/offene-posten');
  await page.getByLabel('Mandat').fill('MAN-absent');
  await page.getByLabel('Betrag in Euro').fill('5');
  await page.getByLabel('Fälligkeit').fill('2026-10-15');
  await page.getByLabel('Zweck').fill('Beitrag');
  await page.getByRole('button', { name: 'Erfassen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte den Posten nicht erfassen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Offene Posten states an empty state and passes axe', async ({ page }) => {
  await page.route('**/api/receivables**', (route) => route.fulfill({ json: [] }));
  await page.goto('/kasse/offene-posten');

  await expect(page.getByText('Keine Posten in diesem Status.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
