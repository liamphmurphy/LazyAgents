import { defineConfig, devices } from "@playwright/test";

// Real-stack suite (no page.route mocks): run it through e2e-stack/run.sh,
// which starts Postgres, the Go API, a local runner and Next, then sets
// E2E_STACK_BASE_URL. Fake CLIs stand in for opencode/claude/codex; see
// e2e-stack/README.md. Chats share one workspace, so tests run serially.
const baseURL = process.env.E2E_STACK_BASE_URL;
if (!baseURL) {
  throw new Error("E2E_STACK_BASE_URL is not set; run `npm run e2e:stack` (e2e-stack/run.sh)");
}

export default defineConfig({
  testDir: "./e2e-stack",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 60_000,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: { baseURL, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
