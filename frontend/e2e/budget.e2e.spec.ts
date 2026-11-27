import { test, expect } from './session';
import AxeBuilder from '@axe-core/playwright';

const positions = [
  {
    position: {
      ID: 1,
      Year: 2026,
      Chapter: '0710',
      Title: '6810',
      Description: 'Beschaffung',
      AllocatedCents: 100000,
      CreatedBy: 'alice',
    },
    balance: { PositionID: 1, AllocatedCents: 100000, CommittedCents: 25000, ActualCents: 5000 },
    available_cents: 70000,
  },
];

test('Mittelübersicht shows the budget and passes axe', async ({ page }) => {
  await page.route(/\/api\/budget\/positions(\?.*)?$/, (route) => route.fulfill({ json: positions }));
  await page.goto('/budget/haushaltsstellen');

  await expect(page.getByTestId('position-1')).toContainText('Beschaffung');
  await expect(page.getByTestId('available-1')).toContainText('700,00');

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mittelübersicht states an empty budget and passes axe', async ({ page }) => {
  await page.route(/\/api\/budget\/positions(\?.*)?$/, (route) => route.fulfill({ json: [] }));
  await page.goto('/budget/haushaltsstellen');

  await expect(page.getByText('Es sind keine Haushaltsstellen angelegt.')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mittelübersicht reports a failed load and passes axe', async ({ page }) => {
  await page.route(/\/api\/budget\/positions(\?.*)?$/, (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );
  await page.goto('/budget/haushaltsstellen');

  await expect(page.getByRole('alert')).toContainText('Konnte die Mittelübersicht nicht laden.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mittelübersicht reads the positions one page at a time, and passes axe', async ({ page }) => {
  const offsets: string[] = [];
  const row = (id: number) => ({
    position: {
      ID: id,
      Year: 2026,
      Chapter: '0710',
      Title: String(id),
      Description: `Stelle ${id}`,
      AllocatedCents: 100000,
      CreatedBy: 'alice',
    },
    balance: { PositionID: id, AllocatedCents: 100000, CommittedCents: 0, ActualCents: 0 },
    available_cents: 100000,
  });

  await page.route(/\/api\/budget\/positions(\?.*)?$/, (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('offset') ?? '0');
    offsets.push(String(offset));
    // The first page is full, so the screen offers the next one; the second is short, so it stops.
    return route.fulfill({
      json: offset === 0 ? Array.from({ length: 50 }, (_, i) => row(i + 1)) : [row(51)],
    });
  });

  await page.goto('/budget/haushaltsstellen');
  await expect(page.getByTestId('position-1')).toBeVisible();
  await expect(page.getByTestId('position-50')).toBeVisible();
  expect(offsets).toEqual(['0']);

  await page.getByTestId('load-more').click();
  await expect(page.getByTestId('position-51')).toBeVisible();
  expect(offsets).toEqual(['0', '50']);
  // A short page ends the budget, so the offer is gone.
  await expect(page.getByTestId('load-more')).toHaveCount(0);

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
