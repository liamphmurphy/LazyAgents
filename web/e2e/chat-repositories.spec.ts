import { expect, test } from "@playwright/test";
import { composerOption, installChatApi, openEmptyChat, persona } from "./mock-chat-api";

test("a repository-less coding chat attaches unlisted repositories without replacing the conversation", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: false }, repos: [] })] });
  const id = await openEmptyChat(page);
  const composer = page.getByRole("combobox", { name: "Message" });
  await composer.fill("Keep this conversation");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByText("Echo: Keep this conversation", { exact: true })).toBeVisible();
  const picker = page.getByTestId("repository-picker").first();
  await picker.getByRole("button", { name: /^Repository:/ }).click();
  await page.getByRole("button", { name: "Other repository…" }).click();
  await page.getByLabel("Repository name or GitHub URL").fill("acme/unlisted");
  await page.getByRole("button", { name: "Use repository" }).click();
  await expect(picker).toContainText("acme/unlisted");
  await composer.fill("/repo acme/second");
  await composer.press("Enter");
  await expect(picker).toContainText("acme/second");
  await composer.fill("/repo acme/unlisted");
  await composer.press("Enter");
  await expect(picker).toContainText("acme/unlisted");
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
  await page.getByRole("button", { name: "Other repository…" }).click();
  await page.getByLabel("Repository name or GitHub URL").fill("acme/next");
  await page.getByRole("button", { name: "Use repository" }).click();
  await expect(picker).toContainText("acme/next (next turn)");
  expect(mock.chats[0].repo_url).toBeUndefined();
  expect(mock.unhandled).toEqual([]);
});

test("the repository chip sits in the Welcome's + menu, lists the persona's repos, and always offers Other repository", async ({ page }) => {
  const repos = ["https://github.com/acme/api.git", "git@github.com:acme/app.git"];
  const mock = await installChatApi(page, { personas: [persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: false }, repos })] });
  await page.goto("/chat?project=WEB");
  // On the Welcome, which has no header, it's in the composer's + menu; in a chat, in the header.
  const chip = (await composerOption(page, "repository")).getByTestId("repository-picker");
  await expect(chip).toContainText("acme/api");
  await chip.getByRole("button", { name: /^Repository:/ }).click();
  const menu = page.getByRole("dialog").filter({ has: page.getByRole("region", { name: "Dev’s repositories" }) });
  await expect(menu.getByRole("region", { name: "Dev’s repositories" }).getByRole("button")).toHaveText([/acme\/api/, /acme\/app/]);
  await expect(menu.getByRole("button", { name: /acme\/api/ })).toHaveAttribute("aria-current", "true");
  await expect(menu.getByRole("button", { name: "Other repository…" })).toBeVisible();
  await menu.getByRole("button", { name: /acme\/app/ }).click();
  await expect(chip).toContainText("acme/app");
  await page.getByRole("combobox", { name: "Message" }).fill("fix the build");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[^/]+/);
  // Still in the composer once the chat exists, and the old header pill is gone.
  await expect(page.getByTestId("repository-picker")).toHaveCount(1);
  await expect(page.getByTestId("repository-picker").first()).toContainText("acme/app");
  expect(mock.created[0]).toMatchObject({ repo_url: repos[1] });
  expect(mock.unhandled).toEqual([]);
});

test("a persona that can't write code has no repository chip", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [persona(1, { name: "Writer", is_default: true, capabilities: { code: false, images: false }, repos: ["https://github.com/acme/api"] })] });
  await page.goto("/chat?project=WEB");
  await expect(page.getByRole("combobox", { name: "Message" })).toBeVisible();
  await expect(page.getByTestId("repository-picker")).toHaveCount(0);
  expect(mock.unhandled).toEqual([]);
});
