import { expect, test } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

const map = (page: import("@playwright/test").Page) => page.getByRole("region", { name: "Dependency map", exact: true });

test("board map lays out chains, traverses dependencies, and opens the existing editor", async ({ page }) => {
  const api = await installBoardApi(page);
  api.editTicket("WEB-2", { estimate_points: 5 });
  await page.goto("/board?project=WEB&tab=map");
  const m = map(page);
  await expect(m.getByText("Longest unfinished chain: 2 tickets · 8 pts", { exact: true })).toBeVisible();
  const root = m.getByRole("button", { name: /WEB-2 Backlog/ });
  const child = m.getByRole("button", { name: /WEB-1 Todo/ });
  await root.focus();
  await page.keyboard.press("ArrowRight");
  await expect(child).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Edit WEB-1" })).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(child).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  await expect(root).toBeFocused();
  await m.getByRole("button", { name: "List", exact: true }).click();
  await expect(m.getByRole("table")).toBeVisible();
  await expect(m.getByRole("row").filter({ hasText: "WEB-1 Build ticket modal" })).toContainText("WEB-2 (Backlog)");
  expect(api.unhandled).toEqual([]);
});

test("filtered map retains canceled external blockers and removes only the selected dependency", async ({ page }) => {
  const api = await installBoardApi(page);
  api.editTicket("WEB-2", { status: "canceled", epic: { id: "epic-2", name: "Other epic" } });
  api.editTicket("WEB-1", { blocked_by: [
    { id: "ticket-2", key: "WEB-2", title: "Write integration checks", status: "canceled" },
    { id: "ticket-3", key: "WEB-3", title: "ADR and contract", status: "done" },
  ] });
  let body: unknown;
  await page.route("**/api/v1/tickets/WEB-1/dependencies", async (route) => {
    body = route.request().postDataJSON();
    api.editTicket("WEB-1", { dependency_revision: 2, blocked_by: api.tickets[0].blocked_by.filter((b) => b.key !== "WEB-2") });
    await route.fulfill({ json: api.tickets[0] });
  });
  await page.goto("/board?project=WEB&epic=epic-1&tab=map");
  const m = map(page);
  await expect(m.getByText("WEB-2 still blocks WEB-1", { exact: true })).toBeVisible();
  await expect(m.getByRole("button", { name: /WEB-2 Canceled/ })).toContainText("outside filter");
  const reopen = m.getByRole("button", { name: "Open WEB-2 to reopen", exact: true });
  await reopen.click();
  await expect(page.getByRole("dialog", { name: "Edit WEB-2" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(reopen).toBeFocused();
  await m.getByRole("button", { name: "Remove WEB-2 from WEB-1", exact: true }).click();
  await expect(m.getByText("Dependency removed.")).toBeVisible();
  await expect(m.getByText("WEB-2 still blocks WEB-1", { exact: true })).toHaveCount(0);
  expect(body).toEqual({ revision: 1, blocked_by: ["WEB-3"] });
});

test("failed dependency writes retain blockers and show the server conflict", async ({ page }) => {
  const api = await installBoardApi(page);
  api.editTicket("WEB-2", { status: "canceled" });
  api.editTicket("WEB-1", { blocked_by: [{ id: "ticket-2", key: "WEB-2", title: "Write integration checks", status: "canceled" }] });
  await page.route("**/api/v1/tickets/WEB-1/dependencies", (route) => route.fulfill({ status: 409, json: { code: "conflict", message: "Dependencies changed elsewhere" } }));
  await page.goto("/board?project=WEB&tab=map");
  const m = map(page);
  await m.getByRole("button", { name: "Remove WEB-2 from WEB-1", exact: true }).click();
  await expect(m.getByRole("alert")).toContainText("Dependencies changed elsewhere");
  await expect(m.getByText("WEB-2 still blocks WEB-1", { exact: true })).toBeVisible();
});

test("epic Map tab offers the same map, accessible list, and editor", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/epics/epic-1");
  await page.getByRole("navigation", { name: "Epic views" }).getByRole("button", { name: "Map", exact: true }).click();
  const m = map(page);
  await expect(m.getByRole("button", { name: /WEB-6 Canceled/ }).first()).toBeVisible();
  await m.getByRole("button", { name: "List", exact: true }).click();
  await m.getByRole("button", { name: "WEB-1 Build ticket modal", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Edit WEB-1" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(m.getByRole("button", { name: "WEB-1 Build ticket modal", exact: true })).toBeFocused();
});

for (const failure of ["map", "editor"] as const) {
  test(`epic editor retains a dirty draft after terminal background ${failure} failure`, async ({ page }) => {
    await page.clock.install();
    await installBoardApi(page);
    await page.goto("/epics/epic-1");
    await page.getByRole("navigation", { name: "Epic views" }).getByRole("button", { name: "Map", exact: true }).click();
    const m = page.locator('section[aria-label="Dependency map"]');
    await m.getByRole("button", { name: /WEB-1 Todo/ }).click();
    const dialog = page.getByRole("dialog", { name: "Edit WEB-1" });
    await dialog.getByRole("textbox", { name: "Title", exact: true }).fill("Unsaved map draft");
    let failed = 0;
    await page.route(failure === "map" ? "**/api/v1/tickets?**" : "**/api/v1/tickets/WEB-1", async (route) => {
      failed++;
      await route.fulfill({ status: 500, json: { code: "internal", message: "Refetch failed" } });
    });
    // Age both successful queries beyond the provider's 30s staleTime.
    // Keep retry timers running, then reconnect to refetch stale active queries.
    await page.clock.fastForward(31_000);
    await page.evaluate(() => window.dispatchEvent(new Event("offline")));
    await page.evaluate(() => window.dispatchEvent(new Event("online")));
    const message = failure === "map" ? "Couldn't refresh dependency map:" : "Couldn't refresh WEB-1:";
    // The alert appears only once retries finish and the query enters error state.
    await expect(m.getByRole("alert", { includeHidden: true }).filter({ hasText: message })).toBeVisible({ timeout: 20_000 });
    expect(failed).toBeGreaterThan(0);
    await expect(dialog.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Unsaved map draft");
    await expect(m.locator('[data-ticket-key="WEB-1"]').first()).toBeVisible();
  });
}
