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

const mandates = [
  { ID: 1, Reference: 'MAN-1', DebtorName: 'Max Mustermann', DebtorIBAN: 'DE89370400440532013000', DebtorBIC: '', SignatureDate: '2025-03-14T00:00:00Z', SequenceType: 'RCUR', Active: true },
  { ID: 2, Reference: 'MAN-2', DebtorName: 'Erika Musterfrau', DebtorIBAN: 'DE89370400440532013000', DebtorBIC: '', SignatureDate: '2025-03-14T00:00:00Z', SequenceType: 'RCUR', Active: true },
];

test('Einzüge builds a run from hand-picked mandates and passes axe', async ({ page }) => {
  const bodies: unknown[] = [];
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/direct-debit\/mandates\?active=true/, (route) => route.fulfill({ json: mandates }));
  await page.route('**/api/direct-debit/runs', (route) => {
    bodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: { ...run, ItemCount: 1, SumCents: 4250 } });
  });

  await page.goto('/kasse/einzuege');
  await page.getByLabel('Referenz').fill('RUN-1');
  await page.getByLabel('Gläubiger-ID').fill('DE98ZZZ09999999999');
  await page.getByLabel('Einzugsdatum').fill('2026-11-01');
  // The picker is only offered once the hand-picked source is chosen, and only the mandates the
  // run's sequence type can collect under are loaded.
  await page.getByLabel('Aus gewählten Mandaten').check();
  await page.getByLabel('Mandat', { exact: true }).selectOption('MAN-1');
  await page.getByLabel('Betrag', { exact: true }).fill('42,50');
  await page.getByLabel('Verwendungszweck').fill('Semesterbeitrag');
  await page.getByRole('button', { name: 'Posten hinzufügen' }).click();
  await expect(page.getByTestId('item-MAN-1')).toContainText('42,50');

  await page.getByRole('button', { name: 'Einzug erstellen' }).click();
  await expect(page.getByText('Einzug erstellt.').first()).toBeVisible();
  expect(bodies).toEqual([
    {
      reference: 'RUN-1',
      account_id: 1,
      creditor_id: 'DE98ZZZ09999999999',
      collection_date: '2026-11-01',
      sequence_type: 'RCUR',
      local_instrument: 'CORE',
      items: [{ mandate_reference: 'MAN-1', end_to_end_id: 'MAN-1', amount_cents: 4250, purpose: 'Semesterbeitrag' }],
    },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
