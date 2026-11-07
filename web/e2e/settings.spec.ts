import { test, expect } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

test("settings save and replace write-only secrets and show runner configuration", async ({ page }) => {
  await installChatApi(page);
  const at = "2026-11-07T00:00:00Z";
  let saved = false;
  const writes: string[] = [];
  await page.route("**/api/v1/secrets", async route => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON().value);
      saved = true;
      return route.fulfill({ json: { id: "secret", name: "harness:openai", created_at: at, updated_at: at } });
    }
    return route.fulfill({ json: { key_configured: true, known: [], secrets: saved ? [{ id: "secret", name: "harness:openai", created_at: at, updated_at: at }] : [] } });
  });
  await page.route("**/api/v1/runner", route => route.fulfill({ json: { online: true, slots: 3, busy: 1, queued: 0, runners: [{ id: "docker-one", backend: "docker", max_concurrent: 3, running: 1, heartbeat_at: at }] } }));
  await page.goto("/settings");
  await expect(page.getByText("No secrets yet.", { exact: false })).toBeVisible();
  await expect(page.getByText("concurrency 3", { exact: false })).toBeVisible();
  await page.getByRole("listitem").filter({ hasText: "OpenAI" }).getByRole("button", { name: "Add key" }).click();
  await page.getByLabel("Value", { exact: true }).fill("private-value-END1");
  await page.getByRole("button", { name: "Save secret" }).click();
  await expect(page.getByRole("status").filter({ hasText: "harness:openai saved" })).toBeVisible();
  await expect(page.getByLabel("Value", { exact: true })).toHaveValue("");
  await expect(page.getByRole("listitem").filter({ hasText: "OpenAI" }).getByText("Set", { exact: true })).toBeVisible();
  await page.getByRole("row").filter({ hasText: "harness:openai" }).getByRole("button", { name: "Replace" }).click();
  await page.getByLabel("Value", { exact: true }).fill("private-value-END2");
  await page.getByRole("button", { name: "Save secret" }).click();
  await expect(page.getByLabel("Value", { exact: true })).toHaveValue("");
  expect(writes).toEqual(["private-value-END1", "private-value-END2"]);
  await expect(page.getByText("END1", { exact: false })).toHaveCount(0);
  await page.screenshot({ path: "test-results/settings-desktop.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/settings-mobile.png", fullPage: true });
});

test("project repository validation accepts SSH and HTTPS inline", async ({ page }) => {
  await installChatApi(page);
  await page.route("**/api/v1/projects", route => route.fulfill({ json: [] }));
  await page.goto("/settings/projects");
  await page.getByRole("button", { name: "New project" }).click();
  const repo = page.getByLabel("GitHub repository");
  await repo.fill("git@gitlab.com:owner/repo");
  await expect(repo).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("button", { name: "Create project" })).toBeDisabled();
  for (const url of ["https://github.com/owner/repo", "git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git"]) {
    await repo.fill(url);
    await expect(repo).toHaveAttribute("aria-invalid", "false");
    await expect(page.getByText("Valid GitHub repository.", { exact: false })).toBeVisible();
  }
});

test("settings render loading, recoverable errors and empty runner state", async ({ page }) => {
  await installChatApi(page);
  let fail = true;
  await page.route("**/api/v1/secrets", route => fail
    ? route.fulfill({ status: 503, json: { code: "internal", message: "Secrets unavailable" } })
    : route.fulfill({ json: { key_configured: false, known: [], secrets: [] } }));
  await page.route("**/api/v1/runner", route => route.fulfill({ json: { online: false, slots: 0, busy: 0, queued: 0, runners: [] } }));
  await page.goto("/settings");
  await expect(page.getByText("No runners online.", { exact: false })).toBeVisible();
  const error = page.getByRole("alert").filter({ hasText: "Couldn’t load secrets" });
  await expect(error).toBeVisible({ timeout: 15_000 });
  fail = false;
  await error.getByRole("button", { name: "Retry" }).click();
  await expect(page.getByText("Secrets are read-only until SECRETS_KEY is set")).toBeVisible();
  await expect(page.getByRole("button", { name: "Save secret" })).toBeDisabled();
});
