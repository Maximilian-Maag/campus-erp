import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const base = {
  Supplier: 'Lieferant GmbH',
  BudgetPositionID: 1,
  Currency: 'EUR',
  NetCents: 11900,
  VatCents: 0,
};

test('Bestellungen lists the orders, approves a draft and passes axe', async ({ page }) => {
  let orders = [
    { ...base, ID: 1, Number: 'PO-1', GrossCents: 11900, Status: 'draft' },
    { ...base, ID: 2, Number: 'PO-2', GrossCents: 23800, Status: 'approved' },
  ];
  await page.route('**/api/purchase-orders', (route) => route.fulfill({ json: orders }));
  await page.route('**/api/purchase-orders/1/approve', (route) => {
    orders = [{ ...orders[0], Status: 'approved' }, orders[1]];
    return route.fulfill({ json: orders[0] });
  });

  await page.goto('/procurement/bestellungen');
  await expect(page.getByTestId('status-PO-1')).toContainText('Entwurf');
  await expect(page.getByTestId('order-PO-2')).toContainText('238,00');

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);

  await page.getByRole('button', { name: 'Bestellung PO-1 genehmigen' }).click();
  await expect(page.getByTestId('status-PO-1')).toContainText('Genehmigt');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Bestellungen filters by status and passes axe', async ({ page }) => {
  const orders = [
    { ...base, ID: 1, Number: 'PO-1', GrossCents: 11900, Status: 'draft' },
    { ...base, ID: 2, Number: 'PO-2', GrossCents: 23800, Status: 'approved' },
  ];
  await page.route('**/api/purchase-orders', (route) => route.fulfill({ json: orders }));
  await page.goto('/procurement/bestellungen');
  await expect(page.getByTestId('order-PO-1')).toBeVisible();

  await page.getByLabel('Status').selectOption('approved');
  await expect(page.getByTestId('order-PO-2')).toBeVisible();
  await expect(page.getByTestId('order-PO-1')).toHaveCount(0);

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Bestellungen reports a failed load and passes axe', async ({ page }) => {
  await page.route('**/api/purchase-orders', (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );
  await page.goto('/procurement/bestellungen');

  await expect(page.getByRole('alert')).toContainText('Konnte die Bestellungen nicht laden.');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
