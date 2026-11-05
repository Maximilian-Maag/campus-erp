import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const accounts = [
  { ID: 1, Name: 'Universität', IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR' },
];

const approved = [
  { ID: 1, Reference: 'AO-1', AmountCents: 2500, Currency: 'EUR', BudgetPositionID: 1, Purpose: 'Miete', DueDate: '2026-11-01T00:00:00Z', Status: 'approved' },
  { ID: 2, Reference: 'AO-2', AmountCents: 3000, Currency: 'EUR', BudgetPositionID: 1, Purpose: 'Strom', DueDate: '2026-11-01T00:00:00Z', Status: 'approved' },
];

const run = {
  ID: 1,
  Reference: 'RUN-1',
  AccountID: 1,
  ExecutionDate: '2026-11-01T00:00:00Z',
  MessageID: 'RUN-1',
  ItemCount: 2,
  SumCents: 5500,
  FileHash: 'h',
  CreatedBy: 'carol',
};

test('Zahlungsläufe creates a run from the approved orders and shows the file, and passes axe', async ({ page }) => {
  const bodies: unknown[] = [];
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/payment-orders\?status=approved/, (route) => route.fulfill({ json: approved }));
  await page.route('**/api/payment-runs', (route) => {
    bodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: run });
  });
  await page.route('**/api/payment-runs/1/file', (route) =>
    route.fulfill({ contentType: 'application/xml', body: '<Document><CstmrCdtTrfInitn/></Document>' })
  );
  await page.route('**/api/payment-runs/1/exports', (route) => route.fulfill({ status: 202, json: { job_id: 77 } }));

  await page.goto('/kasse/zahlungslaeufe');
  await page.getByLabel('Referenz').fill('RUN-1');
  await page.getByLabel('Ausführungsdatum').fill('2026-11-01');
  await page.getByLabel('Anordnung AO-1 auswählen').check();
  await page.getByLabel('Anordnung AO-2 auswählen').check();
  // The screen shows what the run will move before it is created.
  await expect(page.getByTestId('selected-sum')).toContainText('55,00');
  await page.getByRole('button', { name: 'Zahlungslauf erstellen' }).click();

  await expect(page.getByText('Zahlungslauf erstellt.').first()).toBeVisible();
  await expect(page.getByTestId('run-RUN-1')).toContainText('2 Posten');
  expect(bodies).toEqual([
    { reference: 'RUN-1', account_id: 1, execution_date: '2026-11-01', order_ids: [1, 2] },
  ]);

  await page.getByRole('button', { name: 'Datei anzeigen' }).click();
  await expect(page.getByTestId('run-file-RUN-1')).toContainText('<CstmrCdtTrfInitn/>');

  await page.getByRole('button', { name: 'An Bankkanal übergeben' }).click();
  await expect(page.getByText('Job 77', { exact: false })).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsläufe reports a refused run and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/payment-orders\?status=approved/, (route) => route.fulfill({ json: approved }));
  await page.route('**/api/payment-runs', (route) => route.fulfill({ status: 422, json: { error: 'order not approved' } }));

  await page.goto('/kasse/zahlungslaeufe');
  await page.getByLabel('Referenz').fill('RUN-1');
  await page.getByLabel('Ausführungsdatum').fill('2026-11-01');
  await page.getByLabel('Anordnung AO-1 auswählen').check();
  await page.getByRole('button', { name: 'Zahlungslauf erstellen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte den Zahlungslauf nicht erstellen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Zahlungsläufe states that no approved order exists and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/payment-orders\?status=approved/, (route) => route.fulfill({ json: [] }));

  await page.goto('/kasse/zahlungslaeufe');

  await expect(page.getByText('Es ist keine genehmigte Zahlungsanordnung vorhanden.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
