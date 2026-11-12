import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const stiftung = {
  ID: 1,
  Year: 2026,
  Number: 'STIFT-1',
  Description: 'Deutschlandstipendium',
  CarryOver: false,
};
const older = {
  ID: 2,
  Year: 2025,
  Number: 'STIFT-2',
  Description: 'Stipendium alt',
  CarryOver: true,
};
const created = {
  ID: 3,
  Year: 2026,
  Number: 'STIFT-3',
  Description: 'Stipendium neu',
  CarryOver: true,
};
const book = { ID: 7, FoundationID: 1, Reference: 'B-1', Currency: 'EUR', CreatedBy: 'alice' };
const bookings = [
  { ID: 3, BookID: 7, Kind: 'income', AmountCents: 50000, Purpose: 'Spende', BookedBy: 'alice' },
];
const balance = { BookID: 7, IncomeCents: 50000, ExpenseCents: 7500 };

test('Stiftungen finds a foundation, its books and the movements of a book, and passes axe', async ({ page }) => {
  const foundationBodies: unknown[] = [];
  const bookBodies: unknown[] = [];
  const bookingBodies: unknown[] = [];

  await page.route('**/api/foundations', (route) => {
    if (route.request().method() === 'POST') {
      foundationBodies.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: created });
    }
    return route.fulfill({ json: [stiftung, older] });
  });
  // The year is narrowed by the server, so only the 2025 foundation comes back.
  await page.route(/\/api\/foundations\?year=/, (route) => route.fulfill({ json: [older] }));
  await page.route('**/api/foundations/1/books', (route) => route.fulfill({ json: [book] }));
  await page.route('**/api/foundation-books', (route) => {
    bookBodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: { ...book, ID: 8, Reference: 'B-2' } });
  });
  await page.route('**/api/foundation-books/7/bookings', (route) => {
    if (route.request().method() === 'POST') {
      bookingBodies.push(route.request().postDataJSON());
      return route.fulfill({
        status: 201,
        json: {
          ID: 4,
          BookID: 7,
          Kind: 'expense',
          AmountCents: 12000,
          Purpose: 'Auszahlung',
          BookedBy: 'alice',
        },
      });
    }
    return route.fulfill({ json: bookings });
  });
  // The balance is derived, so it is read again after the booking — and the figure it then
  // reports is the server's, not one the screen worked out.
  let balanceReads = 0;
  await page.route('**/api/foundation-books/7/balance', (route) => {
    balanceReads += 1;
    if (balanceReads === 1) {
      return route.fulfill({ json: balance });
    }
    return route.fulfill({ json: { BookID: 7, IncomeCents: 50000, ExpenseCents: 12000 } });
  });

  await page.goto('/finanzen/stiftungen');
  await expect(page.getByTestId('foundation-1')).toContainText('Deutschlandstipendium');
  await expect(page.getByTestId('foundation-2')).toContainText('ja');

  // A foundation is created, and the list is read again afterwards.
  await page.getByLabel('Haushaltsjahr').fill('2026');
  await page.getByLabel('Nummer').fill('STIFT-3');
  await page.getByLabel('Bezeichnung').fill('Stipendium neu');
  await page.getByLabel('Übertrag möglich').check();
  await page.getByRole('button', { name: 'Stiftung anlegen' }).click();
  await expect(page.getByText('Stiftung angelegt.').first()).toBeVisible();
  expect(foundationBodies).toEqual([
    { year: 2026, number: 'STIFT-3', description: 'Stipendium neu', carry_over: true },
  ]);

  // Its books are read for the foundation that was chosen.
  await page.getByTestId('foundation-1').getByRole('button', { name: 'Bücher anzeigen' }).click();
  await expect(page.getByText('Stiftungsbücher — 2026/STIFT-1')).toBeVisible();
  await expect(page.getByTestId('book-7')).toContainText('B-1');

  // A book is created against that foundation.
  await page.getByLabel('Referenz').fill('B-2');
  await page.getByLabel('Währung').fill('EUR');
  await page.getByRole('button', { name: 'Buch anlegen' }).click();
  await expect(page.getByText('Buch angelegt.').first()).toBeVisible();
  await expect(page.getByTestId('book-8')).toContainText('B-2');
  expect(bookBodies).toEqual([{ foundation_id: 1, reference: 'B-2', currency: 'EUR' }]);

  // A book's movements and its derived balance.
  await page.getByTestId('book-7').getByRole('button', { name: 'Buchungen anzeigen' }).click();
  await expect(page.getByTestId('booking-3')).toContainText('Spende');
  await expect(page.getByTestId('booking-3')).toContainText('Einnahme');
  await expect(page.getByText('Einnahmen 500,00 — Ausgaben 75,00 — Saldo 425,00')).toBeVisible();

  // A movement is booked in euro and joins the movements.
  await page.getByLabel('Art').selectOption('expense');
  await page.getByLabel('Betrag in Euro').fill('120,00');
  await page.getByLabel('Verwendungszweck').fill('Auszahlung');
  await page.getByRole('button', { name: 'Buchung erfassen' }).click();
  await expect(page.getByText('Buchung erfasst.').first()).toBeVisible();
  await expect(page.getByTestId('booking-4')).toContainText('Ausgabe');
  expect(bookingBodies).toEqual([{ kind: 'expense', amount_cents: 12000, purpose: 'Auszahlung' }]);

  // The year narrows the list on the server.
  await page.getByLabel('Jahr', { exact: true }).fill('2025');
  await page.getByRole('button', { name: 'Anzeigen', exact: true }).click();
  await expect(page.getByTestId('foundation-2')).toBeVisible();
  await expect(page.getByTestId('foundation-1')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Stiftungen closes a year into the next and names what moved, and passes axe', async ({ page }) => {
  const carryBodies: unknown[] = [];
  const carrying = { ...stiftung, CarryOver: true };

  await page.route('**/api/foundations', (route) => route.fulfill({ json: [carrying] }));
  await page.route('**/api/foundations/1/books', (route) => route.fulfill({ json: [book] }));
  await page.route('**/api/foundation-books/7/carry-over', (route) => {
    carryBodies.push(route.request().postDataJSON());
    return route.fulfill({
      status: 201,
      json: {
        source_book_id: 7,
        source_reference: 'B-1',
        year: 2026,
        target_book_id: 12,
        target_reference: 'B-1',
        booking_id: 40,
        cents: 42500,
      },
    });
  });

  await page.goto('/finanzen/stiftungen');
  await page.getByTestId('foundation-1').getByRole('button', { name: 'Bücher anzeigen' }).click();
  await expect(page.getByTestId('book-7')).toBeVisible();

  await page.getByTestId('carry-over-7').click();
  await expect(
    page.getByText('425,00 € aus 2026 übertragen — Buch B-1 im Jahr 2027 angelegt.'),
  ).toBeVisible();
  expect(carryBodies).toEqual([{ reference: '' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Stiftungen names the refusal when a year cannot be closed, and passes axe', async ({ page }) => {
  const carrying = { ...stiftung, CarryOver: true };
  await page.route('**/api/foundations', (route) => route.fulfill({ json: [carrying] }));
  await page.route('**/api/foundations/1/books', (route) => route.fulfill({ json: [book] }));
  await page.route('**/api/foundation-books/7/carry-over', (route) =>
    route.fulfill({
      status: 422,
      json: { error: 'kasse: the next year\'s foundation does not exist', code: 'kasse.carry_over_target_missing' },
    })
  );

  await page.goto('/finanzen/stiftungen');
  await page.getByTestId('foundation-1').getByRole('button', { name: 'Bücher anzeigen' }).click();
  await page.getByTestId('carry-over-7').click();

  // The refusal names what to do next, not just that something failed.
  await expect(page.getByText('Für das Folgejahr ist noch keine Stiftung angelegt.')).toBeVisible();
  await expect(page.getByTestId('carry-over-7')).toBeEnabled();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Stiftungen cancels a movement and shows the counter-movement beside it, and passes axe', async ({ page }) => {
  const carryBodies: unknown[] = [];
  const carrying = { ...stiftung, CarryOver: true };
  const movement = { ID: 3, BookID: 7, Kind: 'income', AmountCents: 50000, Purpose: 'Spende (doppelt)', BookedBy: 'alice', Reverses: null, ReversedBy: null };
  const counter = { ID: 12, BookID: 7, Kind: 'expense', AmountCents: 50000, Purpose: 'Storno zu Buchung 3', BookedBy: 'alice', Reverses: 3, ReversedBy: null };
  let reads = 0;

  await page.route('**/api/foundations', (route) => route.fulfill({ json: [carrying] }));
  await page.route('**/api/foundations/1/books', (route) => route.fulfill({ json: [book] }));
  await page.route('**/api/foundation-books/7/bookings', (route) => {
    reads += 1;
    // Before the cancellation the mistake is on its own; after it, the counter-movement is there.
    return route.fulfill({ json: reads === 1 ? [movement] : [{ ...movement, ReversedBy: 12 }, counter] });
  });
  await page.route('**/api/foundation-books/7/balance', (route) =>
    route.fulfill({ json: { BookID: 7, IncomeCents: 50000, ExpenseCents: 50000 } })
  );
  await page.route('**/api/foundation-bookings/3/reverse', (route) => {
    carryBodies.push(route.request().postDataJSON());
    return route.fulfill({
      status: 201,
      json: { original_id: 3, book_id: 7, counter_booking_id: 12, counter_kind: 'expense', amount_cents: 50000, purpose: 'Storno zu Buchung 3' },
    });
  });

  await page.goto('/finanzen/stiftungen');
  await page.getByTestId('foundation-1').getByRole('button', { name: 'Bücher anzeigen' }).click();
  await page.getByTestId('book-7').getByRole('button', { name: 'Buchungen anzeigen' }).click();
  await expect(page.getByTestId('reverse-3')).toBeVisible();

  await page.getByTestId('reverse-3').click();
  await expect(page.getByText('Buchung 3 storniert — Gegenbuchung 12 angelegt.')).toBeVisible();
  // The mistake stays, marked, with its counter-movement named as one.
  await expect(page.getByTestId('reversed-3')).toContainText('storniert');
  await expect(page.getByTestId('counter-12')).toContainText('Gegenbuchung');
  await expect(page.getByTestId('booking-3')).toContainText('Spende (doppelt)');
  expect(carryBodies).toEqual([{ reason: '' }]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Stiftungen refuses an incomplete foundation and reports an unreadable year, and passes axe', async ({ page }) => {
  await page.route('**/api/foundations', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 422, json: { error: 'a number is required' } })
      : route.fulfill({ json: [] })
  );
  await page.route(/\/api\/foundations\?year=/, (route) =>
    route.fulfill({ status: 422, json: { error: 'year must be an integer' } })
  );

  await page.goto('/finanzen/stiftungen');
  await expect(page.getByText('Es ist noch keine Stiftung zu diesem Jahr angelegt.')).toBeVisible();

  // Nothing typed is refused here, not by the API.
  await page.getByRole('button', { name: 'Stiftung anlegen' }).click();
  await expect(page.getByText('Ein Jahr ist erforderlich.')).toBeVisible();

  // A year the server will not read is reported as a failed load, not as an empty year. (The
  // refusal above is still on screen and is about its own form, so the message is named.)
  await page.getByLabel('Jahr', { exact: true }).fill('heuer');
  await page.getByRole('button', { name: 'Anzeigen', exact: true }).click();
  await expect(page.getByText('Konnte die Stiftungen nicht laden.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
