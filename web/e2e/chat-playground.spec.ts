import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// A ```playground fence opens the chat's playground beside it. The app, its
// OpenAPI document and its logs come from the runner's gateway, on the page's
// host at the playground's port: mocked here as http://localhost:8091.

test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

const ID = "01a1c6cb-986d-73de-b7f4-52e0cef03de9";
const GATEWAY = "http://localhost:8091";
const at = "2026-11-10T17:42:00Z";

function playground(over: Record<string, unknown> = {}) {
  return {
    id: ID, chat_id: "c", state: "running", step: "", action: null, port: 8091, branch: "feat/runner-settings",
    error: null, error_log: "", stop_reason: "", idle_ttl_seconds: 1800, started_at: at, data_reset_at: at, last_used_at: at, updated_at: at,
    processes: [{ name: "web", port: 3000 }, { name: "api", port: 8000 }],
    services: [{ name: "postgres", image: "postgres:17-alpine" }],
    routes: [{ path: "/api", process: "api" }, { path: "/", process: "web" }],
    openapi: "api/openapi.yaml",
    ...over,
  };
}

const spec = {
  openapi: "3.0.3",
  info: { title: "Vesper" },
  servers: [{ url: "/api/v1" }],
  tags: [{ name: "runs" }],
  paths: {
    "/runner/test-environments": {
      get: { operationId: "listTestEnvironments", tags: ["runs"], responses: {} },
      put: {
        operationId: "putTestEnvironment",
        summary: "Replace a repository's test environment",
        description: "<img src=x onerror=\"document.title='ran'\"> stays text",
        tags: ["runs"],
        requestBody: { required: true, content: { "application/json": { schema: { $ref: "#/components/schemas/Input" } } } },
        responses: {},
      },
    },
  },
  components: {
    schemas: { Input: { type: "object", required: ["repo", "recipe"], properties: { repo: { type: "string", example: "github.com/liamphmurphy/vesper" }, recipe: { type: "object", properties: { nix_packages: { type: "array", items: { type: "string" } } } } } } },
  },
};

type Calls = { actions: string[]; sent: { method: string; body: string | null; cookie: string }[] };

async function open(page: Page, initial = playground()) {
  const calls: Calls = { actions: [], sent: [] };
  let current: Record<string, unknown> = initial;
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/playground(\/[a-z]+)?$/, (route) => {
    const m = /\/playground\/([a-z]+)$/.exec(new URL(route.request().url()).pathname);
    if (m) {
      calls.actions.push(m[1]);
      current = { ...current, ...(m[1] === "start" ? { state: "starting", step: "Starting postgres", error: null } : { action: m[1] }) };
      return route.fulfill({ contentType: "application/json", body: JSON.stringify(current) });
    }
    return route.fulfill({ contentType: "application/json", body: JSON.stringify({ playground: current }) });
  });
  const cors = (route: Route) => ({ "Access-Control-Allow-Origin": new URL(route.request().frame().url()).origin, "Access-Control-Allow-Credentials": "true", "Access-Control-Expose-Headers": "Content-Type" });
  await page.route(`${GATEWAY}/**`, async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    if (url.pathname === "/__vesper/openapi") return route.fulfill({ headers: cors(route), contentType: "application/json", body: JSON.stringify(spec) });
    if (url.pathname === "/__vesper/logs") {
      const now = new Date().toISOString();
      const events = [
        { t: now, source: "web", stream: "stdout", text: "ready on :3000 in 2.1s" },
        { t: now, source: "api", stream: "stderr", text: 'error: null value in column "packages"' },
      ];
      // retry: a long reconnect delay, so the closed mock stream doesn't restart.
      return route.fulfill({ headers: cors(route), contentType: "text/event-stream", body: "retry: 600000\n\n" + events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join("") });
    }
    if (req.method() === "OPTIONS") return route.fulfill({ status: 204, headers: { ...cors(route), "Access-Control-Allow-Methods": "PUT", "Access-Control-Allow-Headers": "content-type" } });
    if (url.pathname.startsWith("/api/v1/")) {
      calls.sent.push({ method: req.method(), body: req.postData(), cookie: (await req.allHeaders()).cookie ?? "" });
      return route.fulfill({ status: 500, headers: cors(route), contentType: "application/json", body: JSON.stringify({ code: "internal", message: "internal error" }) });
    }
    return route.fulfill({ contentType: "text/html", body: "<!doctype html><title>Acme</title><h1>Acme Admin</h1>" });
  });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill(`playground: ${ID}`);
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page.getByTestId("playground-card")).toBeVisible();
  return calls;
}

