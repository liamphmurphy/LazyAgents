import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

// Real-stack run page (LAZYAGENTS-163): no page.route mocks. An agent's run (an
// unmanaged session on the real /mcp endpoint, beside the fake runner) asks for
// approval and asks a question; the person decides with A in the run thread and
// answers from its composer, and the agent reads both.

const API = process.env.E2E_STACK_API_URL;
if (!API) throw new Error("E2E_STACK_API_URL is not set; run `npm run e2e:stack`");

/** Minimal streamable-HTTP MCP client: initialize once, then tools/call. */
async function mcpSession(request: APIRequestContext, token: string) {
  const headers = { authorization: `Bearer ${token}`, "content-type": "application/json", accept: "application/json, text/event-stream" };
  let id = 0;
  const parse = async (res: Awaited<ReturnType<APIRequestContext["post"]>>) => {
    const body = await res.text();
    const data = body.split("\n").filter((l) => l.startsWith("data: ")).map((l) => JSON.parse(l.slice(6)));
    const msg = data.length ? data[data.length - 1] : JSON.parse(body);
    if (msg.error) throw new Error(`MCP error: ${JSON.stringify(msg.error)}`);
    return msg.result;
  };
  const init = await request.post(`${API}/mcp`, { headers, data: { jsonrpc: "2.0", id: ++id, method: "initialize",
    params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "run-composer-e2e", version: "1" } } } });
  expect(init.ok()).toBeTruthy();
  await parse(init);
  const session = { ...headers, "mcp-session-id": init.headers()["mcp-session-id"] };
  await request.post(`${API}/mcp`, { headers: session, data: { jsonrpc: "2.0", method: "notifications/initialized" } });
  return async (name: string, args: Record<string, unknown>) => {
    const result = await parse(await request.post(`${API}/mcp`, { headers: session,
      data: { jsonrpc: "2.0", id: ++id, method: "tools/call", params: { name, arguments: args } } }));
    expect(result.isError, `${name}: ${JSON.stringify(result.content)}`).toBeFalsy();
    return result.structuredContent as Record<string, unknown>;
  };
}

test("decide and reply to a waiting run from the run thread", async ({ page, request }) => {
  const api = async (method: string, path: string, data?: unknown) => {
    const res = await request.fetch(`${API}/api/v1${path}`, { method, data });
    expect(res.ok(), `${method} ${path}: ${res.status()} ${await res.text()}`).toBeTruthy();
    return res.status() === 204 ? undefined : res.json();
  };
  const runState = async (id: string) => (await api("GET", `/runs/${id}`)).run.state as string;

  const created = await api("POST", "/agents", { handle: "thread-agent", name: "Thread agent", kind: "main" });
  await api("PUT", "/agents/thread-agent/definition", { name: "Thread agent", role: "", specialty: "", color_index: 4,
    spec: { ...created.spec, scopes: { ...created.spec.scopes, "deploy:staging": "requires_approval" } } });
  const ticket = await api("POST", "/tickets", { project_key: "E2E", title: "Reply from the thread", status: "todo", assignee_agent_id: created.agent.id });
  const { token } = await api("POST", "/agents/thread-agent/tokens", { label: "run-composer e2e" });
  const call = await mcpSession(request, token);
  const runId = String((await call("claim_ticket", { ticket: ticket.key })).run_id);
  await call("post_plan", { ticket: ticket.key, steps: ["Ask for a deploy", "Ask a question"] });

  // --- An approval, decided with A in the thread --------------------------
  const asked = await call("request_approval", { ticket: ticket.key, scope: "deploy:staging", action: "Deploy build 7 to staging" });
  await page.goto(`/runs/${runId}`);
  const thread = page.getByRole("region", { name: "Run thread" });
  await expect(thread.getByRole("heading", { name: "Reply from the thread" })).toBeVisible();
  await expect(thread.getByText("Step 1 of 2", { exact: true })).toBeVisible();
  const card = thread.getByRole("group", { name: "Needs attention" });
  await expect(card).toContainText("Deploy build 7 to staging");
  await page.locator("body").click({ position: { x: 900, y: 5 } });
  await page.keyboard.press("a");
  expect(await call("wait_for_approval", { approval_id: asked.approval_id, timeout_s: 5 })).toMatchObject({ state: "approved" });
  await expect.poll(() => runState(runId)).toBe("running");
  await expect(card).toHaveCount(0);

  // --- A question, answered from the composer ------------------------------
  await call("await_human", { ticket: ticket.key, reason: "Which region should staging use?" });
  await expect(thread.getByRole("group", { name: "Needs attention" })).toContainText("Which region should staging use?");
  const composer = page.getByRole("form", { name: "Reply to Thread agent" });
  await composer.getByRole("textbox", { name: "Reply to Thread agent" }).fill("eu-west-1, please.");
  await composer.getByRole("textbox", { name: "Reply to Thread agent" }).press("Enter");
  // The run resumes at once, so the composer goes back to Follow up.
  await expect(page.getByRole("form", { name: "Follow up with Thread agent" })).toBeVisible();
  await expect.poll(() => runState(runId)).toBe("running");
  const signal = await call("await_signal", { ticket: ticket.key, timeout_s: 5 });
  expect(JSON.stringify(signal)).toContain("eu-west-1, please.");
});
