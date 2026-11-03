import { defineConfig, devices } from "@playwright/test";

// The chat spec mocks the REST API (e2e/mock-chat-api.ts), so only the Next
// app needs to run. Set E2E_BASE_URL to run against an already running stack.
const external = process.env.E2E_BASE_URL;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: external ?? "http://localhost:3100", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: external
    ? undefined
    : {
        command: "npm run build && npx next start -p 3100",
        url: "http://localhost:3100",
        reuseExistingServer: !process.env.CI,
        timeout: 240_000,
      },
});
