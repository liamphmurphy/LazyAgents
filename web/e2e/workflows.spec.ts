import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Workflows: build one on the canvas, review a persona's suggestion, start from /workflow, approve a gate.

const WF = "22222222-2222-4222-8222-222222222222";
const RUN = "33333333-3333-4333-8333-333333333333";
const now = () => new Date().toISOString();
const node = (id: string, type: string, title: string, x: number, config = {}) => ({ id, type, title, x, y: 200, config });
const graph = {
  nodes: [node("start", "trigger", "Chat message", 40), node("work", "agent", "Research", 300, { prompt: "Research it" }), node("ok", "gate", "Approve draft", 560), node("page", "wiki", "Add to wiki", 820, { path: "Research/Result" })],
  edges: [{ from: "start", to: "work" }, { from: "work", to: "ok" }, { from: "ok", to: "page" }],
};
const workflow = (over = {}) => ({ id: WF, name: "Research to wiki", description: "Research and file it", draft: graph, issues: [], published_version: 1, unpublished_changes: false, proposals: [], updated_at: now(), ...over });
const run = (over = {}) => ({
  id: RUN, chat_id: "c", name: "Research to wiki", version: 1, graph, input: "Compare queues", state: "waiting", error: "", created_at: now(),
  steps: [{ node_id: "start", attempt: 1, state: "done", output: "Compare queues", decision: "", note: "", created_at: now() }, { node_id: "work", attempt: 1, state: "done", output: "The **findings**", decision: "", note: "", created_at: now() }, { node_id: "ok", attempt: 1, state: "waiting", output: "", decision: "", note: "", created_at: now() }],
  ...over,
});

async function serveWorkflows(page: Page, state: { wf: ReturnType<typeof workflow>; posts: { url: string; body: unknown }[]; runs: unknown[] }) {
  await page.route(/\/api\/v1\/workflows(\/.*)?(\?.*)?$/, async (route: Route) => {
    const req = route.request();
    if (req.method() === "GET" && /\/workflows$/.test(req.url())) return route.fulfill({ json: { workflows: [state.wf] } });
    if (req.method() === "GET") return route.fulfill({ json: state.wf });
    state.posts.push({ url: req.url(), body: req.postData() ? req.postDataJSON() : null });
    if (req.url().endsWith("/apply")) state.wf = { ...state.wf, proposals: [], unpublished_changes: true };
    return route.fulfill({ json: state.wf });
  });
  await page.route(/\/api\/v1\/workflow-runs(\/.*)?$/, async (route: Route) => {
    const req = route.request();
    if (req.method() === "POST") {
      state.posts.push({ url: req.url(), body: req.postDataJSON() });
      state.runs = [run({ state: "done" })];
      return route.fulfill({ json: run({ state: "done" }) });
    }
    return route.fulfill({ json: /workflow-runs$/.test(req.url()) ? { runs: state.runs } : run() });
  });
  await page.route(/\/api\/v1\/chats\/[^/]+\/workflows$/, async (route: Route) => {
    const req = route.request();
    if (req.method() === "POST") {
      state.posts.push({ url: req.url(), body: req.postDataJSON() });
      state.runs = [run()];
      return route.fulfill({ status: 201, json: run() });
    }
    return route.fulfill({ json: { runs: state.runs } });
  });
}

test("the canvas shows the draft and adding a node saves it", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  const state = { wf: workflow(), posts: [] as { url: string; body: unknown }[], runs: [] as unknown[] };
  await serveWorkflows(page, state);
  await page.goto(`/workflows/${WF}`);
  await expect(page.getByTestId("node-work")).toBeVisible();
  await page.getByRole("button", { name: "Post to chat" }).click();
  await expect(page.getByTestId("node-chat_1")).toBeVisible();
  await expect(page.getByTestId("workflow-inspector")).toBeVisible();
  await expect.poll(() => state.posts.some((p) => JSON.stringify(p.body).includes("chat_1")), { timeout: 5000 }).toBe(true);
  await page.screenshot({ path: "/tmp/workflow-editor.png" });
});

test("a persona's suggestion previews on the canvas until applied", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  const proposed = { nodes: [...graph.nodes, node("check", "agent", "Fact check", 300, { prompt: "Check" })], edges: graph.edges };
  const state = { wf: workflow({ proposals: [{ id: "44444444-4444-4444-8444-444444444444", summary: "Add a fact check", graph: proposed, created_at: now() }] }), posts: [] as { url: string; body: unknown }[], runs: [] as unknown[] };
  await serveWorkflows(page, state);
  await page.goto(`/workflows/${WF}`);
  await expect(page.getByTestId("workflow-proposal")).toContainText("Add a fact check");
  await expect(page.getByTestId("node-check")).toContainText("+ Agent step");
  await expect(page.getByRole("button", { name: "Publish" })).toBeDisabled();
  await page.getByRole("button", { name: "Apply" }).click();
  await expect(page.getByTestId("workflow-proposal")).toHaveCount(0);
  expect(state.posts.some((p) => p.url.endsWith("/apply"))).toBe(true);
});

test("/workflow lists published workflows, confirms, then the gate appears above the composer", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  const state = { wf: workflow(), posts: [] as { url: string; body: unknown }[], runs: [] as unknown[] };
  await serveWorkflows(page, state);
  await page.goto("/chat?project=WEB");
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  const box = page.getByRole("combobox", { name: "Message" });
  await expect(box).toBeFocused();
  await box.pressSequentially("/workflow ");
  await page.getByTestId("command-suggestions").getByRole("option", { name: "Research to wiki" }).click();
  await expect(page.getByTestId("workflow-start")).toContainText("Run Research to wiki?");
  await page.getByRole("textbox", { name: "What to run it on" }).fill("Compare queues");
  await page.getByRole("button", { name: "Run workflow" }).click();
  await expect(page.getByTestId("workflow-gate")).toContainText("The findings");
  await page.getByRole("button", { name: "Approve" }).click();
  await expect.poll(() => state.posts.some((p) => p.url.endsWith("/gate"))).toBe(true);
  const start = state.posts.find((p) => p.url.endsWith("/workflows"))!;
  expect(start.body).toMatchObject({ workflow_id: WF, input: "Compare queues" });
});

test("the run page shows step status and the gate", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  const state = { wf: workflow(), posts: [] as { url: string; body: unknown }[], runs: [] as unknown[] };
  await serveWorkflows(page, state);
  await page.goto(`/workflows/runs/${RUN}`);
  await expect(page.getByTestId("node-ok")).toContainText("Waiting for you");
  await expect(page.getByTestId("workflow-gate")).toBeVisible();
  await page.screenshot({ path: "/tmp/workflow-run.png" });
});
