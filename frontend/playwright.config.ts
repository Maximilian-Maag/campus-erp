import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env['CI'],
  retries: process.env['CI'] ? 1 : 0,
  reporter: process.env['CI'] ? 'github' : 'list',
  use: {
    baseURL: 'http://localhost:4200',
    trace: 'on-first-retry',
    // Everything runs as a user who asked their system for less motion. It is the honest
    // default for a suite that asserts accessibility — the design system honours the preference
    // (`_base.scss`), so a page that needs an animation to become legible fails here — and it
    // keeps axe from measuring a transition: a filled button half-way through its 120ms
    // background fade reads as white-on-pale-blue and fails the contrast check at random.
    reducedMotion: 'reduce'
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'npx ng serve --port 4200',
    url: 'http://localhost:4200',
    reuseExistingServer: !process.env['CI'],
    timeout: 120_000
  }
});
