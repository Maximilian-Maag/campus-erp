import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const accounts = [
  { ID: 1, Name: 'Universität', IBAN: 'DE02120300000000202051', BIC: 'BYLADEM1001', Currency: 'EUR' },
  { ID: 2, Name: 'Zweitkonto', IBAN: 'DE02500105170137075030', BIC: 'BYLADEM1001', Currency: 'EUR' },
];

const statement = {
  ID: 1,
  AccountID: 1,
  Reference: 'STMT-1',
  Number: 1,
  Page: 1,
  Currency: 'EUR',
  OpeningBalanceCents: 1000000,
  OpeningDate: '2026-10-01T00:00:00Z',
  ClosingBalanceCents: 1000000,
  ClosingDate: '2026-10-31T00:00:00Z',
  LineCount: 1,
  ImportedBy: 'carol',
};

const line = {
  ID: 9, LineNo: 1, ValueDate: '2026-10-05T00:00:00Z', BookingDate: '2026-10-05T00:00:00Z',
  Credit: false, Return: false, AmountCents: 125000, Currency: 'EUR', PostingKey: '051',
  ClientRef: 'X', BankRef: 'B', Purpose: 'Zahlung', EndToEndRef: '', CounterpartyName: 'Stadtwerke',
  CounterpartyIBAN: 'DE02120300000000202051', CounterpartyBIC: 'BYLADEM1001',
  UnclearedCents: 0, Status: 'matched', OrderID: 1,
};

test('Kontoauszüge lists the statements of an account and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => route.fulfill({ json: [statement] }));
  await page.goto('/kasse/kontoauszuege');

  await expect(page.getByTestId('statement-STMT-1')).toContainText('STMT-1');
  await expect(page.getByTestId('statement-STMT-1')).toContainText('31.10.2026');
  await expect(page.getByTestId('statement-STMT-1')).toContainText('10.000,00');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge opens a statement and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => route.fulfill({ json: [statement] }));
  await page.route(/\/api\/bank-statements\/1$/, (route) =>
    route.fulfill({ json: { ...statement, Lines: [line] } })
  );

  await page.goto('/kasse/kontoauszuege');
  await page.getByRole('button', { name: 'Zeilen von Auszug STMT-1 anzeigen' }).click();

  await expect(page.getByTestId('lines-STMT-1')).toBeVisible();
  await expect(page.getByTestId('line-1')).toContainText('Zahlung');
  await expect(page.getByTestId('line-1')).toContainText('1.250,00');
  await expect(page.getByTestId('line-1')).toContainText('matched');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge asks the server for the chosen account and passes axe', async ({ page }) => {
  const seen: string[] = [];
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => {
    const url = new URL(route.request().url());
    seen.push(url.searchParams.get('account_id') ?? '');
    const accountID = Number(url.searchParams.get('account_id'));
    return route.fulfill({ json: accountID === 1 ? [statement] : [] });
  });

  await page.goto('/kasse/kontoauszuege');
  await expect(page.getByTestId('statement-STMT-1')).toBeVisible();

  await page.getByLabel('Konto', { exact: true }).selectOption('2');
  await expect(page.getByText('Für dieses Konto ist kein Auszug eingelesen.')).toBeVisible();

  // The filter really travelled to the server.
  expect(seen).toContain('2');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge reads a statement in and passes axe', async ({ page }) => {
  let stored: (typeof statement)[] = [];
  const bodies: unknown[] = [];
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => route.fulfill({ json: stored }));
  await page.route('**/api/bank-statements', (route) => {
    if (route.request().method() === 'POST') {
      bodies.push(route.request().postDataJSON());
      stored = [statement];
      return route.fulfill({ status: 201, json: [statement] });
    }
    return route.fulfill({ json: stored });
  });

  await page.goto('/kasse/kontoauszuege');
  await expect(page.getByText('Für dieses Konto ist kein Auszug eingelesen.')).toBeVisible();

  await page.getByLabel('Inhalt der MT940-Datei').fill(':20:STMT-1\n:25:DE02120300000000202051\n:62F:C261031EUR10000,00\n-');
  await page.getByRole('button', { name: 'Einlesen' }).click();

  await expect(page.getByText('Ein Auszug eingelesen.', { exact: true })).toBeVisible();
  await expect(page.getByTestId('statement-STMT-1')).toBeVisible();
  expect(bodies).toEqual([{
    account_id: 1,
    content: ':20:STMT-1\n:25:DE02120300000000202051\n:62F:C261031EUR10000,00\n-',
  }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge reports a refused import and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => route.fulfill({ json: [] }));
  await page.route('**/api/bank-statements', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 422, json: { error: 'invalid statement' } });
    }
    return route.fulfill({ json: [] });
  });

  await page.goto('/kasse/kontoauszuege');
  await page.getByLabel('Inhalt der MT940-Datei').fill(':20:BAD\n');
  await page.getByRole('button', { name: 'Einlesen' }).click();

  await expect(page.getByRole('alert')).toContainText('Konnte den Auszug nicht einlesen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge states that no own account exists and passes axe', async ({ page }) => {
  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: [] }));
  await page.goto('/kasse/kontoauszuege');

  await expect(page.getByText('Es ist kein eigenes Konto angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kontoauszüge reads the statements one page at a time, and passes axe', async ({ page }) => {
  const offsets: string[] = [];
  const page50 = Array.from({ length: 50 }, (_, i) => ({
    ...statement,
    ID: i + 1,
    Reference: `STMT-${i + 1}`,
  }));

  await page.route('**/api/cash-accounts', (route) => route.fulfill({ json: accounts }));
  await page.route(/\/api\/bank-statements\?.*$/, (route) => {
    const url = new URL(route.request().url());
    offsets.push(url.searchParams.get('offset') ?? '');
    // The first page is full, so the screen offers the next one; the second is short, so it stops.
    const offset = Number(url.searchParams.get('offset') ?? '0');
    return route.fulfill({ json: offset === 0 ? page50 : [{ ...statement, ID: 51, Reference: 'STMT-51' }] });
  });

  await page.goto('/kasse/kontoauszuege');
  await expect(page.getByTestId('statement-STMT-1')).toBeVisible();
  await expect(page.getByTestId('statement-STMT-50')).toBeVisible();
  expect(offsets).toEqual(['0']);

  await page.getByTestId('load-more').click();
  await expect(page.getByTestId('statement-STMT-51')).toBeVisible();
  // The pages continue rather than repeat, and what was read is still on screen.
  expect(offsets).toEqual(['0', '50']);
  await expect(page.getByTestId('statement-STMT-1')).toBeVisible();
  await expect(page.getByTestId('load-more')).toHaveCount(0);

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
