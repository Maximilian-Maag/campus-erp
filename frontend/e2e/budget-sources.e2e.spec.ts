import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const source = {
  ID: 1,
  Code: 'LHH',
  Name: 'Landesmittel',
  Chapter: '05',
  Active: true,
  CreatedBy: 'alice',
};

const center = { ID: 2, Code: 'BW-1', Name: 'Fakultät 1', Active: true, CreatedBy: 'alice' };

const position = {
  ID: 7,
  Year: 2026,
  Chapter: '05',
  Title: '1234',
  Description: 'Beschaffung',
  AllocatedCents: 100000,
  CreatedBy: 'alice',
  SourceID: null as number | null,
  CenterID: null as number | null,
};

const positionRow = {
  position,
  balance: { PositionID: 7, AllocatedCents: 100000, CommittedCents: 0, ActualCents: 0 },
  available_cents: 100000,
};

test('Mittelherkunft creates a source and a centre and names a position, and passes axe', async ({ page }) => {
  const created: unknown[] = [];
  const assignments: unknown[] = [];
  await page.route('**/api/budget/sources', (route) =>
    route.request().method() === 'POST'
      ? (created.push(route.request().postDataJSON()),
        route.fulfill({ status: 201, json: { ...source, ID: 3, Code: 'S-1', Name: 'Stipendien', Chapter: '' } }))
      : route.fulfill({ status: 200, json: [source] })
  );
  await page.route('**/api/budget/centers', (route) =>
    route.request().method() === 'POST'
      ? (created.push(route.request().postDataJSON()),
        route.fulfill({ status: 201, json: { ...center, ID: 4, Code: 'BW-2', Name: 'Fakultät 2' } }))
      : route.fulfill({ status: 200, json: [center] })
  );
  await page.route('**/api/budget/positions', (route) =>
    route.fulfill({ status: 200, json: [positionRow] })
  );
  await page.route('**/api/budget/positions/*/assignment', (route) => {
    assignments.push(route.request().postDataJSON());
    return route.fulfill({ status: 200, json: { ...position, SourceID: 1, CenterID: 2 } });
  });

  await page.goto('/budget/mittelherkunft');
  await expect(page.getByRole('heading', { name: 'Mittelherkunft und Bewirtschaftung' })).toBeVisible();
  await expect(page.getByTestId('source-1')).toContainText('Landesmittel');
  await expect(page.getByTestId('center-2')).toContainText('Fakultät 1');

  await page.getByLabel('Schlüssel', { exact: true }).first().fill('S-1');
  await page.getByLabel('Bezeichnung', { exact: true }).first().fill('Stipendien');
  await page.getByRole('button', { name: 'Mittelquelle anlegen' }).click();
  await expect(page.getByText('Mittelquelle S-1 angelegt.')).toBeVisible();

  // The second "Schlüssel" is the centre form's.
  await page.getByLabel('Schlüssel', { exact: true }).nth(1).fill('BW-2');
  await page.getByLabel('Bezeichnung', { exact: true }).nth(1).fill('Fakultät 2');
  await page.getByRole('button', { name: 'Bewirtschaftungsstelle anlegen' }).click();
  await expect(page.getByText('Bewirtschaftungsstelle BW-2 angelegt.')).toBeVisible();

  expect(created).toEqual([
    { code: 'S-1', name: 'Stipendien', chapter: '' },
    { code: 'BW-2', name: 'Fakultät 2' },
  ]);

  await page.getByLabel('Haushaltsstelle').selectOption('7');
  await page.getByLabel('Mittelquelle').selectOption('1');
  await page.getByLabel('Bewirtschaftungsstelle', { exact: true }).selectOption('2');
  await page.getByRole('button', { name: 'Zuordnung speichern' }).click();

  await expect(page.getByText('Die Zuordnung ist gespeichert.')).toBeVisible();
  expect(assignments).toEqual([{ source_id: 1, center_id: 2 }]);
  await expect(page.getByTestId('position-7')).toContainText('LHH — Landesmittel');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mittelherkunft reads a budget file and states what it did, and passes axe', async ({ page }) => {
  const imports: unknown[] = [];
  await page.route('**/api/budget/imports', (route) => {
    imports.push(route.request().postDataJSON());
    return route.fulfill({
      status: 201,
      json: {
        Layout: 'budget-import/v1',
        SourcesCreated: 1,
        SourcesKept: 0,
        CentersCreated: 1,
        CentersKept: 1,
        PositionsCreated: 2,
        PositionsKept: 0,
        PositionsNamed: 2,
      },
    });
  });
  await page.route('**/api/budget/sources', (route) => route.fulfill({ status: 200, json: [source] }));
  await page.route('**/api/budget/centers', (route) => route.fulfill({ status: 200, json: [center] }));
  await page.route('**/api/budget/positions', (route) =>
    route.fulfill({ status: 200, json: [positionRow] })
  );

  await page.goto('/budget/mittelherkunft');
  const file = 'source_code;source_name;source_chapter;center_code;center_name;year;chapter;title;description;ansatz\nLHH;Landesmittel;05;BW-1;Fakultaet 1;2026;05;1234;Beschaffung;10.000,00\n';

  // An empty textarea is refused before the API is asked.
  await page.getByRole('button', { name: 'Einlesen' }).click();
  await expect(page.getByRole('alert')).toContainText('Es ist keine Budgetdatei eingelesen.');

  await page.getByLabel('Inhalt der Budgetdatei').fill(file);
  await page.getByRole('button', { name: 'Einlesen' }).click();

  await expect(page.getByRole('status')).toContainText('1 Mittelquellen, 1 Bewirtschaftungsstellen und 2 Haushaltsstellen angelegt');
  // The format field is left empty, which the API reads as the standard layout.
  expect(imports).toEqual([{ layout: '', content: file }]);
  // The file is cleared for the next one.
  await expect(page.getByLabel('Inhalt der Budgetdatei')).toHaveValue('');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Mittelherkunft reports a refused create and an empty budget, and passes axe', async ({ page }) => {
  await page.route('**/api/budget/sources', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({ status: 409, json: { error: 'a source with that code already exists' } })
      : route.fulfill({ status: 200, json: [] })
  );
  await page.route('**/api/budget/centers', (route) => route.fulfill({ status: 200, json: [] }));
  await page.route('**/api/budget/positions', (route) => route.fulfill({ status: 200, json: [] }));

  await page.goto('/budget/mittelherkunft');
  await expect(page.getByText('Es ist keine Mittelquelle angelegt.')).toBeVisible();
  await expect(page.getByText('Es ist keine Haushaltsstelle angelegt.')).toBeVisible();

  await page.getByLabel('Schlüssel', { exact: true }).first().fill('LHH');
  await page.getByLabel('Bezeichnung', { exact: true }).first().fill('Nochmal');
  await page.getByRole('button', { name: 'Mittelquelle anlegen' }).click();
  await expect(page.getByRole('alert')).toContainText('Die Mittelquelle konnte nicht angelegt werden.');

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
