import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";

// LAZYAGENTS-47: the Dashboard answers "what is the team doing and what needs me".
type S = components["schemas"];

const project: S["Project"] = { id: "project-web", key: "WEB", name: "Web app", color: "#6e56cf", default_branch: "main", archived: false, revision: 1 };
const atlas: S["AgentRef"] = { id: "aaaaaaaa-0000-4000-8000-000000000001", name: "Atlas", handle: "atlas", color_index: 0 };
const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
const today = new Date().toISOString().slice(0, 10);

const inboxItem = (id: string, kind: S["InboxItem"]["kind"], key: string, min: number): S["InboxItem"] => ({
  id, kind, title: `${kind} on ${key}`, summary: "", href: `/tickets/${key}`, ticket_key: key, project: "WEB", created_at: minutesAgo(min), resolved: false, read: false,
});

const run = (id: string, purpose: S["Run"]["purpose"], state: S["Run"]["state"], extra: Partial<S["Run"]> = {}): S["Run"] => ({
  id, ticket_key: "", purpose, agent: atlas, mode: "managed", state, harness: "claude-code", model: "opus", started_at: minutesAgo(12),
  tool_calls: 0, input_tokens: 0, output_tokens: 0, cost_usd: 0.84, cost_source: "harness", plan: [], approval_scopes: [], scopes: [], ...extra,
});

const card = (n: number, unmet = 0): S["TicketSummary"] => ({
  id: `t-${n}`, key: `WEB-${n}`, project_key: "WEB", title: `Ticket ${n}`, status: "todo", priority: "none", labels: [],
  criteria_total: 0, criteria_done: 0, unmet_dependencies: unmet, updated_at: minutesAgo(n),
});

const epic = (name: string, health: S["Epic"]["health"]): S["Epic"] => ({
  id: `epic-${name}`, project_id: project.id, name, health, requires_mockup: false, status: "open",
  criteria_total: 0, criteria_done: 0, points_total: 106, points_done: 44, tickets_total: 20, tickets_open: 11,
});

type State = { runner: boolean; runsFail: boolean; runs: S["Run"][]; harness: boolean; repo: boolean; plans: number };

