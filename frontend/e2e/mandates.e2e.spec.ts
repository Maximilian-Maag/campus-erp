import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const mandate = {
  ID: 1,
  Reference: 'MND-1',
  DebtorName: 'Erika Mustermann',
  DebtorIBAN: 'DE02120300000000202051',
  DebtorBIC: 'BYLADEM1001',
  SignatureDate: '2026-04-01T00:00:00Z',
  SequenceType: 'RCUR',
  Active: true,
};

test('Mandate lists the mandates, narrows them and registers one, and passes axe', async ({ page }) => {
  const calls: string[] = [];
  const created: unknown[] = [];
  let mandates = [mandate];

  await page.route('**/api/direct-debit/mandates**', (route) => {
    const request = route.request();
    calls.push(request.url());
    if (request.method() === 'POST') {
      created.push(request.postDataJSON());
      mandates = [...mandates, { ...mandate, ID: 2, Reference: 'MND-2', SequenceType: 'FRST' }];
      return route.fulfill({ status: 201, json: { ...mandate, ID: 2, Reference: 'MND-2', SequenceType: 'FRST' } });
    }
    return route.fulfill({ status: 200, json: mandates });
  });

  await page.goto('/kasse/mandate');
  await expect(page.getByTestId('mandate-MND-1')).toContainText('Erika Mustermann');
  await expect(page.getByTestId('sequence-MND-1')).toContainText('wiederkehrend');
  // The first read asks for the active mandates: the filters are the server's.
  expect(calls[0]).toContain('active=true');

  // A filter change is a new question to the server, not a narrowing on screen.
  await page.getByLabel('Sequenztyp').first().selectOption('FRST');
  await expect.poll(() => calls.length).toBeGreaterThan(1);
  expect(calls.at(-1)).toContain('sequence_type=FRST');

  await page.getByLabel('Referenz').fill('MND-2');
  await page.getByLabel('Schuldner').fill('Max Mustermann');
  await page.getByLabel('IBAN').fill('DE02120300000000202051');
  await page.getByLabel('BIC (optional)').fill('BYLADEM1001');
  await page.getByLabel('Unterschriftsdatum').fill('2026-04-02');
  await page.getByLabel('Sequenztyp').last().selectOption('FRST');
  await page.getByRole('button', { name: 'Mandat aufnehmen' }).click();

  await expect(page.getByText('Mandat MND-2 aufgenommen.')).toBeVisible();
  expect(created).toEqual([
    {
      reference: 'MND-2',
      debtor_name: 'Max Mustermann',
      debtor_iban: 'DE02120300000000202051',
      debtor_bic: 'BYLADEM1001',
      signature_date: '2026-04-02',
      sequence_type: 'FRST',
    },
  ]);
  await expect(page.getByTestId('mandate-MND-2')).toBeVisible();

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Mandate asks twice before retiring one, and passes axe', async ({ page }) => {
  const retires: string[] = [];
  let mandates = [mandate, { ...mandate, ID: 2, Reference: 'MND-2' }];

  await page.route('**/api/direct-debit/mandates**', (route) => {
    const request = route.request();
    if (request.method() === 'POST') {
      const reference = request.url().split('/').at(-2)!;
      retires.push(reference);
      mandates = mandates.map((m) => (m.Reference === reference ? { ...m, Active: false } : m));
      return route.fulfill({ status: 200, json: { ...mandate, Reference: reference, Active: false } });
    }
    return route.fulfill({ status: 200, json: mandates });
  });

  await page.goto('/kasse/mandate');
  await expect(page.getByTestId('mandate-MND-1')).toContainText('aktiv');

  // The first click asks; nothing is sent.
  await page.getByRole('button', { name: 'Mandat MND-1 zurückziehen' }).click();
  await expect(page.getByRole('button', { name: 'Mandat MND-1 wirklich zurückziehen' })).toBeVisible();
  expect(retires).toEqual([]);

  // Abandoning it leaves the mandate alone.
  await page.getByRole('button', { name: 'Abbrechen' }).click();
  await expect(page.getByRole('button', { name: 'Mandat MND-1 zurückziehen' })).toBeVisible();
  expect(retires).toEqual([]);

  // Answering the question revokes it, and the row says so without a reload.
  await page.getByRole('button', { name: 'Mandat MND-1 zurückziehen' }).click();
  await page.getByRole('button', { name: 'Mandat MND-1 wirklich zurückziehen' }).click();
  await expect(page.getByText('Mandat MND-1 zurückgezogen.')).toBeVisible();
  await expect(page.getByTestId('mandate-MND-1')).toContainText('zurückgezogen');
  expect(retires).toEqual(['MND-1']);

  // The retired mandate no longer offers the action; the other one still does.
  await expect(page.getByRole('button', { name: 'Mandat MND-1 zurückziehen' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Mandat MND-2 zurückziehen' })).toBeVisible();

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Mandate refuses an incomplete mandate and reports a refused one, and passes axe', async ({ page }) => {
  await page.route('**/api/direct-debit/mandates**', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 422, json: { error: 'invalid IBAN' } })
      : route.fulfill({ status: 200, json: [] })
  );

  await page.goto('/kasse/mandate');
  await expect(page.getByText('Es sind keine Mandate erfasst.')).toBeVisible();

  // Nothing filled in: the screen refuses before asking.
  await page.getByRole('button', { name: 'Mandat aufnehmen' }).click();
  await expect(page.getByRole('alert')).toContainText('Referenz, Schuldner, IBAN und Unterschriftsdatum sind erforderlich.');

  // Everything but the IBAN's checksum, which the server owns.
  await page.getByLabel('Referenz').fill('MND-9');
  await page.getByLabel('Schuldner').fill('Max Mustermann');
  await page.getByLabel('IBAN').fill('DE02120300000000202099');
  await page.getByLabel('Unterschriftsdatum').fill('2026-04-02');
  await page.getByRole('button', { name: 'Mandat aufnehmen' }).click();
  await expect(page.getByRole('alert')).toContainText('Das Mandat konnte nicht aufgenommen werden.');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
