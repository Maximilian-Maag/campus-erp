import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const run = (over: Record<string, unknown> = {}) => ({
  at: '2026-11-11T04:12:00Z',
  actor: 'alice',
  summary: 'source=3732 loaded=3732 checksum=abc1234 previous=abc1234 drift=false',
  ...over,
});

test('Importe lists the datasets and the runs and starts one, and passes axe', async ({ page }) => {
  const started: unknown[] = [];
  const queries: string[] = [];
  let runs = [run()];

  await page.route('**/api/etl/datasets', (route) =>
    route.fulfill({ status: 200, json: { datasets: ['bank-leit-zahl', 'krankenkasse'] } })
  );
  await page.route('**/api/etl/runs**', (route) => {
    const request = route.request();
    queries.push(request.url());
    if (request.method() === 'POST') {
      started.push(request.postDataJSON());
      runs = [run({ actor: 'bob', summary: 'source=3732 loaded=3732 checksum=def5678 drift=true' }), ...runs];
      return route.fulfill({
        status: 200,
        json: {
          dataset: 'bank-leit-zahl',
          source: 3732,
          loaded: 3732,
          checksum: 'def5678',
          previous: 'abc1234',
          drift: true,
          at: '2026-11-11T05:00:00Z',
        },
      });
    }
    return route.fulfill({ status: 200, json: { runs } });
  });

  await page.goto('/importe');
  await expect(page.getByTestId('run-0')).toContainText('alice');
  // The history is every dataset's until one is chosen.
  expect(queries[0]).toContain('/api/etl/runs?limit=20');

  // Starting a run needs a dataset: the screen prevents it and says why, rather than sending an
  // empty one and reporting a refusal.
  await expect(page.getByRole('button', { name: 'Datenübernahme starten' })).toBeDisabled();
  await expect(page.getByText('Für eine Datenübernahme ist ein Datensatz zu wählen.')).toBeVisible();
  expect(started).toEqual([]);

  // Choosing one narrows the history and offers the run.
  await page.getByLabel('Datensatz').selectOption('bank-leit-zahl');
  await expect.poll(() => queries.some((q) => q.includes('dataset=bank-leit-zahl'))).toBe(true);

  await page.getByRole('button', { name: 'Datenübernahme starten' }).click();
  await expect(page.getByRole('status')).toContainText('3732 Zeilen gelesen, 3732 übernommen');
  await expect(page.getByRole('status')).toContainText('Die Quelle hat sich seit dem letzten Lauf geändert.');
  expect(started).toEqual([{ dataset: 'bank-leit-zahl' }]);

  // The run is read back into the history rather than assumed.
  await expect(page.getByTestId('run-0')).toContainText('bob');

  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Importe reports an empty installation, and passes axe', async ({ page }) => {
  await page.route('**/api/etl/datasets', (route) => route.fulfill({ status: 200, json: { datasets: [] } }));
  await page.route('**/api/etl/runs**', (route) => route.fulfill({ status: 200, json: { runs: [] } }));

  await page.goto('/importe');
  await expect(page.getByText('Es sind keine Datenübernahmen eingerichtet.')).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('Importe says so when nothing has run yet, and passes axe', async ({ page }) => {
  await page.route('**/api/etl/datasets', (route) =>
    route.fulfill({ status: 200, json: { datasets: ['bank-leit-zahl'] } })
  );
  await page.route('**/api/etl/runs**', (route) => route.fulfill({ status: 200, json: { runs: [] } }));

  await page.goto('/importe');
  await expect(page.getByText('Es ist noch keine Datenübernahme gelaufen.')).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
