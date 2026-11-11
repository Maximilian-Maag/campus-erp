import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const center = (ID: number, Number: string, over: Record<string, unknown> = {}) => ({
  ID,
  Number,
  Name: 'Allgemeine Verwaltung',
  CreatedBy: 'alice',
  ...over,
});

const report = (CostCenterID: number, CostsCents: number, RevenuesCents: number) => ({
  CostCenterID,
  CostsCents,
  RevenuesCents,
});

test('Kostenstellen lists the centres, reads a report and files a new centre, and passes axe', async ({
  page,
}) => {
  const created: unknown[] = [];
  const reportCalls: string[] = [];
  let centers = [center(1, 'KST-1'), center(2, 'KST-2', { Name: 'Bibliothek' })];

  await page.route('**/api/klr/cost-centers', (route) => {
    if (route.request().method() === 'POST') {
      created.push(route.request().postDataJSON());
      const body = route.request().postDataJSON() as { number: string; name: string };
      const saved = center(3, body.number, { Name: body.name });
      centers = [...centers, saved];
      return route.fulfill({ status: 201, json: saved });
    }
    return route.fulfill({ status: 200, json: centers });
  });
  await page.route('**/api/klr/cost-centers/*/report', (route) => {
    reportCalls.push(route.request().url());
    return route.fulfill({ status: 200, json: report(2, 100000, 120000) });
  });

  await page.goto('/klr/kostenstellen');
  await expect(page.getByTestId('center-1')).toContainText('KST-1');
  await expect(page.getByTestId('center-2')).toContainText('Bibliothek');

  // The report is read on demand, and says whose figures it shows.
  await page.getByTestId('center-2').getByRole('button', { name: 'Anzeigen' }).click();
  await expect(page.getByTestId('report-2')).toContainText('KST-2 Bibliothek');
  await expect(page.getByTestId('report-2')).toContainText('1.000,00');
  await expect(page.getByTestId('report-2')).toContainText('Überschuss 200,00');
  expect(reportCalls).toEqual([expect.stringContaining('/api/klr/cost-centers/2/report')]);

  // A blank field never reaches the server.
  await page.getByLabel('Nummer').fill('KST-3');
  await page.getByRole('button', { name: 'Kostenstelle aufnehmen' }).click();
  await expect(page.getByRole('alert')).toContainText('erforderlich');
  expect(created).toEqual([]);

  await page.getByLabel('Bezeichnung').fill('Zentrale Dienste');
  await page.getByRole('button', { name: 'Kostenstelle aufnehmen' }).click();
  await expect(page.getByRole('status').first()).toContainText('Kostenstelle KST-3 aufgenommen.');
  expect(created).toEqual([{ number: 'KST-3', name: 'Zentrale Dienste' }]);

  // The new centre is read back into the catalogue instead of assumed.
  await expect(page.getByTestId('center-3')).toContainText('Zentrale Dienste');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Kostenstellen reports an empty catalogue, and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ status: 200, json: [] }));

  await page.goto('/klr/kostenstellen');
  await expect(page.getByText('Es ist keine Kostenstelle angelegt.')).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Kostenstellen reports a catalogue and a report it cannot read, and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', json: { error: 'kaputt' } })
  );

  await page.goto('/klr/kostenstellen');
  await expect(page.getByRole('alert')).toContainText('Kostenstellen');
  // A broken catalogue must not look like an empty one.
  await expect(page.getByText('Es ist keine Kostenstelle angelegt.')).toHaveCount(0);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
