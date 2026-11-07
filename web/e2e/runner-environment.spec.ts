import { expect, test } from "@playwright/test";
import type { Schemas } from "../lib/api/client";
import { installChatApi } from "./mock-chat-api";

const runId = "00000000-0000-4000-8000-000000000101";
const otherId = "00000000-0000-4000-8000-000000000102";
const at = "2026-11-06T22:00:00Z";
const snapshot: Schemas["EnvironmentSnapshot"] = {
  run_id: runId, runner_id: "mac-local", backend: "local", recorded_at: at,
  ticket_key: "WEB-1", project_key: "WEB", project_name: "Web app",
  report: {
    status: "ready", manifest: ".lazyagents-deps.json", go_modules: ["server"], npm_projects: ["web"], playwright: ["web"],
    tools: [{ name: "go", available: true, version: "go version go1.26.0 darwin/arm64" }, { name: "node", available: true, version: "v24.21.0" }],
    caches: [{ name: "npm downloads", path: "/runner/cache/npm", shared: true, exists: true, writable: true },
      { name: "Go build", path: "/workspace/.lazyagents/cache/go-build", shared: false, exists: false, writable: false }],
    packages: [{ name: "next", version: "16.4.3", ecosystem: "npm", project: "web" }],
  },
};
const environment: Schemas["RunnerEnvironment"] = {
  runner: { online: true, runners: [{ id: "mac-local", backend: "local", heartbeat_at: at, max_concurrent: 2, running: 1 }], slots: 2, busy: 1, queued: 0 },
  snapshots: [snapshot, { ...snapshot, run_id: otherId, runner_id: "docker-offline", backend: "docker", report: { ...snapshot.report, status: "failed", error: "missing required tool: sqlc", tools: [{ name: "sqlc", available: false }] } }],
};

test("environment shows measured tools, project libraries, caches and setup failures", async ({ page }) => {
  await installChatApi(page);
  await page.route("**/api/v1/runner/environment*", (route) => route.fulfill({ json: environment }));
  await page.goto("/settings");
  await page.getByRole("link", { name: "View runner environment" }).click();
  await expect(page.getByRole("heading", { name: "Runner environment", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "go version go1.26.0 darwin/arm64" })).toBeVisible();
  await expect(page.getByText("/runner/cache/npm", { exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "16.4.3" })).toBeVisible();
  await expect(page.getByText("Not created yet", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "WEB-1 run log" })).toHaveAttribute("href", `/runs/${runId}`);
  await page.screenshot({ path: "test-results/runner-environment-desktop.png", fullPage: true });
  await page.getByLabel("Project and runner").selectOption(otherId);
  await expect(page.getByRole("alert").filter({ hasText: "Dependency setup failed" })).toContainText("missing required tool: sqlc");
  await expect(page.getByText("Runner offline", { exact: true })).toBeVisible();
  await expect(page.getByRole("rowheader", { name: "sqlc" })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/runner-environment-mobile.png", fullPage: true });
});

test("exact-run environment keeps its query and handles older runs without reports", async ({ page }) => {
  await installChatApi(page);
  let requested = "";
  await page.route("**/api/v1/runner/environment*", (route) => {
    requested = new URL(route.request().url()).searchParams.get("run_id") ?? "";
    return route.fulfill({ json: { ...environment, snapshots: [] } });
  });
  await page.goto(`/settings/runner?run_id=${runId}`);
  await expect(page.getByRole("heading", { name: "No environment recorded for this run" })).toBeVisible();
  expect(requested).toBe(runId);
  await expect(page.getByRole("link", { name: "View latest environments" })).toHaveAttribute("href", "/settings/runner");
});

test("environment request errors can be retried", async ({ page }) => {
  await installChatApi(page);
  let recover = false;
  await page.route("**/api/v1/runner/environment*", (route) => recover
    ? route.fulfill({ json: environment })
    : route.fulfill({ status: 503, json: { code: "internal", message: "Runner service unavailable" } }));
  await page.goto("/settings/runner");
  await expect(page.getByRole("alert").filter({ hasText: "Couldn’t load" })).toContainText("Runner service unavailable", { timeout: 15_000 });
  recover = true;
  await page.getByRole("button", { name: "Try again" }).click();
  await expect(page.getByRole("cell", { name: "v24.21.0" })).toBeVisible();
});
