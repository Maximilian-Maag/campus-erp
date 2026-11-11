import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const vat = (Code: string, RateBasisPoints: number, over: Record<string, unknown> = {}) => ({
  ID: 1,
  Code,
  Description: 'Regelsteuersatz',
  RateBasisPoints,
  Kind: 'output',
  ...over,
});

const office = (ID: number, Year: number, Number: string, over: Record<string, unknown> = {}) => ({
  ID,
  Year,
  Number,
  Name: 'Finanzamt Musterstadt',
  Street: 'Amtsweg 1',
  PostalCode: '12345',
  City: 'Musterstadt',
  AccountHolder: 'Landeshauptkasse',
  IBAN: 'DE02120300000000202051',
  BIC: 'BYLADEM1001',
  ...over,
});

test('Steuersätze und Finanzämter shows both catalogues and files a new rate, and passes axe', async ({
  page,
}) => {
  const created: unknown[] = [];
  const officeQueries: string[] = [];
  let vats = [vat('V19', 1900)];

  await page.route('**/api/vat-types', (route) => {
    if (route.request().method() === 'POST') {
      created.push(route.request().postDataJSON());
      const body = route.request().postDataJSON() as { code: string; rate_basis_points: number };
      vats = [...vats, vat(body.code, body.rate_basis_points, { ID: 2, Description: 'ermaessigt', RateBasisPoints: 700, Kind: 'input' })];
      return route.fulfill({ status: 201, json: vats[vats.length - 1] });
    }
    return route.fulfill({ status: 200, json: vats });
  });
  await page.route('**/api/tax-offices**', (route) => {
    officeQueries.push(route.request().url());
    return route.fulfill({ status: 200, json: [office(4, 2026, 'F-4')] });
  });

  await page.goto('/finanzen/steuern');

  // The rate reads as a percentage and the kind in words, not as the API's codes.
  await expect(page.getByTestId('vat-V19')).toContainText('19 %');
  await expect(page.getByTestId('vat-V19')).toContainText('Umsatzsteuer');
  await expect(page.getByTestId('office-4')).toContainText('Finanzamt Musterstadt');
  await expect(page.getByTestId('office-4')).toContainText('DE02120300000000202051');

  // A rate that is not a percentage never reaches the server.
  await page.getByLabel('Schlüssel').fill('V7');
  await page.getByLabel('Bezeichnung').fill('ermaessigt');
  await page.getByLabel('Satz (in Prozent)').fill('sieben');
  await page.getByRole('button', { name: 'Steuersatz aufnehmen' }).click();
  await expect(page.getByRole('alert')).toContainText('Prozentangabe');
  expect(created).toEqual([]);

  // A decimal rate goes as basis points.
  await page.getByLabel('Satz (in Prozent)').fill('7');
  await page.getByLabel('Art').selectOption('input');
  await page.getByRole('button', { name: 'Steuersatz aufnehmen' }).click();
  await expect(page.getByRole('status')).toContainText('Steuersatz V7 aufgenommen.');
  expect(created).toEqual([
    { code: 'V7', description: 'ermaessigt', rate_basis_points: 700, kind: 'input' },
  ]);

  // The new rate is read back into the catalogue instead of assumed.
  await expect(page.getByTestId('vat-V7')).toContainText('Vorsteuer');

  // The year narrows the offices at the server, not in the screen.
  await page.getByLabel('Jahr der Anzeige').fill('2025');
  await page.getByRole('button', { name: 'Anzeigen' }).click();
  await expect.poll(() => officeQueries.some((q) => q.includes('year=2025'))).toBe(true);

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Steuersätze und Finanzämter reports an empty installation, and passes axe', async ({ page }) => {
  await page.route('**/api/vat-types', (route) => route.fulfill({ status: 200, json: [] }));
  await page.route('**/api/tax-offices**', (route) => route.fulfill({ status: 200, json: [] }));

  await page.goto('/finanzen/steuern');
  await expect(page.getByText('Es ist kein Steuersatz erfasst.')).toBeVisible();
  await expect(page.getByText('Es ist kein Finanzamt erfasst.')).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Steuersätze und Finanzämter reports a read that fails, and passes axe', async ({ page }) => {
  await page.route('**/api/vat-types', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', json: { title: 'kaputt' } })
  );
  await page.route('**/api/tax-offices**', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', json: { title: 'kaputt' } })
  );

  await page.goto('/finanzen/steuern');
  // Both failures are visible: one unreadable catalogue must not hide the other's report.
  await expect(page.getByRole('alert').filter({ hasText: 'Steuersätze' })).toBeVisible();
  await expect(page.getByRole('alert').filter({ hasText: 'Finanzämter' })).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
