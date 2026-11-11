import { expect, test } from "@playwright/test";
import { installChatApi, openEmptyChat, persona } from "./mock-chat-api";

test("a repository-less coding chat attaches unlisted repositories without replacing the conversation", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: false }, repos: [] })] });
  const id = await openEmptyChat(page);
  const composer = page.getByRole("combobox", { name: "Message" });
  await composer.fill("Keep this conversation");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText("Echo: Keep this conversation", { exact: true })).toBeVisible();
  const picker = page.getByTestId("repository-picker").first();
  await picker.getByRole("button", { name: /^Repository:/ }).click();
  await picker.getByLabel("Repository name or GitHub URL").fill("acme/unlisted");
  await picker.getByRole("button", { name: "Use repository" }).click();
  await expect(picker).toContainText("Repository: acme/unlisted");
  await composer.fill("/repo acme/second");
  await composer.press("Enter");
  await expect(picker).toContainText("Repository: acme/second");
  await composer.fill("/repo acme/unlisted");
  await composer.press("Enter");
  await expect(picker).toContainText("Repository: acme/unlisted");
  await expect(page).toHaveURL(new RegExp(`/chat/${id}$`));
  await expect(page.getByText("Echo: Keep this conversation", { exact: true })).toBeVisible();
  expect(mock.created).toHaveLength(1);
  expect(mock.chats[0].repositories).toHaveLength(2);
  expect(mock.unhandled).toEqual([]);
  await page.screenshot({ path: "/tmp/vesper-repository-picker.png", fullPage: true });
});

test("selecting a repository during a reply displays the queued selection", async ({ page }) => {
  const mock = await installChatApi(page, { replyDelayMs: 10000, personas: [persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: false }, repos: [] })] });
  await openEmptyChat(page);
  const composer = page.getByRole("combobox", { name: "Message" });
  await composer.fill("hello"); await page.getByRole("button", { name: "Send" }).click();
  await expect.poll(() => mock.chats[0].state).toBe("running");
  const picker = page.getByTestId("repository-picker").first();
  await picker.getByRole("button", { name: /^Repository:/ }).click();
  await picker.getByLabel("Repository name or GitHub URL").fill("acme/next");
  await picker.getByRole("button", { name: "Use repository" }).click();
  await expect(picker).toContainText("acme/next (next turn)");
  expect(mock.chats[0].repo_url).toBeUndefined();
  expect(mock.unhandled).toEqual([]);
});
