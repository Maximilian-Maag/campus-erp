import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const issued = {
  ReceivableID: 7,
  ClaimReference: 'receivable/7',
  MandateReference: 'MAN-1',
  AmountCents: 10000,
  DueDate: '2025-12-31T00:00:00Z',
  LevelCode: '1',
  ChargeCents: 500,
  NoticeID: 9,
  DocumentKey: 'dunning/9.pdf',
  Issued: true,
  Reason: '',
  Detail: '',
};

const skipped = {
  ...issued,
  ReceivableID: 8,
  ClaimReference: 'receivable/8',
  LevelCode: '2',
  ChargeCents: 0,
  NoticeID: 0,
  DocumentKey: '',
  Issued: false,
  Reason: 'not_due',
  Detail: '',
};

const report = {
  Year: 2026,
  AsOf: '2026-11-06T00:00:00Z',
  Issued: 1,
  Skipped: 1,
  Failed: 0,
  Outcomes: [issued, skipped],
};

test('Mahnlauf runs the dunning workflow and reports every outcome, and passes axe', async ({ page }) => {
  const bodies: unknown[] = [];
  await page.route('**/api/dunning/runs', (route) => {
    bodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 200, json: report });
  });

  await page.goto('/kasse/mahnlauf');
  await expect(page.getByRole('heading', { name: 'Mahnlauf' })).toBeVisible();

  await page.getByLabel('Jahr').fill('2026');
  await page.getByLabel('Stichtag').fill('2026-01-10');
  await page.getByRole('button', { name: 'Mahnlauf starten' }).click();

  await expect(
    page.getByText('Mahnlauf abgeschlossen: 1 ausgestellt, 1 übersprungen, 0 fehlgeschlagen.')
  ).toBeVisible();
  // The issued claim carries its level, its charge and a link to the stored notice.
  await expect(page.getByTestId('run-7').locator('td').nth(4)).toHaveText('1');
  await expect(page.getByTestId('run-7').locator('td').nth(5)).toHaveText('5,00');
  await expect(page.getByTestId('run-7')).toContainText('ausgestellt');
  await expect(page.locator('a[href="/api/dunning/notices/9/document"]')).toBeVisible();
  // A skipped claim says why, and carries no charge.
  await expect(page.getByTestId('run-8')).toContainText('Frist nicht verstrichen');
  await expect(page.getByTestId('run-8')).not.toContainText('ausgestellt');

  expect(bodies).toEqual([{ year: 2026, as_of: '2026-01-10' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mahnlauf reports a refused run and an empty day, and passes axe', async ({ page }) => {
  let refused = true;
  await page.route('**/api/dunning/runs', (route) =>
    refused
      ? route.fulfill({ status: 409, json: { error: 'the dunning levels must form one chain' } })
      : route.fulfill({ status: 200, json: { Year: 2026, AsOf: '2026-01-10T00:00:00Z', Issued: 0, Skipped: 0, Failed: 0, Outcomes: [] } })
  );

  await page.goto('/kasse/mahnlauf');
  await page.getByRole('button', { name: 'Mahnlauf starten' }).click();
  await expect(page.getByRole('alert')).toContainText('Der Mahnlauf konnte nicht ausgeführt werden.');

  refused = false;
  await page.getByRole('button', { name: 'Mahnlauf starten' }).click();
  await expect(page.getByText('Kein offener Posten war am Stichtag fällig.')).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
