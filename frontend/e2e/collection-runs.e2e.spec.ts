import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const accounts = [
  { ID: 1, Name: 'Universität', IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR' },
];

const run = {
  ID: 1,
  Reference: 'RUN-1',
  AccountID: 1,
  CreditorID: 'DE98ZZZ09999999999',
  CollectionDate: '2026-11-01T00:00:00Z',
  SequenceType: 'RCUR',
  LocalInstrument: 'CORE',
  MessageID: 'RUN-1',
  ItemCount: 2,
  SumCents: 5500,
  FileHash: 'h',
  CreatedBy: 'carol',
};

test('Einzüge creates a run from the open items and shows the file, and passes axe', async ({ page }) => {
  const bodies: unknown[] = [];
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route('**/api/direct-debit/runs/from-receivables', (route) => {
    bodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: run });
  });
  await page.route('**/api/direct-debit/runs/RUN-1/file', (route) =>
    route.fulfill({ contentType: 'application/xml', body: '<Document><CstmrDrctDbtInitn/></Document>' })
  );

  await page.goto('/kasse/einzuege');
  await page.getByLabel('Referenz').fill('RUN-1');
  await page.getByLabel('Gläubiger-ID').fill('DE98ZZZ09999999999');
  await page.getByLabel('Einzugsdatum').fill('2026-11-01');
  await page.getByLabel('Fälligkeit bis').fill('2026-10-15');
  await page.getByRole('button', { name: 'Einzug erstellen' }).click();

  await expect(page.getByText('Einzug erstellt.').first()).toBeVisible();
  await expect(page.getByTestId('run-RUN-1')).toContainText('2 Posten');
  await expect(page.getByTestId('run-RUN-1')).toContainText('55,00');
  expect(bodies).toEqual([
    { reference: 'RUN-1', account_id: 1, creditor_id: 'DE98ZZZ09999999999', collection_date: '2026-11-01', sequence_type: 'RCUR', local_instrument: 'CORE', due_by: '2026-10-15' },
  ]);

  await page.getByRole('button', { name: 'Datei anzeigen' }).click();
  await expect(page.getByTestId('run-file-RUN-1')).toContainText('<CstmrDrctDbtInitn/>');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Einzüge reports a refused run and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route('**/api/direct-debit/runs/from-receivables', (route) =>
    route.fulfill({ status: 422, json: { error: 'nothing due' } })
  );

  await page.goto('/kasse/einzuege');
  await page.getByLabel('Referenz').fill('RUN-1');
  await page.getByLabel('Gläubiger-ID').fill('DE98ZZZ09999999999');
  await page.getByLabel('Einzugsdatum').fill('2026-11-01');
  await page.getByLabel('Fälligkeit bis').fill('2026-10-15');
  await page.getByRole('button', { name: 'Einzug erstellen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte den Einzug nicht erstellen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Einzüge states that no own account exists and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: [] }));
  await page.goto('/kasse/einzuege');

  await expect(page.getByText('Es ist kein eigenes Konto angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
