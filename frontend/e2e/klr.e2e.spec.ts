import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const centers = [
  { ID: 1, Number: 'K-10', Name: 'Bibliothek', CreatedBy: 'alice' },
  { ID: 2, Number: 'K-20', Name: 'Werkstatt', CreatedBy: 'alice' },
];

const key = {
  ID: 7,
  SourceCenterID: 1,
  TargetCenterID: 2,
  ShareBP: 2500,
  ValidFrom: '2026-04-01',
  ValidTo: null,
  CreatedBy: 'alice',
};

test('Umlage shows the keys of a cost centre and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({ json: centers });
    }
    return route.fulfill({ json: centers[0] });
  });
  await page.route(/\/api\/klr\/cost-centers\/\d+\/allocations$/, (route) => route.fulfill({ json: [key] }));

  await page.goto('/klr/umlage');

  await expect(page.getByTestId('key-7')).toBeVisible();
  await expect(page.getByTestId('key-target-7')).toContainText('K-20 Werkstatt');
  await expect(page.getByTestId('key-share-7')).toContainText('25 %');
  // The key has no end date, so it is open-ended rather than blank.
  await expect(page.getByTestId('key-7')).toContainText('unbegrenzt');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Umlage defines a key, runs a period and passes axe', async ({ page }) => {
  const posted: unknown[] = [];
  const runs: unknown[] = [];
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route(/\/api\/klr\/cost-centers\/\d+\/allocations$/, (route) => {
    if (route.request().method() === 'POST') {
      posted.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: key });
    }
    return route.fulfill({ json: [] });
  });
  await page.route(/\/api\/klr\/cost-centers\/\d+\/allocations\/run$/, (route) => {
    runs.push(route.request().postDataJSON());
    return route.fulfill({
      status: 201,
      json: { ID: 3, SourceCenterID: 1, PeriodFrom: '2026-04-01', PeriodTo: '2026-04-30', AllocatedCents: 80000, Receivers: 1, RunBy: 'carol' },
    });
  });

  await page.goto('/klr/umlage');
  await expect(page.getByText('Für diese Kostenstelle ist kein Schlüssel hinterlegt.')).toBeVisible();

  // A share typed as a percentage travels as basis points.
  await page.getByLabel('Anteil in Prozent').fill('12,5');
  await page.getByLabel('Gültig ab').fill('2026-04-01');
  await page.getByRole('button', { name: 'Anlegen' }).click();
  await expect(page.getByText('Schlüssel angelegt.')).toBeVisible();

  await page.getByLabel('Von').fill('2026-04-01');
  await page.getByLabel('Bis', { exact: true }).fill('2026-04-30');
  await page.getByRole('button', { name: 'Ausführen' }).click();
  await expect(page.getByTestId('run-result')).toContainText('800,00');
  await expect(page.getByTestId('run-result')).toContainText('1 Empfänger');

  expect(posted).toEqual([{ target_center_id: 2, share_bp: 1250, valid_from: '2026-04-01', valid_to: '' }]);
  expect(runs).toEqual([{ from: '2026-04-01', to: '2026-04-30' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Umlage reports a refused run and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route(/\/api\/klr\/cost-centers\/\d+\/allocations$/, (route) => route.fulfill({ json: [key] }));
  await page.route(/\/api\/klr\/cost-centers\/\d+\/allocations\/run$/, (route) =>
    route.fulfill({ status: 409, json: { error: 'already allocated' } })
  );

  await page.goto('/klr/umlage');
  await page.getByLabel('Von').fill('2026-04-01');
  await page.getByLabel('Bis', { exact: true }).fill('2026-04-30');
  await page.getByRole('button', { name: 'Ausführen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte die Umlage nicht ausführen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Umlage states an empty centre list and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: [] }));

  await page.goto('/klr/umlage');
  await expect(page.getByText('Es sind keine Kostenstellen angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