test("the card opens the running app beside the chat, then full screen", async ({ page }) => {
  const calls = await open(page);
  await expect(page.getByTestId("playground-card")).toContainText("feat/runner-settings");
  await page.getByTestId("playground-card").getByRole("button", { name: "Open" }).click();
  const panel = page.getByTestId("playground-panel");
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId("playground-state")).toHaveText("Running");
  await expect(page.frameLocator("[data-testid=playground-preview]").getByRole("heading", { name: "Acme Admin" })).toBeVisible();
  await expect(panel).toContainText("stops after 30 min idle");
  await expect(panel.getByRole("link", { name: "Open in a new tab" })).toHaveAttribute("href", GATEWAY);

  await panel.getByRole("button", { name: "Full screen" }).click();
  const full = page.getByTestId("playground-fullscreen");
  await expect(full).toBeVisible();
  await expect(panel).toBeHidden();
  await page.keyboard.press("Escape");
  await expect(full).toBeHidden();
  await expect(page.getByTestId("playground-panel")).toBeVisible();

  await page.getByTestId("playground-panel").getByRole("button", { name: "↺ Reset data" }).click();
  await expect(page.getByTestId("playground-state")).toHaveText("Resetting");
  expect(calls.actions).toEqual(["reset"]);
});

test("the API tab reads the OpenAPI document and sends from the playground's origin", async ({ page }) => {
  const calls = await open(page);
  await page.getByTestId("playground-card").getByRole("button", { name: "Open" }).click();
  await page.getByRole("button", { name: "Full screen" }).click();
  const full = page.getByTestId("playground-fullscreen");
  await full.getByRole("tab", { name: "API" }).click();
  await expect(full).toContainText("2 operations under /api/v1, from api/openapi.yaml");
  await full.getByRole("textbox", { name: "Filter operations" }).fill("put");
  await full.getByRole("button", { name: /PUT\s*\/runner\/test-environments/ }).click();
  // The document is the agent's: its markup is shown, never run.
  await expect(full.getByText("<img src=x onerror=")).toBeVisible();
  expect(await page.title()).not.toBe("ran");

  const body = full.getByRole("textbox", { name: "Body" });
  await expect(body).toHaveValue(/github.com\/liamphmurphy\/vesper/);
  await body.fill('{"repo":"x","recipe":{"nix_packages":null}}');
  await expect(full).toContainText("recipe.nix_packages expected array, got null");
  await full.getByRole("button", { name: "Send" }).click();
  const res = full.getByTestId("playground-response");
  await expect(res).toContainText("500");
  await expect(res).toContainText('"message": "internal error"');
  expect(calls.sent).toEqual([{ method: "PUT", body: '{"repo":"x","recipe":{"nix_packages":null}}', cookie: expect.any(String) }]);

  await res.getByRole("button", { name: "Open in Logs" }).click();
  await expect(full.getByTestId("playground-logs")).toContainText('error: null value in column "packages"');
  await expect(full.getByRole("tab", { name: /Logs/ })).toContainText("1");
  await full.getByRole("button", { name: "api", exact: true }).click();
  await expect(full.getByTestId("playground-logs")).not.toContainText("ready on :3000");
});

test("a failed playground shows why, and starts again", async ({ page }) => {
  const calls = await open(page, playground({ state: "failed", port: null, error: "setup exited with code 1", error_log: 'migrate: relation "runs" already exists' }));
  await expect(page.getByTestId("playground-card")).toContainText("setup exited with code 1");
  await page.getByRole("button", { name: "Playground", exact: true }).click();
  const panel = page.getByTestId("playground-panel");
  await expect(panel.getByTestId("playground-state")).toHaveText("Failed");
  await expect(panel.getByTestId("playground-error-log")).toContainText("already exists");
  await panel.getByRole("button", { name: "Start again" }).click();
  await expect(panel.getByTestId("playground-step")).toContainText("Starting postgres");
  expect(calls.actions).toEqual(["start"]);
});
