import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const personal = { ID: 1, Code: '4711', Name: 'Personalkosten', CreatedBy: 'alice' };

test('Kostenarten shows the catalogue, maps an account and looks one up, and passes axe', async ({ page }) => {
  const accounts = [{ ID: 1, CostTypeID: 1, AccountCode: '6810' }];
  const mapped: unknown[] = [];

  await page.route('**/api/klr/cost-types', (route) => route.fulfill({ json: [personal] }));
  await page.route('**/api/klr/cost-types/1/accounts', (route) => {
    if (route.request().method() === 'POST') {
      mapped.push(route.request().postDataJSON());
      accounts.push({ ID: 2, CostTypeID: 1, AccountCode: '6820' });
      return route.fulfill({ status: 201, json: { ID: 2, CostTypeID: 1, AccountCode: '6820' } });
    }
    return route.fulfill({ json: accounts });
  });
  await page.route('**/api/klr/accounts/6810/cost-type', (route) => route.fulfill({ json: personal }));

  await page.goto('/klr/kostenarten');

  // The catalogue, with the first entry already chosen and its accounts below it.
  await expect(page.getByTestId('cost-type-code-1')).toHaveText('4711');
  await expect(page.getByTestId('cost-type-name-1')).toHaveText('Personalkosten');
  await expect(page.getByTestId('account-6810')).toContainText('6810');

  // Mapping goes to the chosen cost type, with the code trimmed.
  await page.locator('#klr-account-code').fill('  6820  ');
  await page.getByTestId('map-account').click();
  await expect(page.getByText('Konto zugeordnet.').first()).toBeVisible();
  expect(mapped).toEqual([{ account_code: '6820' }]);
  // The list is reloaded, so the new mapping is on screen rather than only on the server.
  await expect(page.getByTestId('account-6820')).toBeVisible();

  // The other direction of the bridge: which cost type an account belongs to.
  await page.locator('#klr-lookup-code').fill('6810');
  await page.getByTestId('lookup-account').click();
  await expect(page.getByTestId('lookup-result')).toContainText('4711');
  await expect(page.getByTestId('lookup-result')).toContainText('Personalkosten');
  await expect(page.getByTestId('lookup-miss')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test('Kostenarten creates the first cost type and reports a refused one, and passes axe', async ({ page }) => {
  const catalogue: unknown[] = [];
  const created: unknown[] = [];

  await page.route('**/api/klr/cost-types', (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      created.push(body);
      if (catalogue.length > 0) {
        return route.fulfill({ status: 409, json: { error: 'code in use' } });
      }
      catalogue.push(personal);
      return route.fulfill({ status: 201, json: personal });
    }
    return route.fulfill({ json: catalogue });
  });
  await page.route('**/api/klr/cost-types/1/accounts', (route) => route.fulfill({ json: [] }));
  await page.route('**/api/klr/accounts/9999/cost-type', (route) =>
    route.fulfill({ status: 404, json: { error: 'no cost type is mapped to that account' } })
  );

  await page.goto('/klr/kostenarten');

  // An empty catalogue must still offer the form: otherwise the first cost type could not be
  // created from the screen at all.
  await expect(page.getByText('keine Kostenarten angelegt').first()).toBeVisible();
  await page.locator('#klr-code').fill('4711');
  await page.locator('#klr-name').fill('Personalkosten');
  await page.getByTestId('create-cost-type').click();
  await expect(page.getByText('Kostenart angelegt.').first()).toBeVisible();
  expect(created).toEqual([{ code: '4711', name: 'Personalkosten' }]);
  await expect(page.getByTestId('cost-type-code-1')).toHaveText('4711');

  // The code is the identity of a cost type, so a second one under the same code is refused.
  await page.locator('#klr-code').fill('4711');
  await page.locator('#klr-name').fill('Doppelt');
  await page.getByTestId('create-cost-type').click();
  await expect(page.getByRole('alert')).toContainText('Konnte die Kostenart nicht anlegen.');

  // An account nobody has mapped is an answer, not a failure.
  await page.locator('#klr-lookup-code').fill('9999');
  await page.getByTestId('lookup-account').click();
  await expect(page.getByTestId('lookup-miss')).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
