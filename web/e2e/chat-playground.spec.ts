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

// What the gateway's probe sends from the app's page (playground/probe.go), as
// that page's own script would: where it is, a console error, a failed fetch.
const PROBE = `<script>
  const send = (m) => parent.postMessage(Object.assign({ vesperProbe: 1 }, m), "*");
  history.replaceState(null, "", "/orders?page=2");
  send({ type: "location", path: "/orders?page=2", title: "Orders" });
  send({ type: "console", text: "TypeError: cannot read properties of undefined (reading 'total')" });
  send({ type: "request", method: "GET", url: "/api/orders", status: 500, id: "req-42", error: "" });
</script>`;

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
    return route.fulfill({ contentType: "text/html", body: "<!doctype html><title>Acme</title><h1>Acme Admin</h1>" + PROBE });
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
  await full.getByRole("button", { name: "Send", exact: true }).click();
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

test("Send to chat attaches a report and a screenshot of the frame to the next message", async ({ page }) => {
  // Tab capture needs a person to pick the tab; a canvas stream stands in for it.
  await page.addInitScript(() => {
    navigator.mediaDevices.getDisplayMedia = async () => {
      const c = Object.assign(document.createElement("canvas"), { width: 1280, height: 720 });
      const g = c.getContext("2d")!;
      const paint = () => { g.fillStyle = "#3a6"; g.fillRect(0, 0, c.width, c.height); requestAnimationFrame(paint); };
      paint();
      return c.captureStream(30);
    };
  });
  await open(page);
  // After open(): the chat mock's catch-all would otherwise answer first.
  const uploads: { name: string; body: string; bytes: string }[] = [];
  await page.route("**/api/v1/chat-attachments", (route) => {
    const buf = route.request().postDataBuffer() ?? Buffer.alloc(0);
    const raw = buf.toString("utf8");
    const name = /filename="([^"]+)"/.exec(raw)?.[1] ?? "";
    uploads.push({ name, body: raw, bytes: buf.toString("latin1") });
    return route.fulfill({ status: 201, json: { id: `att-${uploads.length}`, filename: name, content_type: name.endsWith(".png") ? "image/png" : "text/markdown", size_bytes: raw.length } });
  });
  await page.getByTestId("playground-card").getByRole("button", { name: "Open" }).click();
  const panel = page.getByTestId("playground-panel");
  // The address bar follows the app's own navigation, which the frame can't tell Vesper.
  await expect(panel.getByRole("textbox", { name: "Path" })).toHaveValue("/orders?page=2");

  await panel.getByRole("button", { name: "Send to chat" }).click();
  const chips = page.getByTestId("attachment-chip");
  await expect(chips).toHaveCount(2);
  await expect(chips.nth(0)).toHaveAttribute("data-status", "done");
  await expect(chips.nth(1)).toHaveAttribute("data-status", "done");
  await expect(page.getByTestId("attachment-preview")).toHaveCount(1);
  await expect(page.getByRole("combobox", { name: "Message" })).toBeFocused();

  const report = uploads.find((u) => u.name.endsWith(".md"))!;
  expect(report.name).toMatch(/^playground-\d{8}-\d{6}\.md$/);
  expect(report.body).toContain("- Page: `/orders?page=2` (Orders)");
  expect(report.body).toContain("- Screenshot: attached");
  expect(report.body).toContain("reading 'total'");
  expect(report.body).toContain("### GET /api/orders → 500");
  expect(report.body).toContain("request id `req-42`");
  expect(uploads.find((u) => u.name.endsWith(".png"))?.bytes).toContain("\x89PNG");
});

test("Send to chat still sends the report when sharing is declined", async ({ page }) => {
  await page.addInitScript(() => {
    navigator.mediaDevices.getDisplayMedia = async () => { throw new DOMException("denied", "NotAllowedError"); };
  });
  await open(page);
  // After open(): the chat mock's catch-all would otherwise answer first.
  const uploads: string[] = [];
  await page.route("**/api/v1/chat-attachments", (route) => {
    const raw = route.request().postDataBuffer()?.toString("latin1") ?? "";
    uploads.push(raw);
    return route.fulfill({ status: 201, json: { id: `att-${uploads.length}`, filename: "r.md", content_type: "text/markdown", size_bytes: 3 } });
  });
  await page.getByTestId("playground-card").getByRole("button", { name: "Open" }).click();
  await expect(page.frameLocator("[data-testid=playground-preview]").getByRole("heading", { name: "Acme Admin" })).toBeVisible();
  await page.getByTestId("playground-panel").getByRole("button", { name: "Send to chat" }).click();
  await expect(page.getByTestId("attachment-chip")).toHaveCount(1);
  expect(uploads).toHaveLength(1);
  expect(uploads[0]).toContain("- Screenshot: none (sharing was cancelled)");
});
