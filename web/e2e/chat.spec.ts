import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const picker = (page: Page) => page.getByRole("button", { name: /Model for the next message/ });

const chips = (page: Page) => page.getByTestId("context-chip");

async function newChat(page: Page) {
  await page.getByRole("button", { name: "New chat" }).first().click();
  // The route swap finishes once the composer of the new chat is there.
  await expect(composer(page)).toBeVisible();
}

async function contextCaretReady(page: Page) {
  // Picking context restores focus/caret on the next paint. Wait for that
  // transition before simulating the next burst of typing.
  await composer(page).evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
  await expect(composer(page)).toBeFocused();
  await expect.poll(() => composer(page).evaluate((el: HTMLTextAreaElement) => el.selectionStart === el.value.length)).toBe(true);
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

test("[[ picks context as removable chips and sends them as context_refs", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);

  // Typing [[ offers pages, artifacts and tickets; picking adds a chip, not text.
  await composer(page).fill("see [[Pla");
  const list = page.getByRole("listbox", { name: "Link to" });
  await expect(list.getByRole("option")).toHaveCount(1);
  await list.getByRole("option", { name: /Eng\/Plan/ }).click();
  await expect(chips(page)).toHaveCount(1);
  await expect(chips(page).first()).toContainText("Eng/Plan");
  await expect(composer(page)).toHaveValue("see ");

  // Picking restores the caret on the next frame; don't type into that restoration.
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));

  // Enter on the highlighted suggestion picks it with the keyboard.
  await contextCaretReady(page);
  await composer(page).pressSequentially("[[WEB-");
  await expect(page.getByRole("option", { name: /WEB-3/ })).toBeVisible();
  await composer(page).press("Enter");
  await expect(chips(page)).toHaveCount(2);

  // "Add context" opens the same picker.
  await page.getByRole("button", { name: "Add context" }).click();
  await page.getByRole("combobox", { name: "Search pages, artifacts and tickets" }).fill("Q3");
  await page.getByRole("option", { name: /Q3 report/ }).click();
  await expect(chips(page)).toHaveCount(3);

  // Picking the same thing twice doesn't duplicate; chips can be removed.
  await contextCaretReady(page);
  await composer(page).pressSequentially("[[WEB-");
  await composer(page).press("Enter");
  await expect(chips(page)).toHaveCount(3);
  await page.getByRole("button", { name: "Remove Q3 report" }).click();
  await expect(chips(page)).toHaveCount(2);

  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: see");
  expect(mock.posted.at(-1)?.body.context_refs).toEqual([
    { kind: "page", id: "11111111-1111-4111-8111-111111111111" },
    { kind: "ticket", key: "WEB-3" },
  ]);
  // Sent messages show read-only chips; the composer's chips are cleared.
  await expect(chips(page)).toHaveCount(0);
  await expect(page.getByTestId("user-message").getByTestId("sent-chip")).toHaveText(["Eng/Plan", "WEB-3"]);

  // Without attachments no context_refs are sent.
  await send(page, "plain");
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: plain");
  expect(mock.posted.at(-1)?.body).not.toHaveProperty("context_refs");
});

test("a rejected attachment shows the server error and keeps the chips", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await page.goto(page.url() + "&attach=ticket:OTHER-9&attach_title=Elsewhere");
  await expect(chips(page)).toHaveCount(1);
  await send(page, "hi");
  await expect(page.getByRole("alert").filter({ hasText: "OTHER-9" })).toContainText("is not in this chat's project");
  await expect(chips(page)).toHaveCount(1);
  await expect(composer(page)).toHaveValue("hi");
});

test("Start chat links pre-attach the item and drop the query param", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB&attach=page:11111111-1111-4111-8111-111111111111&attach_title=Plan");
  await page.getByRole("button", { name: "New chat" }).first().click();
  await expect(chips(page)).toHaveCount(1);
  await expect(chips(page).first()).toContainText("Plan");
  await expect(page).not.toHaveURL(/attach/);
  await send(page, "summarise");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: summarise");
  expect(mock.posted.at(-1)?.body.context_refs).toEqual([{ kind: "page", id: "11111111-1111-4111-8111-111111111111" }]);
});

test("assistant messages list deduplicated Sources with version and section", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await send(page, "plain");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: plain");
  await expect(page.getByTestId("sources")).toHaveCount(0);

  await send(page, "cite: risks");
  const sources = page.getByTestId("assistant-message").nth(1).getByTestId("sources");
  await expect(sources.getByTestId("source")).toHaveCount(3);
  const report = sources.getByRole("link", { name: /Q3 report/ });
  await expect(report).toContainText("v2");
  await expect(report).toContainText("risks");
  await expect(report).toHaveAttribute("href", "/artifacts/22222222-2222-4222-8222-222222222222?version=55555555-5555-4555-8555-555555555555#risks");
  await expect(sources.getByRole("link", { name: /Eng\/Plan/ })).toHaveAttribute("href", "/wiki/Eng/Plan");
  await expect(sources.getByRole("link", { name: /WEB-3/ })).toHaveAttribute("href", "/tickets/WEB-3");
});

