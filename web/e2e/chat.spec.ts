import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

const composer = (page: Page) => page.getByRole("textbox", { name: "Message" });
const picker = (page: Page) => page.getByRole("button", { name: /Model for the next message/ });

async function newChat(page: Page) {
  await page.getByRole("button", { name: "New chat" }).first().click();
  // The route swap finishes once the composer of the new chat is there.
  await expect(composer(page)).toBeVisible();
}

async function send(page: Page, text: string) {
  await composer(page).fill(text);
  await composer(page).press("Enter");
}

test("create, send, switch model, reload keeps history", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await expect(page.getByRole("navigation", { name: "Chats" }).getByText("No chats yet")).toBeVisible();

  await newChat(page);
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(page.getByText("Ask anything about this project")).toBeVisible();
  await expect(composer(page)).toBeFocused();

  // Enter sends, Shift+Enter adds a line.
  await composer(page).fill("hello");
  await composer(page).press("Shift+Enter");
  await composer(page).pressSequentially("world");
  await expect(composer(page)).toHaveValue("hello\nworld");
  await composer(page).press("Enter");
  await expect(composer(page)).toBeDisabled();
  await expect(page.getByText("Thinking…")).toBeVisible();
  const first = page.getByTestId("assistant-message").first();
  await expect(first).toContainText("Echo: hello");
  await expect(first.getByTestId("model-tag")).toHaveText("Claude Code · Opus 5.5");
  await expect(composer(page)).toBeEnabled();
  await expect(composer(page)).toBeFocused();

  // Switch model: a pending indicator shows until the message is sent.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await expect(page.getByTestId("pending-switch")).toContainText("Next message will use Codex · gpt-5-codex");
  await send(page, "second");
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: second");
  await expect(page.getByTestId("assistant-message").nth(1).getByTestId("model-tag")).toHaveText("Codex · gpt-5-codex");
  await expect(page.getByTestId("system-message")).toHaveText("Switched to Codex · gpt-5-codex");
  await expect(page.getByTestId("pending-switch")).toHaveCount(0);

  // History survives a reload.
  await page.reload();
  await expect(page.getByRole("navigation", { name: "Chats" }).getByRole("link", { name: /hello/ })).toBeVisible();
  await expect(page.getByTestId("user-message")).toHaveCount(2);
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: second");
  expect(mock.unhandled).toEqual([]);
});

test("stop re-enables the composer", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await send(page, "slow: take your time");
  await expect(composer(page)).toBeDisabled();
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(composer(page)).toBeEnabled();
  await expect(composer(page)).toBeFocused();
  await expect(page.getByTestId("system-message")).toHaveText("Turn cancelled");
});

test("unavailable models are disabled with their reason", async ({ page }) => {
  await installChatApi(page, { codexAvailable: false });
  await page.goto("/chat");
  await newChat(page);
  await picker(page).click();
  const item = page.getByRole("menuitem", { name: /Codex gpt-5-codex/ });
  await expect(item).toHaveAttribute("aria-disabled", "true");
  await expect(item).toContainText("No harness:openai secret");
});

test("rename and archive from the history list", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await page.getByRole("button", { name: "Rename New chat" }).click();
  await page.getByRole("textbox", { name: "Chat title" }).fill("Roadmap");
  await page.getByRole("textbox", { name: "Chat title" }).press("Enter");
  const nav = page.getByRole("navigation", { name: "Chats" });
  await expect(nav.getByRole("link", { name: "Roadmap" })).toBeVisible();
  await page.getByRole("button", { name: "Archive Roadmap" }).click();
  await expect(page).toHaveURL(/\/chat\?project=WEB/);
  await expect(page.getByRole("navigation", { name: "Chats" }).getByText("No chats yet")).toBeVisible();
});
