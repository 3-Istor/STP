import { defineConfig } from '@playwright/test'
import baseline from './playwright.config'

export default defineConfig({
  ...baseline,
  testMatch: 'pricing.live.spec.ts',
  testIgnore: [],
  fullyParallel: false,
  workers: 1,
  use: { ...baseline.use, baseURL: 'http://127.0.0.1:41768' },
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 41768 --strictPort',
    url: 'http://127.0.0.1:41768',
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
