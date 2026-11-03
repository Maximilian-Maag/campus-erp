import { test, expect, type Route } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const base = {
  AmountCents: 125000,
  Currency: 'EUR',
  BudgetPositionID: 1,
  Purpose: 'Stromabschlag',
  DueDate: '2026-04-01T00:00:00Z',
};

const draft = { ...base, ID: 1, Reference: 'AO-1', Status: 'draft' };
const submitted = { ...base, ID: 2, Reference: 'AO-2', Status: 'submitted' };
const approved = { ...base, ID: 3, Reference: 'AO-3', Status: 'approved' };

/** Stubs the list route, honouring the status filter the screen sends. */
function stubList(orders: typeof draft[]) {
  return (route: Route) => {
    const status = new URL(route.request().url()).searchParams.get('status');
    return route.fulfill({ json: status ? orders.filter((o) => o.Status === status) : orders });
  };
}

test('Zahlungsanordnungen lists the orders with their states and passes axe', async ({ page }) => {
  await page.route(/\/api\/payment-orders(\?.*)?$/, stubList([draft, submitted, approved]));
  await page.goto('/kasse/anordnungen');

  await expect(page.getByTestId('order-AO-1')).toContainText('Stromabschlag');
  await expect(page.getByTestId('order-AO-1')).toContainText('1.250,00');
  await expect(page.getByTestId('order-AO-1')).toContainText('01.04.2026');
  await expect(page.getByTestId('status-AO-1')).toContainText('Entwurf');
  await expect(page.getByTestId('status-AO-2')).toContainText('Vorgelegt');
  await expect(page.getByTestId('status-AO-3')).toContainText('Genehmigt');

  // Only the states that allow an action offer one.
  await expect(page.getByTestId('order-AO-1').getByRole('button', { name: 'Vorlegen' })).toBeVisible();
  await expect(page.getByTestId('order-AO-3').getByRole('button')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsanordnungen filters by status on the server and passes axe', async ({ page }) => {
  const seen: string[] = [];
  await page.route(/\/api\/payment-orders(\?.*)?$/, (route) => {
    const url = new URL(route.request().url());
    seen.push(url.search);
    const status = url.searchParams.get('status');
    return route.fulfill({ json: status ? [draft, submitted, approved].filter((o) => o.Status === status) : [draft, submitted, approved] });
  });

  await page.goto('/kasse/anordnungen');
  await expect(page.getByTestId('order-AO-1')).toBeVisible();

  await page.getByLabel('Status').selectOption('submitted');
  await expect(page.getByTestId('order-AO-2')).toBeVisible();
  await expect(page.getByTestId('order-AO-1')).toHaveCount(0);

  // The filter really travelled to the server.
  expect(seen).toContain('?status=submitted');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsanordnungen submits a draft and shows the new state and passes axe', async ({ page }) => {
  let state = [draft];
  await page.route(/\/api\/payment-orders(\?.*)?$/, (route) => route.fulfill({ json: state }));
  await page.route(/\/api\/payment-orders\/1\/submit$/, (route) => {
    state = [submitted];
    return route.fulfill({ json: { ID: 1, Status: 'submitted' } });
  });

  await page.goto('/kasse/anordnungen');
  await page.getByRole('button', { name: 'Anordnung AO-1 vorlegen' }).click();

  await expect(page.getByTestId('status-AO-2')).toContainText('Vorgelegt');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsanordnungen asks for a reason before rejecting and passes axe', async ({ page }) => {
  let state = [submitted];
  const bodies: unknown[] = [];
  await page.route(/\/api\/payment-orders(\?.*)?$/, (route) => route.fulfill({ json: state }));
  await page.route(/\/api\/payment-orders\/2\/reject$/, (route) => {
    bodies.push(route.request().postDataJSON());
    state = [];
    return route.fulfill({ json: { ID: 2, Status: 'rejected' } });
  });

  await page.goto('/kasse/anordnungen');
  await page.getByRole('button', { name: 'Anordnung AO-2 ablehnen' }).click();

  // The reason box is there, and nothing has been sent yet.
  const reason = page.getByLabel('Grund');
  await expect(reason).toBeVisible();
  expect(bodies).toEqual([]);

  await reason.fill('Sachlich falsch');
  await page.getByRole('button', { name: 'Ablehnung bestätigen' }).click();

  await expect(page.getByText('Es sind keine Zahlungsanordnungen angelegt.')).toBeVisible();
  expect(bodies).toEqual([{ reason: 'Sachlich falsch' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsanordnungen reports a refused action and passes axe', async ({ page }) => {
  await page.route(/\/api\/payment-orders(\?.*)?$/, stubList([submitted]));
  await page.route(/\/api\/payment-orders\/2\/approve$/, (route) =>
    route.fulfill({ status: 403, json: { error: 'four eyes' } })
  );

  await page.goto('/kasse/anordnungen');
  await page.getByRole('button', { name: 'Anordnung AO-2 genehmigen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte die Anordnung AO-2 nicht genehmigen.');
  // The list is still there; a refused action is not a failed load.
  await expect(page.getByTestId('order-AO-2')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsanordnungen states an empty list and passes axe', async ({ page }) => {
  await page.route(/\/api\/payment-orders(\?.*)?$/, (route) => route.fulfill({ json: [] }));
  await page.goto('/kasse/anordnungen');

  await expect(page.getByText('Es sind keine Zahlungsanordnungen angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
