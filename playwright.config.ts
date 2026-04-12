import { defineConfig, devices } from '@playwright/test';
import path from 'path';

const PORT = 9753;
const notesDir = path.resolve(__dirname, 'tests/fixtures/notes');

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: `http://localhost:${PORT}`,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `go run ./cmd/notesview serve --path "${notesDir}"`,
    url: `http://localhost:${PORT}`,
    reuseExistingServer: !process.env.CI,
    env: {
      NOTESVIEW_PORT: String(PORT),
    },
  },
});
