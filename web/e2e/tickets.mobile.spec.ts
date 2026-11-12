import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

// VESPER-16: compact Tickets list, board and ticket detail (project "mobile", 390×844).
const noSidewaysScroll = async (page: Page) =>
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);

test("Tickets opens as a grouped list with one line of sideways-scrolling filter chips", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/tickets?project=WEB");
  await expect(page.getByRole("heading", { name: "Tickets", level: 1 })).toBeVisible();
  await expect(page.getByRole("link", { name: "New ticket" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Tickets layout" }).locator('[aria-current="page"]')).toHaveText("List");

  // Grouped by status, In progress (in review included) first, with 64px rows.
  const groups = page.locator("section[aria-label]");
  await expect(groups.first()).toHaveAttribute("aria-label", "In progress, 2");
  const row = page.locator('[data-ticket-row="WEB-1"]');
  await expect(row).toContainText("Build ticket modal");
  await expect(row).toContainText("WEB-1");
  await expect(row).toContainText("3 pts");
  expect((await row.boundingBox())!.height).toBe(64);

  // The chips stay on one line and scroll inside their row, not the page.
  const chips = page.getByTestId("filter-chips");
  const box = (await chips.boundingBox())!;
  expect(box.height).toBeLessThan(60);
  expect(await chips.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true);
  await chips.evaluate((el) => el.scrollTo({ left: el.scrollWidth }));
  await expect(page.getByRole("button", { name: /^Group by/ })).toBeInViewport();
  await noSidewaysScroll(page);
});

test("the board shows one status column at a time with a pager", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/tickets?layout=board&project=WEB");
  const pager = page.getByRole("group", { name: "Status pager" });
  await expect(pager.locator('[aria-current="true"]')).toContainText("Backlog");
  await expect(page.getByTestId("column-backlog")).toBeInViewport({ ratio: 0.9 });
  await expect(page.getByTestId("column-todo")).not.toBeInViewport();

  await pager.getByRole("button", { name: /In Progress/ }).click();
  await expect(page.getByTestId("column-in_progress")).toBeInViewport({ ratio: 0.9 });
  await expect(page.getByTestId("column-backlog")).not.toBeInViewport();
  await expect(pager.locator('[aria-current="true"]')).toContainText("In Progress");

  // Swiping the columns moves the pager too.
  await page.getByLabel("Board columns").evaluate((el) => el.scrollTo({ left: 0 }));
  await expect(pager.locator('[aria-current="true"]')).toContainText("Backlog");
  await noSidewaysScroll(page);
});

test("opening a ticket from the list and changing its status in a sheet", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/tickets?project=WEB");
  await page.locator('[data-ticket-row="WEB-1"]').click();
  await expect(page).toHaveURL(/\/tickets\/WEB-1$/);
  await expect(page.getByRole("heading", { name: "Build ticket modal" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Primary" })).toHaveCount(0);

  // Comment and Start run are pinned to the bottom edge.
  const actions = page.getByRole("group", { name: "Ticket actions" });
  const bar = (await actions.boundingBox())!;
  expect(Math.round(bar.y + bar.height)).toBe(844);
  await expect(actions.getByRole("button", { name: "Comment" })).toBeVisible();
  await expect(actions.getByRole("button", { name: "Start run" })).toBeVisible();

  // The status chip opens its picker as a bottom sheet.
  const strip = page.getByRole("group", { name: "Properties" });
  await strip.getByRole("button", { name: "Status: Todo" }).click();
  const sheet = page.getByRole("dialog", { name: "Status" });
  await expect(sheet).toBeVisible();
  // Once it has slid up, it sits on the bottom edge, full width.
  await expect.poll(async () => { const b = (await sheet.boundingBox())!; return [Math.round(b.y + b.height), Math.round(b.width)]; }).toEqual([844, 390]);
  await sheet.getByRole("button", { name: "In Progress" }).click();
  await expect(sheet).toHaveCount(0);
  await expect(strip.getByRole("button", { name: "Status: In Progress" })).toBeVisible();
  expect(api.patches.at(-1)).toMatchObject({ key: "WEB-1", body: { status: "in_progress" } });

  // The other chips open sheets too.
  await strip.getByRole("button", { name: "Priority: High" }).click();
  await expect(page.getByRole("dialog", { name: "Priority" })).toBeVisible();
  await page.getByRole("dialog", { name: "Priority" }).getByRole("button", { name: "Close" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await noSidewaysScroll(page);
});

test("criteria actions show without hover", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/tickets/WEB-1");
  expect(await page.evaluate(() => matchMedia("(hover: none)").matches)).toBe(true);
  await expect(page.getByRole("button", { name: "Remove criterion: Works" })).toHaveCSS("opacity", "1");
});
