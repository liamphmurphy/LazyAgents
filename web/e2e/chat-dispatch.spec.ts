import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Starting a ticket's run from a chat: the persona's ```dispatch card, the run in
// the tray above the message box, then the card's summary once it ends.

const RUN = "88888888-8888-4888-8888-888888888888";
const now = () => new Date().toISOString();

const preview = (blockers: string[] = []) => ({
  ticket_key: "WEB-3", agent: { id: "a1", name: "Pike", handle: "pike", color_index: 6 },
  resolved: { harness: "claude-code", model: "opus-5.5" }, repo_url: "https://github.com/acme/web.git", base_branch: "main", branch: "pike/web-3",
  setup: [], context: [], scopes: [], secrets: [], harness_key: { secret: "harness:anthropic", present: true },
  runner: { host_folder_workspaces: false, online: true, runners: [], slots: 3, busy: 1, live: 1, queued: 0 },
  blockers, queued_behind: [],
});

type State = { runs: Record<string, unknown>[]; starts: unknown[]; cancels: number; blockers: string[]; decisions?: string[] };

const run = (over: Record<string, unknown> = {}) => ({
  run_id: RUN, ticket: "WEB-3", ticket_title: "Fix payouts", agent: "pike", state: "running", started_at: now(),
  cost_usd: 0.42, steps_total: 4, steps_done: 1, current_step: "Fix Round()", ...over,
});

async function serve(page: Page, state: State) {
  await page.route(/\/api\/v1\/tickets\/WEB-3\/dispatch$/, (route: Route) => route.fulfill({ json: preview(state.blockers) }));
  await page.route(/\/api\/v1\/tickets\/WEB-3$/, (route: Route) => route.fulfill({ json: { key: "WEB-3", title: "Fix payouts", status: "todo" } }));
  await page.route(/\/api\/v1\/approvals\/[^/]+\/(approve|deny)$/, (route: Route) => {
    (state.decisions ??= []).push(route.request().url().split("/approvals/")[1]);
    state.runs = state.runs.map((r) => ({ ...r, state: "running", wait_reason: undefined, approval: undefined }));
    return route.fulfill({ json: {} });
  });
  await page.route(/\/api\/v1\/runs\/[^/]+\/cancel$/, (route: Route) => {
    state.cancels++;
    state.runs = state.runs.map((r) => ({ ...r, state: "cancelled", ended_at: now() }));
    return route.fulfill({ json: {} });
  });
  await page.route(/\/api\/v1\/chats\/[^/]+\/runs$/, (route: Route) => {
    const req = route.request();
    if (req.method() === "POST") {
      const body = req.postDataJSON();
      state.starts.push(body);
      const started = run({ state: "queued", steps_total: 0, steps_done: 0, current_step: undefined, message_id: body.message_id });
      state.runs = [started];
      return route.fulfill({ status: 201, json: started });
    }
    return route.fulfill({ json: { runs: state.runs } });
  });
}

async function offer(page: Page, state: State) {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  await serve(page, state);
  await page.goto("/chat");
  const box = page.getByRole("combobox", { name: "Message" });
  await box.fill("dispatch: fix the payout bug");
  await box.press("Enter");
  const card = page.getByTestId("dispatch-card");
  await expect(card).toBeVisible();
  return Object.assign(card, { mock });
}

test("a dispatch card starts the run as you, then the tray follows it", async ({ page }) => {
  const state: State = { runs: [], starts: [], cancels: 0, blockers: [] };
  const card = await offer(page, state);
  await expect(card).toContainText("Start a run for WEB-3?");
  await expect(card).toContainText("Fix payouts");
  await expect(card).toContainText("@pike");
  await expect(card).toContainText("acme/web · main");
  await expect(card).toContainText("Immediately");

  await card.getByRole("button", { name: "Start run" }).click();
  await expect.poll(() => state.starts.length).toBe(1);
  expect(state.starts[0]).toMatchObject({ ticket: "WEB-3" });
  expect((state.starts[0] as { message_id?: string }).message_id).toBeTruthy();
  await expect(page.getByTestId("dispatch-started")).toContainText("Started WEB-3 with @pike");

  const tray = page.getByTestId("ticket-run-tray");
  await expect(tray).toContainText("Queued");
  // The agent posts its plan and gets going; the tray follows by polling.
  const messageId = (state.starts[0] as { message_id: string }).message_id;
  state.runs = [run({ message_id: messageId, activity: { tool: "Bash", command: "go test ./internal/payouts" } })];
  await expect(tray).toContainText("Running", { timeout: 10000 });
  await expect(tray).toContainText("Step 2 of 4 · Fix Round()");
  // The header says a run is going and opens it.
  const pill = page.getByTestId("chat-runs-pill");
  await expect(pill).toHaveText("1 run active");
  await expect(pill).toHaveAttribute("href", `/runs/${RUN}`);
  // What it's doing, in the chat's words: never the command itself.
  await expect(page.getByTestId("ticket-run-doing")).toHaveText("Running tests");
  await expect(tray).not.toContainText("go test");
  await page.screenshot({ path: "/tmp/chat-dispatch-tray.png" });

  // It asks for approval: the decision is right in the tray.
  state.runs = [run({ message_id: messageId, state: "waiting", wait_reason: "approval", approval: { id: "99999999-9999-4999-8999-999999999999", scope: "db:migrate", action: "Run migrations on staging", detail: '{"env":"staging"}', risk: "Reversible" } })];
  const approval = page.getByTestId("ticket-run-approval");
  await expect(approval).toContainText("Run migrations on staging", { timeout: 10000 });
  await expect(tray).toContainText("Waiting · approval");
  await page.screenshot({ path: "/tmp/chat-dispatch-approval.png" });
  await approval.getByRole("button", { name: "Approve" }).click();
  await expect.poll(() => state.decisions).toEqual(["99999999-9999-4999-8999-999999999999/approve"]);

  // It asks a question: the tray says to answer here.
  state.runs = [run({ message_id: messageId, state: "waiting", wait_reason: "human_reply" })];
  await expect(tray).toContainText("asked you something. Answer in this chat", { timeout: 10000 });

  // Once it ends it leaves the tray, and the card becomes its summary with the PR.
  state.runs = [run({ message_id: (state.starts[0] as { message_id: string }).message_id, state: "completed", ended_at: now(), steps_done: 4, current_step: undefined, pr_url: "https://github.com/acme/web/pull/412" })];
  await expect(tray).toHaveCount(0, { timeout: 10000 });
  const summary = page.getByTestId("ticket-run");
  await expect(summary).toContainText("Completed");
  await expect(summary.getByRole("link", { name: "Open PR" })).toHaveAttribute("href", "https://github.com/acme/web/pull/412");

  // Review it with me hands the PR to this chat's persona as the person's message.
  await summary.getByRole("button", { name: "Review it with me" }).click();
  await expect.poll(() => card.mock.posted.map((p) => JSON.stringify(p.body))).toContainEqual(expect.stringContaining("Review the pull request for WEB-3: https://github.com/acme/web/pull/412"));
});

test("a card that dispatch would refuse says why and can't start", async ({ page }) => {
  const state: State = { runs: [], starts: [], cancels: 0, blockers: ["WEB-3 already has a running run."] };
  const card = await offer(page, state);
  await expect(card.getByRole("alert")).toContainText("already has a running run");
  await expect(card.getByRole("button", { name: "Start run" })).toBeDisabled();
  await card.getByRole("button", { name: "Not now" }).click();
  await expect(page.getByTestId("dispatch-dismissed")).toBeVisible();
  expect(state.starts).toHaveLength(0);
});

test("Stop in the tray cancels the run", async ({ page }) => {
  const state: State = { runs: [], starts: [], cancels: 0, blockers: [] };
  const card = await offer(page, state);
  await card.getByRole("button", { name: "Start run" }).click();
  const tray = page.getByTestId("ticket-run-tray");
  await expect(tray).toBeVisible();
  await tray.getByRole("button", { name: "Stop" }).click();
  await expect.poll(() => state.cancels).toBe(1);
  await expect(tray).toHaveCount(0, { timeout: 10000 });
  await expect(page.getByTestId("ticket-run")).toContainText("Cancelled");
});