async function installDashboardApi(page: Page, init: Partial<State> = {}) {
  const state: State = {
    runner: true,
    runsFail: false,
    harness: true,
    repo: true,
    plans: 1,
    runs: [
      run("r-ticket", "ticket", "running", {
        ticket_key: "WEB-48", work_title: "API",
        plan: [{ position: 1, text: "Read", state: "done" }, { position: 2, text: "Implement API", state: "current" }, { position: 3, text: "Test", state: "pending" }, { position: 4, text: "Ship", state: "pending" }],
      }),
      run("r-plan", "planning", "waiting", { work_title: "Q4 plan" }),
      run("r-chat-waiting", "chat", "waiting", { work_title: "Design review" }),
      run("r-chat-busy", "chat", "running", { work_title: "Sprint planning" }),
    ],
    ...init,
  };
  const requests: string[] = [];
  await page.addInitScript(() => {
    const w = window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> };
    w.__sse = [];
    class FakeEventSource {
      onopen?: () => void;
      onerror?: () => void;
      onmessage?: (e: { data: string }) => void;
      constructor() {
        w.__sse.push(this);
        setTimeout(() => this.onopen?.(), 0);
      }
      close() {
        w.__sse = w.__sse.filter((x) => x !== this);
      }
    }
    (window as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
  });
  // 501 open cards with the only blocked one last: nothing is cut at 500.
  const cards = [...Array.from({ length: 500 }, (_, i) => card(1000 + i)), card(50, 1)];
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    requests.push(`${path}${url.search}`);
    const json = (data: unknown, status = 200) => route.fulfill({ status, json: data });
    if (path === "/bootstrap") return json({
      workspace: { id: "workspace", name: "Vesper", slug: "lazyagents" },
      me: { id: "user", name: "Liam", email: "liam@example.com", initials: "LM", role_title: "Owner" },
      users: [], agents: [{ ...atlas, kind: "main", role: "Tech lead", specialty: "", model: "opus", status: "running" }],
      epics: [epic("Refresh", "at_risk"), epic("Healthy", "on_track")], projects: [state.repo ? { ...project, repo_url: "git@github.com:acme/web.git" } : project], labels: [],
      counts: { inbox: 6, wiki_suggestions: 0, live_agents: 1 },
    });
    if (path === "/dashboard") return json({
      stats: { range: "30d", generated_at: new Date().toISOString(), performance: { completed: 4, median_cycle_seconds: null, merged_prs: 0, closed_prs: 0, spend_usd: 0, unpriced_runs: 0 },
        daily: [{ date: today, count: 3 }, { date: "2000-01-01", count: 99 }] },
      counts: { total: 0, backlog: 0, todo: 0, in_progress: 0, in_review: 0, done: 0 }, review_over_day: 0, agents: [], projects: [], attention: [], recent_artifacts: [],
      runner: { online: state.runner, runners: [], slots: 1, busy: 0, live: 0, queued: 0 },
      weekly_spend: { since: minutesAgo(60), cost_usd: 42.18, runs: 14, unpriced_runs: 0 },
    });
    if (path === "/runner") return json(state.runner
      ? { online: true, runners: [{ id: "mbp", backend: "docker", max_concurrent: 4, running: 0, heartbeat_at: minutesAgo(0) }], slots: 4, busy: 0, live: 0, queued: 0 }
      : { online: false, runners: [], slots: 0, busy: 0, live: 0, queued: 0 });
    if (path === "/chat-models") return json([
      { harness: "claude-code", label: "Claude Code", available: state.harness, ...(state.harness ? {} : { unavailable_reason: "no harness:anthropic secret" }), models: [{ id: "opus", label: "Opus", default: true }] },
      { harness: "codex", label: "Codex", available: false, unavailable_reason: "no harness:openai secret", models: [{ id: "gpt-5-codex", label: "gpt-5-codex", default: true }] },
    ]);
    if (path === "/secrets") return json({ secrets: [], key_configured: true, known: [] });
    if (path === "/planning-sessions") return json(Array.from({ length: state.plans }, (_, i) => ({ id: `plan-${i}`, project: "WEB" })));
    if (path === "/inbox") return json({ items: [
      inboxItem("m1", "mention", "WEB-40", 500), inboxItem("r1", "review", "WEB-41", 10), inboxItem("a1", "approval", "WEB-44", 5),
      inboxItem("q1", "question", "WEB-43", 60), inboxItem("k1", "mockup", "WEB-47", 30), inboxItem("p1", "planning", "WEB-39", 20),
    ] });
    if (path === "/runs") {
      if (state.runsFail) return json({ code: "internal", message: "runs unavailable" }, 500);
      return json({ runs: state.runs, has_more: false });
    }
    if (path === "/chats") return json({ chats: [
      { id: "chat-1", project: "WEB", title: "Design review", agent: atlas, harness: "claude-code", model: "opus", state: "idle", archived: false, revision: 1, message_count: 4, created_at: minutesAgo(90), updated_at: minutesAgo(5) },
      { id: "chat-2", project: "WEB", title: "Sprint planning", agent: atlas, harness: "claude-code", model: "opus", state: "running", archived: false, revision: 1, message_count: 2, created_at: minutesAgo(30), updated_at: minutesAgo(1) },
    ], has_more: false });
    if (path === "/projects/WEB/board") return json({ project, columns: [
      { status: "backlog", cards: [], count: 0, points: 0, truncated: false },
      { status: "todo", cards, count: cards.length, points: 0, truncated: false },
      { status: "in_progress", cards: [], count: 0, points: 0, truncated: false },
      { status: "in_review", cards: [], count: 0, points: 0, truncated: false },
      { status: "done", cards: [], count: 0, points: 0, truncated: false },
    ] });
    if (path === "/tickets/WEB-50") return json({ key: "WEB-50", blocked_by: [{ id: "t-48", key: "WEB-48", title: "API endpoint", status: "in_progress" }, { id: "t-49", key: "WEB-49", title: "Done one", status: "done" }] });
    return json({ code: "not_found", message: `mock: no handler for ${request.method()} ${path}` }, 404);
  });
  return {
    state,
    requests,
    async push(type: string) {
      await page.evaluate((t) => {
        const streams = (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse;
        for (const s of streams) s.onmessage?.({ data: JSON.stringify({ type: t }) });
      }, type);
    },
  };
}

