import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const centers = [{ ID: 1, Number: 'K-10', Name: 'Verwaltung', CreatedBy: 'alice' }];
const units = [{ ID: 1, Number: 'T-1', Name: 'Studiengang A', CreatedBy: 'alice' }];

test('Kostenträgerrechnung defines a key and runs a period, and passes axe', async ({ page }) => {
  const keys = [{ ID: 1, SourceCenterID: 1, TargetUnitID: 1, ShareBP: 10000, ValidFrom: '2026-01-01T00:00:00Z', ValidTo: null, CreatedBy: 'alice' }];
  const defined: unknown[] = [];
  const runs: unknown[] = [];

  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route('**/api/klr/cost-units', (route) => route.fulfill({ json: units }));
  await page.route('**/api/klr/cost-centers/1/cost-unit-keys', (route) => {
    if (route.request().method() === 'POST') {
      defined.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: keys[0] });
    }
    return route.fulfill({ json: keys });
  });
  await page.route('**/api/klr/cost-centers/1/cost-unit-keys/run', (route) => {
    runs.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: { ID: 3, AllocatedCents: 100000, Receivers: 1 } });
  });

  await page.goto('/klr/kostentraeger');

  // The first centre is chosen, and its keys are shown with the unit and the share in percent.
  await expect(page.getByTestId('unit-key-target-1')).toContainText('T-1');
  await expect(page.getByTestId('unit-key-target-1')).toContainText('Studiengang A');
  await expect(page.getByTestId('unit-key-share-1')).toHaveText('100 %');

  // A share typed as a percentage goes out as basis points, and the key is trimmed to a day.
  await page.locator('#klr-unit-share').fill('25,5');
  await page.locator('#klr-unit-from').fill('2026-01-01');
  await page.locator('#klr-unit-until').fill('2026-12-31');
  await page.getByTestId('define-unit-key').click();
  await expect(page.getByText('Schlüssel angelegt.').first()).toBeVisible();
  expect(defined).toEqual([
    { target_unit_id: 1, share_bp: 2550, valid_from: '2026-01-01', valid_to: '2026-12-31' },
  ]);

  await page.locator('#klr-unit-run-from').fill('2026-04-01');
  await page.locator('#klr-unit-run-to').fill('2026-04-30');
  await page.getByTestId('run-unit-allocation').click();
  await expect(page.getByTestId('unit-run-result')).toContainText('1.000,00');
  await expect(page.getByTestId('unit-run-result')).toContainText('1 Kostenträger');
  expect(runs).toEqual([{ from: '2026-04-01', to: '2026-04-30' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kostenträgerrechnung reports a refused share and an empty catalogue, and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route('**/api/klr/cost-units', (route) => route.fulfill({ json: units }));
  await page.route('**/api/klr/cost-centers/1/cost-unit-keys', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 409, json: { error: 'shares do not add up' } });
    }
    return route.fulfill({ json: [] });
  });
  await page.route('**/api/klr/cost-centers/1/cost-unit-keys/run', (route) =>
    route.fulfill({ status: 422, json: { error: 'nothing to allocate' } })
  );

  await page.goto('/klr/kostentraeger');

  // A centre with no keys says so rather than showing an empty table.
  await expect(page.getByTestId('no-unit-keys')).toBeVisible();

  // A share that is not a percentage never reaches the API.
  await page.locator('#klr-unit-share').fill('keine Zahl');
  await page.locator('#klr-unit-from').fill('2026-01-01');
  await page.getByTestId('define-unit-key').click();
  await expect(page.getByRole('alert')).toContainText('Der Anteil muss eine Prozentzahl zwischen 0 und 100 sein.');

  // A refused run is reported as it comes back.
  await page.locator('#klr-unit-share').fill('100');
  await page.locator('#klr-unit-run-from').fill('2026-04-01');
  await page.locator('#klr-unit-run-to').fill('2026-04-30');
  await page.getByTestId('run-unit-allocation').click();
  await expect(page.getByRole('alert')).toContainText('Konnte die Kostenträgerrechnung nicht ausführen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
