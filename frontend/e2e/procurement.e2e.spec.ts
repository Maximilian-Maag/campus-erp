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
  { ...base, ID: 1, Number: 'PO-1', GrossCents: 11900, InvoicedCents: 0, Status: 'draft' },
  { ...base, ID: 2, Number: 'PO-2', GrossCents: 23800, InvoicedCents: 5000, Status: 'approved' },
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
  // A part-billed order says so, so it is not mistaken for one still waiting for its invoice.
  await expect(page.getByTestId('billed-PO-2')).toContainText('50,00 von 238,00');
  await expect(page.getByTestId('billed-PO-1')).toContainText('—');
  // Nothing is filtered on the first load, and the first page is asked for explicitly. The search is
  // read as parameters rather than compared as a string: the page travels in it too, and a string
  // comparison would make this assertion about the page size instead of about the filter.
  expect(seen.length).toBe(1);
  const first = new URLSearchParams(seen[0]);
  expect(first.get('status')).toBeNull();
  expect(first.get('limit')).toBe('50');
  expect(first.get('offset')).toBe('0');

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
  // The filter travelled to the API rather than being applied to what was loaded. Read as a
  // parameter, so the page that travels beside it does not have to be spelled out here.
  expect(seen.some((s) => new URLSearchParams(s).get('status') === 'approved')).toBe(true);

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

test('Bestellungen reads the orders one page at a time, and passes axe', async ({ page }) => {
  const offsets: string[] = [];
  const page50 = Array.from({ length: 50 }, (_, i) => ({
    ...base, ID: i + 1, Number: `PO-${i + 1}`, GrossCents: 11900, InvoicedCents: 0, Status: 'draft',
  }));

  await page.route(/\/api\/purchase-orders(\?.*)?$/, (route) => {
    const url = new URL(route.request().url());
    offsets.push(url.searchParams.get('offset') ?? '');
    // The first page is full, so the screen offers the next one; the second is short, so it stops.
    const offset = Number(url.searchParams.get('offset') ?? '0');
    return route.fulfill({
      json: offset === 0
        ? page50
        : [{ ...base, ID: 51, Number: 'PO-51', GrossCents: 11900, InvoicedCents: 0, Status: 'draft' }],
    });
  });

  await page.goto('/procurement/bestellungen');
  await expect(page.getByTestId('order-PO-1')).toBeVisible();
  await expect(page.getByTestId('order-PO-50')).toBeVisible();
  expect(offsets).toEqual(['0']);

  await page.getByTestId('load-more').click();
  await expect(page.getByTestId('order-PO-51')).toBeVisible();
  // The pages continue rather than repeat, and what was read is still on screen.
  expect(offsets).toEqual(['0', '50']);
  await expect(page.getByTestId('order-PO-1')).toBeVisible();
  await expect(page.getByTestId('load-more')).toHaveCount(0);

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