const panel = (page: Page, name: string) => page.getByRole("region", { name });

test("panels show live work, gates and delivery health, each linking to its full view", async ({ page }) => {
  await installDashboardApi(page);
  await page.goto("/dashboard");

  await expect(page.getByText("6 items need you · 3 live runs · 2 active chats")).toBeVisible();

  // Needs you: top five in the Inbox's urgency order.
  const needs = panel(page, "Needs you");
  await expect(needs.getByRole("heading")).toHaveText("Needs you· 6");
  await expect(needs.getByRole("link", { name: / on WEB-/ })).toHaveText(["approval on WEB-44", "question on WEB-43", "mockup on WEB-47", "planning on WEB-39", "review on WEB-41"]);
  await expect(needs.getByRole("link", { name: "View Inbox →" })).toHaveAttribute("href", "/inbox");

  // Live runs: the chat waiting between turns is not live work; the responding chat is.
  const runs = panel(page, "Live runs");
  await expect(runs.getByRole("heading")).toHaveText("Live runs· 3");
  await expect(runs).toContainText("Atlas · WEB-48");
  await expect(runs).toContainText("Implement API · step 2 of 4");
  await expect(runs).toContainText("12m elapsed · $0.84 so far");
  await expect(runs).not.toContainText("Design review");
  await expect(runs.getByRole("link", { name: "All Runs →" })).toHaveAttribute("href", "/runs");

  // Chats are counted separately.
  const chats = panel(page, "Chats active");
  await expect(chats.getByRole("heading")).toHaveText("Chats active· 2");
  await expect(chats).toContainText("waiting for your next turn");
  await expect(chats).toContainText("agent responding");
  await expect(chats.getByRole("link", { name: "All Chats →" })).toHaveAttribute("href", "/chat");

  // Blocked: found past the first 500 open tickets, with its open blocker.
  const blocked = panel(page, "Blocked tickets");
  await expect(blocked.getByRole("heading")).toHaveText("Blocked tickets· 1");
  await expect(blocked).toContainText("Blocked by WEB-48 · API endpoint");
  await expect(blocked).not.toContainText("and 1 more");
  await expect(blocked.getByRole("link", { name: "Blocked Board →" })).toHaveAttribute("href", "/board?project=WEB&blocked=true");

  const epics = panel(page, "Epics at risk");
  await expect(epics.getByRole("heading")).toHaveText("Epics at risk· 1");
  await expect(epics).toContainText("44 / 106 points done · 11 tickets open");
  await expect(epics.getByRole("link", { name: "Open epic →" })).toHaveAttribute("href", "/epics/epic-Refresh");
  await expect(epics.getByRole("link", { name: "All Epics →" })).toHaveAttribute("href", "/epics");

  const spend = panel(page, "Spend this week");
  await expect(spend).toContainText("$42.18");
  await expect(spend).toContainText("Across 14 runs");
  await expect(spend.getByRole("link", { name: "View Costs →" })).toHaveAttribute("href", "/costs");

  // Throughput: exactly the last 14 UTC days; older counts are outside the window.
  const through = panel(page, "Throughput · last 14 days");
  await expect(through).toContainText("3 tickets done · canceled excluded · UTC");
  await expect(through.getByRole("img")).toHaveAttribute("aria-label", new RegExp(`${today}: 3$`));
  await expect(through.getByRole("link", { name: "Done tickets →" })).toHaveAttribute("href", "/tickets?status=done&view=all");

  // A configured workspace sees no health banner and no open checklist.
  await expect(page.getByRole("region", { name: "Setup complete" })).toContainText("You’re set up");
  await expect(page.getByRole("region", { name: "Get set up" })).toHaveCount(0);
  await expect(page.getByRole("status", { name: /Agents can’t run|No runner online/ })).toHaveCount(0);
});

