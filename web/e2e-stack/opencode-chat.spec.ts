// REAL-STACK OpenCode chat flow: no page.route / request mocks. The browser
// talks to the real Go API and runner over real Postgres. The CLIs are the
// deterministic fake-harness binary, so this is NOT evidence about the real
// opencode/claude/codex CLIs or providers (see README.md).
import { expect, test, type Page } from "@playwright/test";

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const picker = (page: Page) => page.getByRole("button", { name: /Model for the next message/ });

async function newChat(page: Page) {
  await page.goto("/chat");
  await page.getByRole("button", { name: "New chat" }).first().click();
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(composer(page)).toBeVisible();
}

async function choose(page: Page, item: string) {
  await picker(page).click();
  await page.getByRole("menuitem", { name: item, exact: true }).click();
}

async function send(page: Page, text: string) {
  await composer(page).fill(text);
  await composer(page).press("Enter");
}

const chatId = (page: Page) => new URL(page.url()).pathname.split("/").pop()!;

test("OpenCode: pick model, reply, change model, switch harness, reload keeps both segments", async ({ page }) => {
  await newChat(page);

  // Provider-qualified OpenCode models come from the real catalog endpoint.
  await choose(page, "OpenCode Claude Sonnet 5.5 (stub)");
  await expect(picker(page)).toHaveAttribute("aria-label", /OpenCode, Claude Sonnet 5.5 \(stub\)/);
  await send(page, "hello opencode");
  const replies = page.getByTestId("assistant-message");
  await expect(replies.first()).toContainText("Chat reply (resumed=false");
  await expect(replies.first()).toContainText("hello opencode");
  await expect(replies.first().getByTestId("model-tag")).toHaveText("OpenCode · Claude Sonnet 5.5 (stub)");
  await expect(composer(page)).toBeEnabled();

  // The persisted message carries the provider-qualified id the runner used.
  const first = await (await page.request.get(`/api/v1/chats/${chatId(page)}/messages`)).json();
  const firstReply = JSON.stringify(first);
  expect(firstReply).toContain('"harness":"opencode"');
  expect(firstReply).toContain("anthropic/claude-sonnet-5-5");
  expect(firstReply).toContain("model=anthropic/claude-sonnet-5-5");

  // Model change inside OpenCode starts a new segment.
  await choose(page, "OpenCode GPT-5 (stub)");
  await expect(page.getByTestId("pending-switch")).toContainText("Next message will use OpenCode · GPT-5 (stub)");
  await send(page, "second on gpt");
  await expect(replies.nth(1)).toContainText("second on gpt");
  await expect(replies.nth(1)).toContainText("model=openai/gpt-5");
  await expect(replies.nth(1).getByTestId("model-tag")).toHaveText("OpenCode · GPT-5 (stub)");

  // Harness switch OpenCode -> Claude Code (fake `claude`).
  await choose(page, "Claude Code Claude Sonnet (fake)");
  await send(page, "third on claude");
  await expect(replies.nth(2)).toContainText("third on claude");
  await expect(replies.nth(2).getByTestId("model-tag")).toHaveText("Claude Code · Claude Sonnet (fake)");

  // Reload: history persisted server-side with every harness segment.
  await page.reload();
  await expect(page.getByTestId("user-message")).toHaveCount(3);
  await expect(replies).toHaveCount(3);
  await expect(replies.nth(0).getByTestId("model-tag")).toHaveText("OpenCode · Claude Sonnet 5.5 (stub)");
  await expect(replies.nth(1).getByTestId("model-tag")).toHaveText("OpenCode · GPT-5 (stub)");
  await expect(replies.nth(2).getByTestId("model-tag")).toHaveText("Claude Code · Claude Sonnet (fake)");
  await expect(replies.nth(2)).toContainText("third on claude");
  await expect(picker(page)).toHaveAttribute("aria-label", /Claude Code, Claude Sonnet \(fake\)/);
});

test("OpenCode: cancel a running turn, then repost and get a reply", async ({ page }) => {
  await newChat(page);
  await choose(page, "OpenCode Claude Sonnet 5.5 (stub)");
  // The fake CLI blocks on the first turn of a session whose prompt is "slow".
  await send(page, "slow");
  await expect(composer(page)).toBeDisabled();
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(page.getByRole("button", { name: "Stop" })).toHaveCount(0);
  await expect(composer(page)).toBeEnabled();
  await expect(page.getByTestId("assistant-message")).toHaveCount(0);

  await send(page, "after cancel");
  const reply = page.getByTestId("assistant-message").last();
  await expect(reply).toContainText("after cancel");
  await expect(reply.getByTestId("model-tag")).toHaveText("OpenCode · Claude Sonnet 5.5 (stub)");

  await page.reload();
  await expect(page.getByTestId("user-message")).toHaveCount(2);
  await expect(page.getByTestId("assistant-message")).toHaveCount(1);
  await expect(page.getByTestId("assistant-message").last()).toContainText("after cancel");
});
