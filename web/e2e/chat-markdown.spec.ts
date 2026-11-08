import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// GitHub-flavored markdown in a reply renders as real elements, not literal pipes.

test("tables, task lists, strikethrough and quotes render", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("gfm: sample");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");

  const table = page.getByRole("table");
  await expect(table.getByRole("columnheader", { name: "Option" })).toBeVisible();
  await expect(table.getByRole("row")).toHaveCount(3);
  await expect(table.getByRole("row", { name: "Cartesia" }).getByRole("cell", { name: "$0.011" })).toHaveCSS("text-align", "right");

  await expect(page.getByRole("checkbox").first()).toBeChecked();
  await expect(page.getByRole("checkbox").nth(1)).not.toBeChecked();
  await expect(page.locator("del", { hasText: "old" })).toBeVisible();
  await expect(page.locator("blockquote", { hasText: "Quoted" })).toBeVisible();
});
