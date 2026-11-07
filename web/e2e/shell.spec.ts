import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

// LAZYAGENTS-42: global project switcher, grouped nav and project states.
const nav = (page: Page) => page.getByRole("navigation", { name: "Primary" });
const switcher = (page: Page) => page.getByRole("button", { name: /^Switch project/ });
const board = (page: Page) => page.locator('[data-testid="board-screen"]:visible');

async function installWithArchived(page: Page) {
  const api = await installBoardApi(page);
  api.projects.push({ ...api.projects[0], id: "project-old", key: "OLD", name: "Legacy", archived: true });
  return api;
}

test("switching projects from the board rescopes the board and project nav, and is remembered", async ({ page }) => {
  const api = await installWithArchived(page);
  await page.goto("/board?project=WEB");
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Web app");
  const crumbs = page.getByRole("navigation", { name: "Breadcrumb" });
  await expect(crumbs.getByRole("link", { name: "Web app" })).toBeVisible();
  await expect(crumbs.locator('[aria-current="page"]')).toHaveText("Board");
  for (const group of ["Personal", "Project", "Workspace"]) await expect(nav(page).getByRole("group", { name: group })).toBeVisible();
  await expect(nav(page).getByRole("group", { name: "Project" }).getByRole("link")).toHaveText(["Board", "Tickets", "Epics", "Plans", "Chat", "Artifacts"]);
  await expect(nav(page).locator('[aria-current="page"]')).toHaveText("Board");

  await switcher(page).click();
  // Radix names the menu after its trigger.
  const menu = page.getByRole("menu", { name: /^Switch project/ });
  // Archived projects are never offered.
  await expect(menu.getByRole("menuitemradio")).toHaveText([/All projects/, /Web app/, /Operations/]);
  await expect(menu.getByRole("menuitemradio", { name: /^Web app/ })).toHaveAttribute("aria-checked", "true");
  await menu.getByRole("menuitemradio", { name: /^Operations/ }).click();

  await expect(page).toHaveURL(/\/board\?project=OPS$/);
  await expect(board(page).getByText("No tickets", { exact: true })).toHaveCount(5);
  expect(api.boardRequests.at(-1)).toContain("/projects/OPS/board");
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Operations");
  for (const [label, href] of [
    ["Board", "/board?project=OPS"], ["Tickets", "/tickets?project=OPS"], ["Epics", "/epics?project=OPS"],
    ["Plans", "/plans?project=OPS"], ["Chat", "/chat?project=OPS"], ["Artifacts", "/artifacts?project=OPS"],
    ["Dashboard", "/"], ["Agents", "/agents"], ["Wiki", "/wiki"], ["Settings", "/settings"],
  ]) await expect(nav(page).getByRole("link", { name: label, exact: true })).toHaveAttribute("href", href);

  // No project in the URL resolves to the remembered one.
  await page.goto("/board");
  await expect(page).toHaveURL(/\/board\?project=OPS$/);
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Operations");

  // All projects forgets it, so the board asks for one.
  await switcher(page).click();
  await page.getByRole("menuitemradio", { name: /^All projects/ }).click();
  await expect(page).toHaveURL(/\/board$/);
  const chooser = page.getByRole("region", { name: "Choose a project" });
  await expect(chooser.getByRole("link")).toHaveText([/Web app/, /Operations/]);
  await expect(nav(page).getByRole("link", { name: "Tickets", exact: true })).toHaveAttribute("href", "/tickets");
  await chooser.getByRole("link", { name: /^Web app/ }).click();
  await expect(page).toHaveURL(/\/board\?project=WEB$/);
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Web app");
  expect(api.unhandled).toEqual([]);
});

test("an archived or unknown project shows an explicit empty state with a way back", async ({ page }) => {
  const api = await installWithArchived(page);
  await page.goto("/board?project=OPS");
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Operations");

  await page.goto("/board?project=OLD");
  await expect(page.getByRole("heading", { name: "Project unavailable" })).toBeVisible();
  await expect(page.getByText("OLD is archived. Restore it in Settings to work on it here.")).toBeVisible();
  await expect(page.getByRole("link", { name: "Manage projects" })).toHaveAttribute("href", "/settings/projects");
  await page.getByRole("link", { name: "Back to Operations" }).click();
  await expect(page).toHaveURL(/\/board\?project=OPS$/);

  await page.goto("/board?project=NOPE");
  await expect(page.getByText("No project with the key NOPE.")).toBeVisible();
  // Unavailable projects are never remembered.
  expect(await page.evaluate(() => localStorage.getItem("lazyagents.project"))).toBe("OPS");
  expect(api.boardRequests.some((u) => /OLD|NOPE/.test(u))).toBe(false);
  expect(api.unhandled).toEqual([]);
});

test("Inbox and Runs nav counts refresh on live updates", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB");
  const inbox = nav(page).getByRole("link", { name: /^Inbox/ });
  const runs = nav(page).getByRole("link", { name: /^Runs/ });
  await expect(inbox).toHaveAccessibleName("Inbox");
  await expect(runs).toHaveAccessibleName("Runs");
  api.counts.inbox = 2;
  api.runner.busy = 1;
  await api.emit("run.started");
  await expect(inbox).toHaveAccessibleName(/^Inbox\s*2\s*unread$/);
  await expect(runs).toHaveAccessibleName(/^Runs\s*1\s*running$/);
  expect(api.unhandled).toEqual([]);
});

test("Runs lists live work and marks the nav item current", async ({ page }) => {
  const api = await installBoardApi(page);
  api.runner.busy = 1;
  const agent = { id: "agent-builder", name: "Builder", handle: "builder", kind: "main", color_index: 0, role: "Developer", specialty: "UI", model: "test", status: "running" };
  await page.route("**/api/v1/agent-overviews", (route) => route.fulfill({ json: {
    stats_as_of: new Date().toISOString(),
    agents: [{
      agent, max_concurrency: 1, harness: "claude-code", queue: [], queue_count: 0, scopes: [], stats: {},
      planning_runs: [],
      assignments: [{ run_id: "11111111-1111-4111-8111-111111111111", ticket: { id: "ticket-1", key: "WEB-1", title: "Build ticket modal", status: "in_progress", priority: "high" },
        state: "running", harness: "claude-code", model: "opus", started_at: new Date().toISOString(), step: "", step_position: 0, steps_done: 0, steps_total: 0 }],
    }],
  } }));
  await page.goto("/runs");
  await expect(nav(page).locator('[aria-current="page"]')).toHaveText(/^Runs/);
  await expect(page.getByRole("status").filter({ hasText: "1 running · 0 queued · 3 slots" })).toBeVisible();
  const row = page.getByRole("list", { name: "Live runs" }).getByRole("link");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("WEB-1");
  await expect(row).toHaveAttribute("href", "/runs/11111111-1111-4111-8111-111111111111");
  expect(api.unhandled).toEqual([]);
});

test("cross-project links keep all projects while bare routes use the remembered one", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=OPS");
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Operations");

  // e.g. an agent's "Review assigned tickets" link: not narrowed to OPS.
  await page.goto("/tickets?view=all&assignee=builder");
  await expect(page.getByRole("link", { name: "Build ticket modal" })).toBeVisible();
  await expect(page).toHaveURL(/\/tickets\?view=all&assignee=builder$/);
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: All projects");

  // Opening the route itself resolves to the remembered project.
  await page.goto("/tickets");
  await expect(page).toHaveURL(/\/tickets\?project=OPS$/);
  await expect(switcher(page)).toHaveAccessibleName("Switch project, current: Operations");
  expect(api.unhandled).toEqual([]);
});
