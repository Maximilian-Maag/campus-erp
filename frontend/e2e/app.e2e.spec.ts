import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('shell renders and is accessible', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('app-root')).toBeVisible();

  // BITV 2.0 / WCAG is a legal requirement (ADR-0003, CR-7): no axe violations.
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
