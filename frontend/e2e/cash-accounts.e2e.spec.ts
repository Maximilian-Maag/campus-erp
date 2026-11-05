import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const accounts = [
  { ID: 1, Name: 'Universität', IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR' },
];

test('Eigene Konten lists the accounts, creates one and passes axe', async ({ page }) => {
  const bodies: unknown[] = [];
  await page.route('**/api/cash-accounts', (route) => {
    if (route.request().method() === 'POST') {
      bodies.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: { ID: 2, Name: 'Zweitkonto', IBAN: 'DE89370400440532013000', BIC: 'COBADEFFXXX', Currency: 'EUR' } });
    }
    return route.fulfill({ json: accounts });
  });

  await page.goto('/kasse/konten');
  await expect(page.getByTestId('account-1')).toContainText('DE02120300000000202051');

  await page.getByLabel('Name').fill('Zweitkonto');
  await page.getByLabel('IBAN').fill('DE89370400440532013000');
  await page.getByLabel('BIC').fill('COBADEFFXXX');
  await page.getByRole('button', { name: 'Konto anlegen' }).click();

  await expect(page.getByText('Konto angelegt.').first()).toBeVisible();
  await expect(page.getByTestId('account-2')).toContainText('Zweitkonto');
  expect(bodies).toEqual([
    { name: 'Zweitkonto', iban: 'DE89370400440532013000', bic: 'COBADEFFXXX', currency: 'EUR' },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Eigene Konten reports a refused account and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 422, json: { error: 'invalid IBAN' } })
      : route.fulfill({ json: accounts })
  );

  await page.goto('/kasse/konten');
  await page.getByLabel('Name').fill('Zweitkonto');
  await page.getByLabel('IBAN').fill('DE00');
  await page.getByRole('button', { name: 'Konto anlegen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte das Konto nicht anlegen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Eigene Konten states an empty list and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/konten');

  await expect(page.getByText('Es ist noch kein eigenes Konto angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
