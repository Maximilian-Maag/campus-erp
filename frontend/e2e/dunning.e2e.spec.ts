import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const levels = [
  {
    ID: 1,
    Year: 2026,
    Code: '1',
    Description: '1. Mahnung',
    DaysAllowed: 14,
    Active: true,
    NextCode: '2',
    Bands: [
      { UpToCents: 5000, FeeCents: 500, RateBasisPoints: 0 },
      { UpToCents: 0, FeeCents: 2500, RateBasisPoints: 200 },
    ],
  },
];

const notice = {
  ID: 9,
  ClaimReference: 'F-2026-001',
  LevelCode: '1',
  OutstandingCents: 12000,
  ChargeCents: 2740,
  IssuedOn: '2026-11-05T00:00:00Z',
  IssuedBy: 'alice',
  DocumentKey: 'dunning/9.pdf',
};

test('Mahnwesen lists the levels, defines one and issues a notice, and passes axe', async ({ page }) => {
  const levelBodies: unknown[] = [];
  const noticeBodies: unknown[] = [];
  await page.route('**/api/dunning/levels', (route) => {
    if (route.request().method() === 'POST') {
      levelBodies.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: levels[0] });
    }
    return route.fulfill({ json: levels });
  });
  await page.route(/\/api\/dunning\/levels\?year=/, (route) => route.fulfill({ json: levels }));
  await page.route('**/api/dunning/notices', (route) => {
    noticeBodies.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: notice });
  });

  await page.goto('/kasse/mahnwesen');
  // The level's bands are read as the operator thinks of them.
  await expect(page.getByTestId('level-2026-1')).toContainText('bis 50,00: 5,00 + 0 %');

  // Narrows to one year on the server.
  await page.getByLabel('Jahr', { exact: true }).fill('2026');
  await page.getByLabel('Jahr', { exact: true }).blur();

  await page.getByLabel('Kürzel').fill('2');
  await page.getByLabel('Bezeichnung').fill('2. Mahnung');
  await page.getByLabel('Obergrenze').fill('50,00');
  await page.getByLabel('Gebühr').fill('5,00');
  await page.getByLabel('Zinssatz in Prozent').fill('0,5');
  await page.getByRole('button', { name: 'Mahnstufe anlegen' }).click();

  await expect(page.getByText('Mahnstufe angelegt.').first()).toBeVisible();
  expect(levelBodies).toEqual([
    {
      year: 2026,
      code: '2',
      description: '2. Mahnung',
      days_allowed: 14,
      active: true,
      next_code: '',
      bands: [{ up_to_cents: 5000, fee_cents: 500, rate_basis_points: 50 }],
    },
  ]);

  await page.getByLabel('Vorgangsnummer').fill('F-2026-001');
  await page.getByLabel('Offener Betrag').fill('120,00');
  await page.getByRole('button', { name: 'Mahnung ausstellen' }).click();

  await expect(page.getByText('Mahnung ausgestellt.').first()).toBeVisible();
  await expect(page.getByTestId('notice-9')).toContainText('27,40');
  expect(noticeBodies).toEqual([
    { year: 2026, level_code: '1', claim_reference: 'F-2026-001', outstanding_cents: 12000 },
  ]);
  // The notice carries a stored PDF, so the screen links to it.
  await expect(page.locator('a[href="/api/dunning/notices/9/document"]')).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mahnwesen reports refusals and an empty catalogue, and passes axe', async ({ page }) => {
  await page.route('**/api/dunning/levels', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 422, json: { error: 'bands must ascend' } })
      : route.fulfill({ json: [] })
  );

  await page.goto('/kasse/mahnwesen');
  // No level is on offer, so no notice can be issued against one.
  await expect(page.getByText('Es ist keine Mahnstufe angelegt.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Mahnung ausstellen' })).toHaveCount(0);

  await page.getByLabel('Kürzel').fill('1');
  await page.getByRole('button', { name: 'Mahnstufe anlegen' }).click();
  await expect(page.getByRole('alert')).toContainText('Konnte die Mahnstufe nicht anlegen.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mahnwesen refuses an incomplete notice before asking the API and passes axe', async ({ page }) => {
  await page.route('**/api/dunning/levels', (route) => route.fulfill({ json: levels }));

  await page.goto('/kasse/mahnwesen');
  await page.getByRole('button', { name: 'Mahnung ausstellen' }).click();

  await expect(page.getByRole('alert')).toContainText('Eine Vorgangsnummer ist erforderlich.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
