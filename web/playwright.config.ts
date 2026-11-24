import { defineConfig, devices } from '@playwright/test'
import { fileURLToPath } from 'node:url'

const binary = process.env.GDY_E2E_BINARY || fileURLToPath(new URL(process.platform === 'win32' ? '../dist/gandengyan.exe' : '../dist/gandengyan', import.meta.url))
export const baseURL = 'http://127.0.0.1:18081'
export const fixtureURL = 'http://127.0.0.1:18082'

export default defineConfig({
  testDir: 'tests/e2e',
  timeout: 60_000,
  expect: { timeout: 6_000 },
  workers: 1,
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    viewport: { width: 1440, height: 900 },
    trace: 'retain-on-failure',
  },
  webServer: [
    { command: '"' + binary + '" -listen 127.0.0.1:18081', url: baseURL + '/healthz', timeout: 30_000 },
    {
      command: 'go test -C ../server ./internal/gateway -run "^TestBrowserFixture$" -count=1',
      env: { GDY_E2E_FIXTURE_ADDR: '127.0.0.1:18082' },
      url: fixtureURL + '/healthz', timeout: 45_000,
    },
  ],
})
