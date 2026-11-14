import { expect, test } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

// The ticket's side panel switches between its properties and the work chat with your persona,
// which says which ticket the next message is about; the choice is remembered.
test("the ticket's side panel switches to Chat, which names the ticket, and remembers it", async ({ page }) => {
  await installBoardApi(page);
  await page.route("**/api/v1/dock-chats?*", (route) => route.fulfill({ json: { chats: [] } }));
  await page.goto("/tickets/WEB-1");
  const panel = page.getByRole("complementary", { name: "Properties" });
  const tabs = panel.getByRole("tablist", { name: "Side panel" });
  await expect(tabs.getByRole("tab", { name: "Properties" })).toHaveAttribute("aria-selected", "true");
  await tabs.getByRole("tab", { name: "Chat" }).click();
  await expect(panel.getByTestId("work-chat-subject")).toHaveText("Viewing: WEB-1 · Build ticket modal");
  await expect(panel.getByRole("button", { name: "Start a chat" })).toBeVisible();
  await page.reload();
  await expect(tabs.getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");
  await tabs.getByRole("tab", { name: "Properties" }).click();
  await expect(panel.getByTestId("work-chat")).toHaveCount(0);
});