test("save a reply to the wiki with a link back to the chat", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await send(page, "plain");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: plain");

  await page.getByRole("button", { name: "Save to wiki" }).click();
  const path = page.getByRole("textbox", { name: "Page path" });
  await expect(path).toHaveValue("Chats/plain");
  await path.fill("Notes/From chat");
  await page.getByRole("button", { name: "Save page" }).click();
  await expect(page.getByRole("link", { name: "Open Notes/From chat" })).toHaveAttribute("href", "/wiki/Notes/From%20chat");
  const chatId = mock.posted[0].chat;
  expect(mock.wikiPages).toHaveLength(1);
  expect(mock.wikiPages[0].project_key).toBe("WEB");
  expect(mock.wikiPages[0].body_md).toContain("Echo: plain");
  expect(mock.wikiPages[0].body_md).toContain(`(/chat/${chatId})`);
  await page.getByRole("button", { name: "Done" }).click();

  // A taken path shows the server's error and keeps the dialog open.
  await page.getByRole("button", { name: "Save to wiki" }).click();
  await page.getByRole("textbox", { name: "Page path" }).fill("Notes/From chat");
  await page.getByRole("button", { name: "Save page" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "already exists" })).toBeVisible();
});

test("save a reply as a report artifact with a link back to the chat", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await send(page, "plain");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: plain");
  await page.getByRole("button", { name: "Save as artifact" }).click();
  await page.getByRole("textbox", { name: "Title" }).fill("Plain answer");
  await page.getByRole("button", { name: "Save report" }).click();
  await expect(page.getByRole("link", { name: "Open Plain answer" })).toHaveAttribute("href", "/artifacts/66666666-6666-4666-8666-666666666666");
  const form = mock.artifactForms[0];
  expect(form).toContain("report");
  expect(form).toContain("Echo: plain");
  expect(form).toContain(`(/chat/${mock.posted[0].chat})`);
});

test("OpenCode model is chosen with the keyboard, posted, and survives reload", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await picker(page).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeVisible();
  const target = page.getByRole("menuitem", { name: /OpenCode openai\/gpt-5/ });
  // Walk down the menu until the target item has focus.
  for (let i = 0; i < 8 && !(await target.evaluate((el) => el === document.activeElement)); i++) {
    await page.keyboard.press("ArrowDown");
    await page.waitForTimeout(50);
  }
  await expect(target).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(picker(page)).toHaveAttribute("aria-label", /OpenCode, openai\/gpt-5/);
  await send(page, "hello opencode");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello opencode");
  expect(mock.posted.at(-1)?.body).toMatchObject({ harness: "opencode", model: "openai/gpt-5" });
  await expect(page.getByTestId("model-tag")).toHaveText("OpenCode · openai/gpt-5");

  await page.reload();
  await expect(picker(page)).toHaveAttribute("aria-label", /OpenCode, openai\/gpt-5/);
  await expect(page.getByTestId("model-tag")).toHaveText("OpenCode · openai/gpt-5");
  expect(mock.unhandled).toEqual([]);
});

test("unavailable OpenCode shows its reason and can't be selected", async ({ page }) => {
  await installChatApi(page, { opencodeAvailable: false });
  await page.goto("/chat");
  await newChat(page);
  await picker(page).click();
  const item = page.getByRole("menuitem", { name: /OpenCode anthropic\/claude-sonnet-4-5/ });
  await expect(item).toHaveAttribute("aria-disabled", "true");
  await expect(item).toContainText("OpenCode is not installed");
  await item.click({ force: true });
  await page.keyboard.press("Escape");
  await expect(picker(page)).toHaveAttribute("aria-label", /Claude Code/);
});

test("a rejected OpenCode post shows the error and keeps the selection", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat");
  await newChat(page);
  await page.route("**/api/v1/chats/*/messages", (route) =>
    route.request().method() === "POST"
      ? route.fulfill({ status: 422, json: { code: "invalid", message: "Unknown or unavailable harness/model", field: "model" } })
      : route.fallback(),
  );
  await picker(page).click();
  await page.getByRole("menuitem", { name: /OpenCode anthropic\/claude-sonnet-4-5/ }).click();
  await send(page, "will fail");
  await expect(page.getByRole("alert").filter({ hasText: "Unknown or unavailable" })).toBeVisible();
  await expect(picker(page)).toHaveAttribute("aria-label", /OpenCode, anthropic\/claude-sonnet-4-5/);
  await expect(composer(page)).toHaveValue("will fail");
  expect(mock.posted).toEqual([]);
});
