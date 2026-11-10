import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const approvedOrder = {
  ID: 7,
  Number: 'PO-2026-1',
  Supplier: 'Lieferant GmbH',
  BudgetPositionID: 1,
  Currency: 'EUR',
  NetCents: 11000,
  VatCents: 2090,
  GrossCents: 13090,
  Status: 'approved',
};

const openState = {
  OrderID: 7,
  Complete: false,
  Progress: [
    { LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 0 },
    { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 500 },
  ],
  Receipts: [
    {
      ID: 3,
      OrderID: 7,
      Reference: 'LS-1',
      ReceivedOn: '2026-11-20T00:00:00Z',
      ReceivedBy: 'carol',
      Lines: [{ LineNo: 2, QuantityMilli: 500 }],
      ReceivedCents: 298,
    },
  ],
};

test('Wareneingang shows the arrival per line and books a partial delivery, and passes axe', async ({ page }) => {
  const posts: unknown[] = [];
  await page.route('**/api/purchase-orders?status=approved', (route) =>
    route.fulfill({ status: 200, json: [approvedOrder] })
  );
  await page.route('**/api/purchase-orders/7/receipts', (route) => {
    if (route.request().method() === 'POST') {
      posts.push(route.request().postDataJSON());
      return route.fulfill({
        status: 201,
        json: {
          Receipt: { ID: 4, OrderID: 7, Reference: 'LS-2', ReceivedOn: '2026-11-25T00:00:00Z', ReceivedBy: 'carol', Lines: [{ LineNo: 1, QuantityMilli: 500 }], ReceivedCents: 5950 },
          State: { ...openState, Progress: [{ LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 500 }, { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 500 }] },
        },
      });
    }
    return route.fulfill({ status: 200, json: openState });
  });

  await page.goto('/procurement/wareneingang');
  await expect(page.getByRole('heading', { name: 'Wareneingang' })).toBeVisible();

  await page.getByLabel('Bestellung').selectOption('7');

  // Every line's ordered / received / open quantity, as the server derived them.
  await expect(page.getByTestId('line-1').locator('td').nth(2)).toHaveText('1');
  await expect(page.getByTestId('line-1').locator('td').nth(4)).toHaveText('1');
  await expect(page.getByTestId('line-2').locator('td').nth(3)).toHaveText('0,5');
  await expect(page.getByTestId('line-2').locator('td').nth(4)).toHaveText('1,5');
  await expect(page.getByTestId('receipt-3')).toContainText('LS-1');
  await expect(page.getByTestId('receipt-3')).toContainText('2,98');

  // A quantity is typed in units: half a unit is 500 milli-units on the wire.
  await page.getByLabel('Lieferschein').fill('LS-2');
  await page.getByTestId('quantity-1').fill('0,5');
  await page.getByTestId('quantity-2').fill('1,5');
  await page.getByRole('button', { name: 'Wareneingang buchen' }).click();

  await expect(page.getByText('Wareneingang gebucht (59,50).')).toBeVisible();
  expect(posts).toEqual([
    {
      reference: 'LS-2',
      received_on: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
      lines: [
        { line_no: 1, quantity_milli: 500 },
        { line_no: 2, quantity_milli: 1500 },
      ],
    },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Wareneingang reports a refused booking and an order that is complete, and passes axe', async ({ page }) => {
  let complete = false;
  await page.route('**/api/purchase-orders?status=approved', (route) =>
    route.fulfill({ status: 200, json: [approvedOrder] })
  );
  await page.route('**/api/purchase-orders/7/receipts', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 422, json: { error: 'the receipt exceeds the ordered quantity' } });
    }
    return route.fulfill({
      status: 200,
      json: complete
        ? { ...openState, Complete: true, Progress: [{ LineNo: 1, Description: 'Ware', OrderedMilli: 1000, ReceivedMilli: 1000 }, { LineNo: 2, Description: 'Versand', OrderedMilli: 2000, ReceivedMilli: 2000 }] }
        : openState,
    });
  });

  await page.goto('/procurement/wareneingang');
  await page.getByLabel('Bestellung').selectOption('7');
  await page.getByLabel('Lieferschein').fill('LS-2');
  await page.getByTestId('quantity-1').fill('2');
  await page.getByRole('button', { name: 'Wareneingang buchen' }).click();
  await expect(page.getByRole('alert')).toContainText('Der Wareneingang konnte nicht gebucht werden.');

  // An order that arrived in full offers no further booking.
  complete = true;
  await page.getByLabel('Bestellung').selectOption('');
  await page.getByLabel('Bestellung').selectOption('7');
  await expect(page.getByText('Die Bestellung ist vollständig geliefert.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Wareneingang buchen' })).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Wareneingang states an empty list of approved orders, and passes axe', async ({ page }) => {
  await page.route('**/api/purchase-orders?status=approved', (route) =>
    route.fulfill({ status: 200, json: [] })
  );

  await page.goto('/procurement/wareneingang');
  await expect(page.getByText('Es ist keine freigegebene Bestellung offen.')).toBeVisible();
  await expect(page.getByLabel('Bestellung')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
