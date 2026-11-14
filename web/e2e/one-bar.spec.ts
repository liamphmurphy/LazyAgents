import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";
import { installChatApi } from "./mock-chat-api";
import { deckWith, installDeck } from "./mock-deck-api";

// One shell: every page's top bar is 44px with back and forward in the same place, and an app's
// dock continues that bar instead of stacking headers of its own under it.

test.use({ viewport: { width: 1400, height: 900 } });

async function backOf(page: Page) {
  const back = page.getByRole("group", { name: "History" }).getByRole("button", { name: "Go back" });
  await expect(back).toBeVisible();
  return (await back.boundingBox())!;
}

test("Knowledge's tabbed bar is the shared top bar: same height, back button in the same place", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/settings");
  const reference = await backOf(page);
  const settingsBar = (await page.locator("header").filter({ has: page.getByRole("group", { name: "History" }) }).boundingBox())!;

  await installChatApi(page);
  await page.goto("/knowledge");
  const bar = page.locator("header").filter({ has: page.getByRole("tablist", { name: "Open pages" }) });
  await expect(bar).toBeVisible();
  const box = (await bar.boundingBox())!;
  expect(box.height).toBe(settingsBar.height);
  const back = await backOf(page);
  expect(back.x).toBe(reference.x);
  expect(back.width).toBe(reference.width);
  // The tabs sit on the bar's bottom rule.
  const tab = (await page.getByRole("tab").first().boundingBox())!;
  expect(Math.round(tab.y + tab.height)).toBe(Math.round(box.y + box.height));
});

test("Tickets has one bar: the project is a pill in the breadcrumb, New ticket and the layouts on the right", async ({ page }) => {
  await installBoardApi(page);
  for (const layout of ["", "&layout=board", "&layout=epics"]) {
    await page.goto(`/tickets?project=WEB${layout}`);
    const bar = page.locator("header").filter({ has: page.getByRole("navigation", { name: "Breadcrumb" }) });
    await expect(bar.getByRole("navigation", { name: "Breadcrumb" }).getByRole("button", { name: "Switch project, current: Web app" })).toBeVisible();
    await expect(bar.getByRole("navigation", { name: "Tickets layout" })).toBeVisible();
    await expect(bar.getByRole("link", { name: /New ticket/ })).toBeVisible();
    // Nothing else holds the switcher: no second row for it.
    await expect(page.getByRole("button", { name: /^Switch project/ })).toHaveCount(1);
  }
});

test("an editor's dock continues the top bar: the chat's header is the dock's first row, the messages right under it", async ({ page }) => {
  await installDeck(page, [{ seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" }]);
  const dock = page.getByRole("complementary", { name: "Panel", exact: true });
  const row = dock.getByTestId("dock-header");
  await expect(row.getByTestId("chat-header")).toBeVisible();
  const editorBar = (await page.locator("[data-editor-layout] header").first().boundingBox())!;
  const dockRow = (await row.boundingBox())!;
  expect(dockRow.y).toBe(editorBar.y);
  expect(dockRow.height).toBe(editorBar.height);
  expect(dockRow.height).toBe(44);
  // One tab, so no tab strip; and no other header between that row and the conversation.
  await expect(dock.getByRole("tablist", { name: "Panel tabs" })).toHaveCount(0);
  await expect(dock.getByTestId("chat-header")).toHaveCount(1);
  const panel = (await dock.getByRole("tabpanel", { name: "Chat" }).first().boundingBox())!;
  expect(Math.round(panel.y)).toBe(Math.round(dockRow.y + dockRow.height));
  // The app doesn't name itself again in its own dock.
  await expect(row).not.toContainText("Slides /");
});
