import { expect, test, type Page } from "@playwright/test";
import { catalog, installChatApi } from "./mock-chat-api";

const at = "2026-11-04T12:00:00Z";
const SECRET = "sk-proj-super-secret-value-WXYZ";

type Secret = { id: string; name: string; grants_scope?: string; created_at: string; updated_at: string; last_used_at?: string; project?: unknown };

/** Secrets, projects and runner mocks on top of the shared API mock. */
async function installSettingsApi(page: Page, opts: { runners?: boolean; secretsFail?: boolean } = {}) {
  const api = await installChatApi(page, { codexAvailable: false });
  const secrets: Secret[] = [
    { id: "s1", name: "harness:anthropic", created_at: at, updated_at: at, last_used_at: at },
    { id: "s2", name: "git:ssh", grants_scope: "repo:write", created_at: at, updated_at: at },
    { id: "s3", name: "db:staging", grants_scope: "db:migrate", created_at: at, updated_at: at },
  ];
  const puts: Record<string, unknown>[] = [];
  const projectPuts: Record<string, unknown>[] = [];
  let modelFetches = 0;
  const project = { id: "p1", key: "WEB", name: "Web app", color: "#6e56cf", repo_url: "git@github.com:acme/web.git", default_branch: "main", archived: false, revision: 1 };
  await page.route("**/api/v1/secrets", async (route) => {
    const req = route.request();
    if (opts.secretsFail) return route.fulfill({ status: 500, json: { error: { code: "internal", message: "The API is unavailable." } } });
    if (req.method() === "PUT") {
      const body = req.postDataJSON() as { name: string; value: string };
      puts.push(body);
      const s = { id: `s${secrets.length + 1}`, name: body.name, created_at: at, updated_at: new Date().toISOString() };
      secrets.push(s);
      return route.fulfill({ json: s });
    }
    return route.fulfill({ json: { secrets, key_configured: true, known: [{ name: "db:staging", description: "Staging database URL" }] } });
  });
  await page.route("**/api/v1/projects", (route) => route.fulfill({ json: [project] }));
  await page.route("**/api/v1/projects/WEB", (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>;
    projectPuts.push(body);
    return route.fulfill({ json: { ...project, ...body, repo_url: "git@github.com:acme/payments.git", revision: 2 } });
  });
  await page.route("**/api/v1/runner", (route) =>
    route.fulfill({
      json: opts.runners === false
        ? { online: false, runners: [], slots: 0, busy: 0, live: 0, queued: 3 }
        : { online: true, runners: [{ id: "mbp-4242", backend: "docker", max_concurrent: 4, running: 2, version: "0.9.1", heartbeat_at: new Date().toISOString() }], slots: 4, busy: 2, live: 2, queued: 3 },
    }),
  );
  // Like the server: Codex is available once harness:openai exists.
  await page.route("**/api/v1/chat-models", (route) => {
    modelFetches++;
    return route.fulfill({ json: catalog(secrets.some((s) => s.name === "harness:openai")) });
  });
  return { api, puts, projectPuts, models: () => modelFetches };
}

test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

