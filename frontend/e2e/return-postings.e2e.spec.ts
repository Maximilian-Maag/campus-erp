import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const debitReturn = {
  ID: 7,
  LineNo: 1,
  ValueDate: '2026-11-20T00:00:00Z',
  BookingDate: '2026-11-20T00:00:00Z',
  Credit: false,
  Return: true,
  AmountCents: 12500,
  Currency: 'EUR',
  PostingKey: '009',
  ClientRef: 'ANORD-2026-9',
  BankRef: 'BANKREF',
  Purpose: 'Rückläufer Lastschrift',
  EndToEndRef: '',
  CounterpartyName: '',
  CounterpartyIBAN: '',
  CounterpartyBIC: '',
  UnclearedCents: 0,
  Status: 'returned',
  OrderID: null,
};

const creditReturn = {
  ...debitReturn,
  ID: 8,
  LineNo: 2,
  Credit: true,
  AmountCents: 4999,
  PostingKey: '059',
  ClientRef: 'ANORD-2026-4',
  Purpose: 'Rückläufer Zahlung',
};

test('Rückläufer lists the returning lines of a period and queues them for the ledger side, and passes axe', async ({ page }) => {
  const gets: string[] = [];
  const posts: unknown[] = [];
  await page.route('**/api/bank-statement-returns*', (route) => {
    gets.push(route.request().url());
    return route.fulfill({ status: 200, json: [debitReturn, creditReturn] });
  });
  await page.route('**/api/ledger/return-postings', (route) => {
    posts.push(route.request().postDataJSON());
    return route.fulfill({ status: 202, json: { job_id: 42 } });
  });

  await page.goto('/kasse/ruecklaeufer');
  await expect(page.getByRole('heading', { name: 'Rückläufer' })).toBeVisible();

  await page.getByLabel('Von').fill('2026-11-01');
  await page.getByLabel('Bis').fill('2026-11-30');
  await page.getByRole('button', { name: 'Rückläufer laden' }).click();

  // The period is the server's: both days travel as the screen's query.
  await expect(page.getByTestId('return-7')).toContainText('009');
  await expect(page.getByTestId('return-7')).toContainText('125,00');
  await expect(page.getByTestId('return-7')).toContainText('Ausgang');
  await expect(page.getByTestId('return-7')).toContainText('Rückläufer Lastschrift');
  await expect(page.getByTestId('return-8')).toContainText('Eingang');
  await expect(page.getByTestId('return-8')).toContainText('059');
  expect(gets).toHaveLength(1);
  expect(gets[0]).toContain('from=2026-11-01');
  expect(gets[0]).toContain('to=2026-11-30');

  await page.getByRole('button', { name: 'An FiBu übergeben' }).click();
  await expect(page.getByText('Übergabe an die FiBu eingeplant (Auftrag 42).')).toBeVisible();
  expect(posts).toEqual([{ from: '2026-11-01', to: '2026-11-30' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Rückläufer reports a refused period and an empty one, and passes axe', async ({ page }) => {
  let refused = true;
  await page.route('**/api/bank-statement-returns*', (route) =>
    refused
      ? route.fulfill({ status: 422, json: { error: 'the return-postings period end precedes its start' } })
      : route.fulfill({ status: 200, json: [] })
  );
  await page.route('**/api/ledger/return-postings', (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );

  await page.goto('/kasse/ruecklaeufer');
  await page.getByLabel('Von').fill('2026-11-01');
  await page.getByLabel('Bis').fill('2026-11-30');

  await page.getByRole('button', { name: 'Rückläufer laden' }).click();
  await expect(page.getByRole('alert')).toContainText('Die Rückläufer konnten nicht geladen werden.');
  await expect(page.locator('table')).toHaveCount(0);

  refused = false;
  await page.getByRole('button', { name: 'Rückläufer laden' }).click();
  await expect(page.getByText('Für den Zeitraum sind keine Rückläufer eingelesen.')).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);

  // An empty period offers no hand-over: there is nothing to hand over.
  await expect(page.getByRole('button', { name: 'An FiBu übergeben' })).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Rückläufer reports a failed hand-over, and passes axe', async ({ page }) => {
  await page.route('**/api/bank-statement-returns*', (route) =>
    route.fulfill({ status: 200, json: [debitReturn] })
  );
  await page.route('**/api/ledger/return-postings', (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );

  await page.goto('/kasse/ruecklaeufer');
  await page.getByLabel('Von').fill('2026-11-01');
  await page.getByLabel('Bis').fill('2026-11-30');
  await page.getByRole('button', { name: 'Rückläufer laden' }).click();
  await expect(page.getByTestId('return-7')).toBeVisible();

  await page.getByRole('button', { name: 'An FiBu übergeben' }).click();
  await expect(page.getByRole('alert')).toContainText('Die Übergabe konnte nicht eingeplant werden.');
  await expect(page.getByTestId('return-7')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
