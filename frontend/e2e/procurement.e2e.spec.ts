import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const base = {
  Supplier: 'Lieferant GmbH',
  BudgetPositionID: 1,
  Currency: 'EUR',
  NetCents: 11900,
  VatCents: 0,
};
const seed = () => [
  { ...base, ID: 1, Number: 'PO-1', GrossCents: 11900, Status: 'draft' },
  { ...base, ID: 2, Number: 'PO-2', GrossCents: 23800, Status: 'approved' },
];

/** The list route, honouring the `status` filter the screen now sends, and the approve action. */
async function stubOrders(
  page: import('@playwright/test').Page,
  seen: string[],
  orders: { ID: number; Number: string; Status: string }[]
) {
  // The list route (with or without a query string) ...
  await page.route(/\/api\/purchase-orders(\?.*)?$/, (route) => {
    const url = new URL(route.request().url());
    seen.push(url.search);
    const status = url.searchParams.get('status');
    return route.fulfill({ json: status ? orders.filter((o) => o.Status === status) : orders });
  });
  // ... and the approval, which is a different path.
  await page.route(/\/api\/purchase-orders\/\d+\/approve$/, (route) => {
    const id = Number(new URL(route.request().url()).pathname.split('/').at(-2));
    orders = orders.map((o) => (o.ID === id ? { ...o, Status: 'approved' } : o));
    return route.fulfill({ json: orders.find((o) => o.ID === id) });
  });
}

test('Bestellungen lists the orders, approves a draft and passes axe', async ({ page }) => {
  const seen: string[] = [];
  await stubOrders(page, seen, seed());

  await page.goto('/procurement/bestellungen');
  await expect(page.getByTestId('status-PO-1')).toContainText('Entwurf');
  await expect(page.getByTestId('order-PO-2')).toContainText('238,00');
  // Nothing is filtered on the first load.
  expect(seen).toEqual(['']);

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);

  await page.getByRole('button', { name: 'Bestellung PO-1 genehmigen' }).click();
  await expect(page.getByTestId('status-PO-1')).toContainText('Genehmigt');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Bestellungen filters on the server and passes axe', async ({ page }) => {
  const seen: string[] = [];
  await stubOrders(page, seen, seed());

  await page.goto('/procurement/bestellungen');
  await expect(page.getByTestId('order-PO-1')).toBeVisible();

  await page.getByLabel('Status').selectOption('approved');
  await expect(page.getByTestId('order-PO-2')).toBeVisible();
  await expect(page.getByTestId('order-PO-1')).toHaveCount(0);
  // The filter travelled to the API rather than being applied to what was loaded.
  expect(seen).toContain('?status=approved');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Bestellungen reports a failed load and passes axe', async ({ page }) => {
  await page.route(/\/api\/purchase-orders(\?.*)?$/, (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );
  await page.goto('/procurement/bestellungen');

  await expect(page.getByRole('alert')).toContainText('Konnte die Bestellungen nicht laden.');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
