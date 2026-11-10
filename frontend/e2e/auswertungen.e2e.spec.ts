import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const reports = [
  {
    ID: 'kasse-daily-close',
    Title: 'Kassenabschluss',
    Columns: [
      { Key: 'reference', Title: 'Beleg', Align: 'left' },
      { Key: 'amount', Title: 'Betrag', Align: 'right' },
    ],
  },
  {
    ID: 'klr-allocation-overview',
    Title: 'Umlageregeln',
    Columns: [
      { Key: 'source', Title: 'Von Kostenstelle', Align: 'left' },
      { Key: 'share', Title: 'Anteil', Align: 'right' },
    ],
  },
];

test('Auswertungen lists the report catalogue and queues a render for a period, and passes axe', async ({ page }) => {
  const posts: unknown[] = [];
  await page.route('**/api/reports', (route) => route.fulfill({ status: 200, json: reports }));
  await page.route('**/api/reports/renders', (route) => {
    posts.push(route.request().postDataJSON());
    return route.fulfill({ status: 202, json: { job_id: 42 } });
  });

  await page.goto('/auswertungen');
  await expect(page.getByRole('heading', { name: 'Auswertungen' })).toBeVisible();

  await expect(page.getByTestId('report-kasse-daily-close')).toContainText('Kassenabschluss');
  await expect(page.getByTestId('report-klr-allocation-overview')).toContainText(
    'Von Kostenstelle, Anteil'
  );

  // `exact` because the section's own accessible name ("Auswertungen") contains the label's text.
  await page.getByLabel('Auswertung', { exact: true }).selectOption('klr-allocation-overview');
  await page.getByLabel('Von').fill('2026-04-01');
  await page.getByLabel('Bis').fill('2026-04-30');
  await page.getByRole('button', { name: 'Auswertung erzeugen' }).click();

  await expect(page.getByText('Auswertung eingeplant (Auftrag 42).')).toBeVisible();
  await expect(page.getByRole('link', { name: 'klr-allocation-overview-20260401-20260430.pdf' })).toBeVisible();
  expect(posts).toEqual([
    { report_id: 'klr-allocation-overview', from: '2026-04-01', to: '2026-04-30' },
  ]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Auswertungen reports a refused render and an empty catalogue, and passes axe', async ({ page }) => {
  let empty = false;
  await page.route('**/api/reports', (route) =>
    route.fulfill({ status: 200, json: empty ? [] : reports })
  );
  await page.route('**/api/reports/renders', (route) =>
    route.fulfill({ status: 500, json: { error: 'boom' } })
  );

  await page.goto('/auswertungen');
  await page.getByLabel('Auswertung', { exact: true }).selectOption('kasse-daily-close');
  await page.getByLabel('Von').fill('2026-04-01');
  await page.getByLabel('Bis').fill('2026-04-30');
  await page.getByRole('button', { name: 'Auswertung erzeugen' }).click();
  await expect(page.getByRole('alert')).toContainText('Die Auswertung konnte nicht eingeplant werden.');

  // A catalogue with nothing in it offers no form at all.
  empty = true;
  await page.reload();
  await expect(page.getByText('Es ist keine Auswertung eingerichtet.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Auswertung erzeugen' })).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