test("provider secrets show status and unlocks, are write-only, and saving refreshes the model picker", async ({ page }) => {
  const mock = await installSettingsApi(page);
  await page.goto("/chat?project=WEB");
  await page.getByRole("button", { name: "New conversation" }).first().click();
  await page.getByRole("button", { name: /Model for the next message/ }).click();
  await expect(page.getByRole("menuitem", { name: /Codex gpt-5-codex/ })).toHaveAttribute("aria-disabled", "true");
  await page.keyboard.press("Escape");
  const before = mock.models();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: /Settings/ }).click();

  await expect(page.getByRole("heading", { name: "Settings", level: 1 })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toContainText("a setting runs need is missing");
  await expect(page.getByRole("status").filter({ hasText: "Codex and OpenCode openai/… can’t run" })).toBeVisible();
  const conn = page.locator("#secrets li");
  await expect(conn.filter({ hasText: "harness:anthropic" })).toContainText("Set");
  await expect(conn.filter({ hasText: "harness:anthropic" })).toContainText("✓ Claude Code");
  await expect(conn.filter({ hasText: "harness:openai" })).toContainText("Not set");
  await expect(conn.filter({ hasText: "harness:openai" })).toContainText("✕ Codex");
  await expect(page.locator("#secrets")).not.toContainText("••••");
  await page.screenshot({ path: "test-results/settings-desktop.png", fullPage: true });

  await page.getByRole("button", { name: "Add OpenAI key" }).click();
  const value = page.getByLabel("New value for harness:openai");
  await expect(value).toBeFocused();
  await expect(value).toHaveAttribute("type", "password");
  await value.fill(SECRET);
  await page.getByRole("button", { name: "Save key" }).click();
  await expect(page.getByRole("status").filter({ hasText: "harness:openai saved. Codex and OpenCode openai/… are now available." })).toBeVisible();
  expect(mock.puts).toEqual([{ name: "harness:openai", value: SECRET }]);
  await expect(page.getByLabel("New value for harness:openai")).toHaveCount(0);
  await expect(page.getByText("Codex and OpenCode openai/… can’t run")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Primary" })).not.toContainText("a setting runs need is missing");
  expect(await page.content()).not.toContain(SECRET);

  // Switching from one Replace to another remounts the form with the new name.
  await page.getByRole("button", { name: "Replace db:staging" }).click();
  await page.getByRole("button", { name: "Add secret" }).click();
  const name = page.locator("#secrets").getByLabel("Name", { exact: true });
  await expect(name).toHaveValue("");
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByRole("button", { name: "Delete harness:anthropic" })).toBeVisible();

  // The chat model cache was invalidated, so back in chat Codex is selectable
  // without a reload (the cache is otherwise fresh for a minute).
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: /Assistant/ }).click();
  await page.getByRole("button", { name: "New conversation" }).first().click();
  await page.getByRole("button", { name: /Model for the next message/ }).click();
  await expect(page.getByRole("menuitem", { name: /Codex gpt-5-codex/ })).not.toHaveAttribute("aria-disabled", "true");
  expect(mock.models()).toBeGreaterThan(before);
});

test("project repo URLs are validated inline and accept SSH and HTTPS", async ({ page }) => {
  const mock = await installSettingsApi(page);
  await page.goto("/settings#projects");
  const projects = page.locator("#projects");
  await expect(projects).toContainText("git@github.com:acme/web.git");
  await projects.getByRole("button", { name: "Edit" }).click();
  const repo = projects.getByLabel("GitHub repository");
  await repo.fill("git@gitlab.com:northwind/payments.git");
  await expect(projects.getByText(/Only GitHub repositories work/)).toBeVisible();
  await expect(repo).toHaveAttribute("aria-invalid", "true");
  await expect(projects.getByRole("button", { name: "Save changes" })).toBeDisabled();
  await repo.fill("https://github.com/acme/payments");
  await expect(projects).toContainText("Saved as git@github.com:acme/payments.git");
  await expect(repo).not.toHaveAttribute("aria-invalid");
  await projects.getByRole("button", { name: "Save changes" }).click();
  await expect(projects.getByRole("status")).toContainText("Web app saved.");
  expect(mock.projectPuts[0]).toMatchObject({ repo_url: "https://github.com/acme/payments", default_branch: "main" });
});

test("runner backends show current values and what they control", async ({ page }) => {
  await installSettingsApi(page);
  await page.goto("/settings#runners");
  const runners = page.locator("#runners");
  await expect(runners.getByRole("rowheader", { name: "mbp-4242" })).toBeVisible();
  await expect(runners).toContainText("RUNNER_BACKEND = docker");
  await expect(runners).toContainText("RUNNER_MAX_CONCURRENT = 4");
  await expect(runners).toContainText("How many runs one runner executes at once");
  await expect(runners).toContainText("2 of 4");
});

test("sections show empty and error states independently", async ({ page }) => {
  await installSettingsApi(page, { runners: false, secretsFail: true });
  await page.goto("/settings");
  await expect(page.getByRole("alert").filter({ hasText: "Couldn’t load provider secrets" })).toBeVisible();
  await expect(page.locator("#secrets").getByRole("button", { name: "Retry" })).toBeVisible();
  await expect(page.locator("#runners")).toContainText("No runner online");
  await expect(page.locator("#runners")).toContainText("RUNNER_BACKEND = not reported");
  await expect(page.locator("#projects")).toContainText("Web app");
  await expect(page.locator("#workspace")).toContainText("Acme");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/settings-mobile.png", fullPage: true });
});
