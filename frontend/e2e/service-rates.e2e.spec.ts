import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const costTypes = [{ ID: 1, Code: '4711', Name: 'Personalkosten', CreatedBy: 'alice' }];
const centers = [
  { ID: 1, Number: 'P-1', Name: 'Rechenzentrum', CreatedBy: 'alice' },
  { ID: 2, Number: 'R-1', Name: 'Bibliothek', CreatedBy: 'alice' },
];
const rates = [
  { ID: 1, CostTypeID: 1, RateCents: 2500, ValidFrom: '2026-01-01T00:00:00Z', ValidTo: null, CreatedBy: 'alice' },
];

test('Verrechnungspreise shows the rates, defines one and charges a service, and passes axe', async ({ page }) => {
  const defined: unknown[] = [];
  const charges: unknown[] = [];

  await page.route('**/api/klr/cost-types', (route) => route.fulfill({ json: costTypes }));
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route('**/api/klr/service-rates', (route) => {
    if (route.request().method() === 'POST') {
      defined.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: rates[0] });
    }
    return route.fulfill({ json: rates });
  });
  await page.route('**/api/klr/service-charges', (route) => {
    charges.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: { ID: 4, AmountCents: 7500, Quantity: 3 } });
  });

  await page.goto('/klr/verrechnungspreise');

  // The rate list, with the cost type as the operator reads it and the price in euro.
  await expect(page.getByTestId('rate-cost-type-1')).toHaveText('4711 Personalkosten');
  await expect(page.getByTestId('rate-amount-1')).toHaveText('25,00');
  await expect(page.getByTestId('rate-1')).toContainText('unbegrenzt');

  // A price typed in euros goes out in cents, and the period is trimmed to days.
  await page.locator('#klr-rate-amount').fill('25,50');
  await page.locator('#klr-rate-from').fill('2026-01-01');
  await page.locator('#klr-rate-until').fill('2026-12-31');
  await page.getByTestId('define-rate').click();
  await expect(page.getByText('Verrechnungspreis hinterlegt.').first()).toBeVisible();
  expect(defined).toEqual([
    { cost_type_id: 1, rate_cents: 2550, valid_from: '2026-01-01', valid_to: '2026-12-31' },
  ]);

  // The charge names both ends, the quantity and the day the service was rendered.
  await page.locator('#klr-charge-quantity').fill('3');
  await page.locator('#klr-charge-on').fill('2026-04-30');
  await page.getByTestId('charge-service').click();
  await expect(page.getByTestId('charge-result')).toContainText('75,00');
  await expect(page.getByTestId('charge-result')).toContainText('3 Einheiten');
  expect(charges).toEqual([
    { provider_center_id: 1, receiver_center_id: 2, cost_type_id: 1, quantity: 3, charged_on: '2026-04-30' },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Verrechnungspreise reports refusals and an empty catalogue, and passes axe', async ({ page }) => {
  await page.route('**/api/klr/cost-types', (route) => route.fulfill({ json: costTypes }));
  await page.route('**/api/klr/cost-centers', (route) => route.fulfill({ json: centers }));
  await page.route('**/api/klr/service-rates', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 422, json: { error: 'rate must be positive' } });
    }
    return route.fulfill({ json: [] });
  });
  await page.route('**/api/klr/service-charges', (route) =>
    route.fulfill({ status: 422, json: { error: 'no service rate is in force' } })
  );

  await page.goto('/klr/verrechnungspreise');

  // No rates yet says so rather than showing an empty table.
  await expect(page.getByTestId('no-rates')).toBeVisible();

  // A price that is not an amount never reaches the API.
  await page.locator('#klr-rate-amount').fill('keine Zahl');
  await page.locator('#klr-rate-from').fill('2026-01-01');
  await page.getByTestId('define-rate').click();
  await expect(page.getByRole('alert')).toContainText('Der Preis muss ein Betrag über 0 sein.');

  // A refused rate is reported as it comes back.
  await page.locator('#klr-rate-amount').fill('25,50');
  await page.getByTestId('define-rate').click();
  await expect(page.getByRole('alert')).toContainText('Konnte den Verrechnungspreis nicht hinterlegen.');

  // A quantity that is not a whole number never reaches the API either.
  await page.locator('#klr-charge-quantity').fill('1,5');
  await page.locator('#klr-charge-on').fill('2026-04-30');
  await page.getByTestId('charge-service').click();
  await expect(page.getByRole('alert')).toContainText('Die Menge muss eine ganze Zahl über 0 sein.');

  // And a refused charge comes back as the API words it.
  await page.locator('#klr-charge-quantity').fill('3');
  await page.getByTestId('charge-service').click();
  await expect(page.getByRole('alert')).toContainText('Konnte die Leistung nicht verrechnen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
