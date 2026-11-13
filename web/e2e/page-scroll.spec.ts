import { expect, test, type Locator, type Page } from "@playwright/test";
import type { Schemas } from "../lib/api/client";
import { installBoardApi } from "./mock-board-api";

async function installLongLists(page: Page) {
  const api = await installBoardApi(page);
  for (let i = 10; i < 60; i++) {
    api.tickets.push({ ...api.tickets[0], id: `ticket-${i}`, key: `WEB-${i}`, title: `Ticket ${i}` });
  }
  const run: Schemas["Run"] = {
    id: "11111111-1111-4111-8111-111111111111", ticket_key: "WEB-1", project_key: "WEB",
    work_title: "Investigate scrolling", purpose: "ticket",
    agent: { id: "agent-builder", name: "Builder", handle: "builder", color_index: 0 },
    mode: "managed", state: "running", harness: "claude-code", model: "test",
    started_at: new Date().toISOString(), tool_calls: 0, input_tokens: 0, output_tokens: 0,
    cost_usd: 0, plan: [], scopes: [], approval_scopes: [],
  };
  await page.route("**/api/v1/runs?*", (route) => route.fulfill({ json: {
    runs: Array.from({ length: 50 }, (_, i) => ({ ...run, id: i === 0 ? run.id : `other-${i}`, work_title: `Run ${i}` })),
    has_more: false,
  } }));
  await page.route(`**/api/v1/runs/${run.id}?*`, (route) => route.fulfill({ json: {
    run, child_runs: [], turns: [], approvals: [],
    events: Array.from({ length: 50 }, (_, i) => ({
      seq: i + 1, type: "message", at: run.started_at,
      payload: { text: `Message ${i}: ${"A paragraph of output. ".repeat(10)}` },
    })),
  } }));
  return run.id;
}

async function expectViewportBoundary(page: Page) {
  await expect.poll(() => page.evaluate(() => ({
    height: document.documentElement.scrollHeight,
    width: document.documentElement.scrollWidth,
    y: window.scrollY,
  }))).toEqual({ height: page.viewportSize()!.height, width: page.viewportSize()!.width, y: 0 });
}

async function expectLastItemReachable(item: Locator) {
  await item.scrollIntoViewIfNeeded();
  await expect(item).toBeInViewport();
}

for (const width of [1280, 390]) {
  test.describe(`${width}px viewport`, () => {
    test.use({ viewport: { width, height: 844 } });

    test("long Runs lists and threads scroll inside the app, including with a system banner", async ({ page }) => {
      const id = await installLongLists(page);
      await page.goto("/runs");
      const list = page.getByRole("complementary", { name: "Runs", exact: true });
      const last = list.getByRole("link", { name: /Run 49/ });
      await expect(last).toBeAttached();
      // The mock has no harness: the system banner also takes space in the viewport.
      await expect(page.getByRole("status", { name: /Agents can’t run/ })).toBeVisible();
      await expectViewportBoundary(page);
      await expectLastItemReachable(last);
      await expectViewportBoundary(page);
      if (width < 768) {
        await page.goto(`/runs/${id}`);
      }
      const thread = page.getByRole("log", { name: width < 768 ? "Run log" : "Run thread", exact: true });
      await expect(thread).toBeVisible();
      await expect(thread).toContainText("Message 49:");
      expect(await thread.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
      await thread.evaluate((el) => el.scrollTo({ top: 0 }));
      await expect(thread.getByText(/^Message 0:/)).toBeInViewport();
      await expectViewportBoundary(page);
      await page.getByRole("button", { name: "Hide this banner for this session" }).click();
      await expectViewportBoundary(page);
    });

    test("long Tickets lists, boards and histories stay inside the app with their last item reachable", async ({ page }) => {
      await installLongLists(page);
      await page.goto("/tickets?project=WEB");
      const last = page.locator('[data-ticket-row="WEB-59"]');
      await expect(last).toBeAttached();
      await expectViewportBoundary(page);
      await expectLastItemReachable(last);
      await expectViewportBoundary(page);
      await page.getByRole("button", { name: "Hide this banner for this session" }).click();
      await expectViewportBoundary(page);
      if (width < 768) await expect(page.getByRole("navigation", { name: "Primary", exact: true })).toBeInViewport();

      await page.goto("/tickets?layout=board&project=WEB");
      const card = page.getByRole("article", { name: "Ticket WEB-59", exact: true });
      await expect(card).toBeAttached();
      await expectViewportBoundary(page);
      await expectLastItemReachable(card);
      await expectViewportBoundary(page);

      await page.goto("/tickets/WEB-1");
      const history = page.getByRole("link", { name: /^Builder run,/ }).last();
      await expect(history).toBeAttached();
      await expectViewportBoundary(page);
      await expectLastItemReachable(history);
      await expectViewportBoundary(page);
    });
  });
}
