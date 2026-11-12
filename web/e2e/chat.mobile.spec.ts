import { expect, test } from "@playwright/test";
import { installChatApi, openEmptyChat } from "./mock-chat-api";

// Mobile smoke (project "mobile", 390×844): a chat opens with its composer in view and no sideways scroll.
test("chat fits the phone viewport", async ({ page }) => {
  await installChatApi(page);
  await openEmptyChat(page);
  const composer = page.getByRole("combobox", { name: "Message" });
  await expect(composer).toBeVisible();
  await expect(composer).toBeInViewport();
  expect(page.viewportSize()).toEqual({ width: 390, height: 844 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
});