test("panels refresh from live events without a reload", async ({ page }) => {
  const api = await installDashboardApi(page);
  await page.goto("/dashboard");
  const runs = panel(page, "Live runs");
  await expect(runs).toContainText("$0.84 so far");

  api.state.runs = api.state.runs.map((r) => (r.id === "r-ticket" ? { ...r, cost_usd: 1.5 } : r));
  await api.push("run.updated");
  await expect(runs).toContainText("$1.50 so far");
  await expect(page.getByText("6 items need you")).toBeVisible();
});

test("a failed panel shows its error and recovers on retry", async ({ page }) => {
  const api = await installDashboardApi(page, { runsFail: true });
  await page.goto("/dashboard");
  const runs = panel(page, "Live runs");
  await expect(runs.getByRole("alert")).toContainText("Couldn’t load: runs unavailable", { timeout: 15_000 });
  // The other panels are unaffected.
  await expect(panel(page, "Needs you").getByRole("heading")).toHaveText("Needs you· 6");

  api.state.runsFail = false;
  await runs.getByRole("button", { name: "Retry" }).click();
  await expect(runs).toContainText("Atlas · WEB-48");
  await expect(runs.getByRole("alert")).toHaveCount(0);
});

test("a new workspace sees the checklist and a health banner naming missing secrets; steps tick without a reload", async ({ page }) => {
  await installDashboardApi(page, { runner: false, harness: false, repo: false, plans: 0 });
  await page.goto("/dashboard");
  const banner = page.getByRole("status", { name: /Agents can’t run/ });
  await expect(banner).toContainText("No harness is available: add harness:anthropic or harness:openai.");
  await expect(banner.getByRole("link", { name: "Fix in Provider credentials →" })).toHaveAttribute("href", "/settings#secrets");

  const setup = page.getByRole("region", { name: "Get set up" });
  await expect(setup).toContainText("0 of 5 done");
  await expect(setup).toContainText("Claude Code: no harness:anthropic secret · Codex: no harness:openai secret");
  await expect(setup.getByRole("link", { name: "Projects →" })).toHaveAttribute("href", "/settings#projects");
  await expect(setup.getByRole("link", { name: "Provider credentials →" })).toHaveAttribute("href", "/settings#secrets");
  await expect(setup.getByRole("link", { name: "Runner backends →" })).toHaveAttribute("href", "/settings#runners");
  await expect(setup.getByRole("link", { name: "Agents →" })).toHaveAttribute("href", "/agents");
  await expect(setup.getByRole("button", { name: "Dismiss" })).toHaveCount(0);

  // Opening Agents ticks "Review the default agents" in the same session.
  await setup.getByRole("link", { name: "Agents →" }).click();
  await expect(page).toHaveURL(/\/agents$/);
  await page.goBack();
  await expect(page.getByRole("region", { name: "Get set up" })).toContainText("1 of 5 done");

  // × hides the banner for the session; it's still hidden after navigating.
  await banner.getByRole("button", { name: "Hide this banner for this session" }).click();
  await expect(page.getByRole("status", { name: /Agents can’t run/ })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("region", { name: "Get set up" })).toBeVisible();
  await expect(page.getByRole("status", { name: /Agents can’t run/ })).toHaveCount(0);
});

test("the runner banner shows when a harness works but no runner is online", async ({ page }) => {
  await installDashboardApi(page, { runner: false });
  await page.goto("/dashboard");
  const banner = page.getByRole("status", { name: "No runner online" });
  await expect(banner).toContainText("Dispatched work stays queued.");
  await expect(banner.getByRole("link", { name: "Runner backends →" })).toHaveAttribute("href", "/settings#runners");
});

test("a complete checklist can be dismissed and doesn't come back", async ({ page }) => {
  await installDashboardApi(page);
  await page.goto("/dashboard");
  const done = page.getByRole("region", { name: "Setup complete" });
  await done.getByRole("button", { name: "Dismiss" }).click();
  await expect(done).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Your team at a glance" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Setup complete" })).toHaveCount(0);
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Settings", level: 1 })).toBeVisible();
  await expect(page.getByRole("region", { name: "Setup complete" })).toHaveCount(0);
});
