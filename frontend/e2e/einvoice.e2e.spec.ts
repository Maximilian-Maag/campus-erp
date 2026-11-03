import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const summary = (ID: number, Number: string, matched: number) => ({
  ID,
  Number,
  IssueDate: '2026-04-01T00:00:00Z',
  TypeCode: '380',
  Currency: 'EUR',
  BuyerReference: 'PO-2026-1',
  GrossCents: 35700,
  MatchedOrderID: matched,
});

test('Eingangsrechnungen lists the invoices, matches one and passes axe', async ({ page }) => {
  let invoices = [summary(1, 'RE-1', 0), summary(2, 'RE-2', 7)];
  await page.route('**/api/e-invoices', (route) => route.fulfill({ json: invoices }));
  await page.route('**/api/e-invoices/1/match', (route) => {
    invoices = [summary(1, 'RE-1', 7), invoices[1]];
    return route.fulfill({ json: { Number: 'RE-1' } });
  });

  await page.goto('/einvoice/rechnungen');
  await expect(page.getByTestId('match-RE-1')).toContainText('offen');
  await expect(page.getByTestId('invoice-RE-2')).toContainText('Bestellung 7');

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);

  await page.getByRole('button', { name: 'Rechnung RE-1 der Bestellung zuordnen' }).click();
  await expect(page.getByTestId('match-RE-1')).toContainText('Bestellung 7');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Eingangsrechnungen reads a document in and passes axe', async ({ page }) => {
  let invoices: unknown[] = [];
  await page.route('**/api/e-invoices', (route) => route.fulfill({ json: invoices }));
  await page.route('**/api/e-invoices/received', (route) => {
    invoices = [summary(1, 'RE-1', 0)];
    return route.fulfill({ status: 201, json: { Number: 'RE-1' } });
  });

  await page.goto('/einvoice/rechnungen');
  await expect(page.getByText('Es sind keine Rechnungen erfasst.')).toBeVisible();

  await page.getByLabel('XRechnung (XML)').fill('<ubl:Invoice/>');
  await page.getByRole('button', { name: 'Rechnung einlesen' }).click();
  await expect(page.getByTestId('invoice-RE-1')).toBeVisible();

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Eingangsrechnungen reports refusals and passes axe', async ({ page }) => {
  await page.route('**/api/e-invoices', (route) => route.fulfill({ status: 500, json: { error: 'boom' } }));
  await page.goto('/einvoice/rechnungen');

  await expect(page.getByRole('alert')).toContainText('Konnte die Rechnungen nicht laden.');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
