import { defineConfig } from '@playwright/test'

const baseURL = 'http://127.0.0.1:41767'
const engines = ['chromium', 'firefox', 'webkit'] as const

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 2,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL,
    locale: 'fr-FR',
    timezoneId: 'UTC',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: engines.flatMap((browserName) => [
    {
      name: `${browserName}-desktop`,
      use: { browserName, viewport: { width: 1280, height: 900 } },
    },
    {
      name: `${browserName}-mobile`,
      use: {
        browserName,
        viewport: { width: 390, height: 844 },
        hasTouch: true,
        // Firefox reste un petit viewport tactile : pas une validation Firefox Android réelle.
        isMobile: browserName !== 'firefox',
      },
    },
  ]),
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 41767 --strictPort',
    url: baseURL,
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
