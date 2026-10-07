import { defineConfig, devices } from '@playwright/test';
import path from 'node:path';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 30000,
  expect: { timeout: 8000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: { baseURL: 'http://127.0.0.1:18080', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 960 } } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],
  webServer: {
    command: '"../dist/HomeClipboardCanvas.exe" serve --dev-http --bind 127.0.0.1 --port 18080',
    url: 'http://127.0.0.1:18080/health',
    reuseExistingServer: false,
    timeout: 20000,
    gracefulShutdown: { signal: 'SIGTERM', timeout: 2000 },
  },
  outputDir: path.join('test-results'),
});
