import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

// VESPER-9: the compact shell (project "mobile", 390×844) has a bottom tab bar instead of the sidebar.
const bar = (page: Page) => page.getByRole("navigation", { name: "Primary" });
const TABS = ["Chat", "Runs", "Tickets", "More"];

test("the tab bar replaces the sidebar and switches between its tabs", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/inbox");
  // Needs you is the second tab, a button that opens the tray rather than a link.
  await expect(bar(page).locator("a, button")).toHaveText(["Chat", "Needs you", "Runs", "Tickets", "More"]);
  await expect(bar(page).getByRole("link")).toHaveText(TABS);
  await expect(page.getByRole("complementary", { name: "Main menu" })).toHaveCount(0);

  // Fixed to the bottom edge, and every tab is at least 44px tall.
  const box = await bar(page).boundingBox();
  expect(Math.round(box!.y + box!.height)).toBe(844);
  for (const name of TABS) {
    const tab = await bar(page).getByRole("link", { name: new RegExp(`^${name}`) }).boundingBox();
    expect(tab!.height).toBeGreaterThanOrEqual(44);
  }

  const routes: Record<string, RegExp> = { Chat: /\/chat/, Runs: /\/runs/, Tickets: /\/tickets/, More: /\/more$/ };
  for (const name of [...TABS.slice(1), TABS[0]]) {
    await bar(page).getByRole("link", { name: new RegExp(`^${name}`) }).click();
    await expect(page).toHaveURL(routes[name]);
    await expect(bar(page).locator('[aria-current="page"]')).toHaveText(name);
  }
  // Still no sideways scroll on a phone.
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
});

test("the Needs you badge and Runs dot follow the nav counts", async ({ page }) => {
  const api = await installBoardApi(page);
  api.counts.needs_you = 3;
  api.runner.live = 2;
  await page.goto("/more");
  await expect(bar(page).getByRole("button", { name: "Needs you, 3 waiting" })).toBeVisible();
  await expect(bar(page).getByTestId("tab-badge")).toHaveText("3");
  await expect(bar(page).getByRole("link", { name: "Runs 2 running" }).getByTestId("tab-dot")).toBeVisible();
});

test("ticket and run detail screens hide the tab bar", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/tickets");
  await expect(bar(page)).toBeVisible();
  await page.goto(`/tickets/${api.tickets[0].key}`);
  await expect(page.getByRole("heading", { name: api.tickets[0].title })).toBeVisible();
  await expect(bar(page)).toHaveCount(0);
  await page.goto("/runs/run-1");
  await expect(page.locator("main")).toBeVisible();
  await expect(bar(page)).toHaveCount(0);
});
