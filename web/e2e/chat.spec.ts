import { expect, test, type Page } from "@playwright/test";
import { catalog, installChatApi } from "./mock-chat-api";

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const picker = (page: Page) => page.getByRole("button", { name: /Model for the next message/ });
const effort = (page: Page) => page.getByRole("button", { name: /Effort for the next message/ });

const chips = (page: Page) => page.getByTestId("context-chip");

// Starts a conversation from the Welcome (or the conversation header).
async function newChat(page: Page) {
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
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
  await page.goto("/chat?project=WEB");
  // First visit: no conversations yet, so the Welcome greets the person.
  await expect(page.getByRole("heading", { name: "Hi Liam, what can I help with?" })).toBeVisible();
  await expect(page.getByRole("list", { name: "Suggestions" }).getByRole("button")).toHaveCount(4);
  await expect(page.getByRole("textbox", { name: "Message" })).toBeVisible();

  await newChat(page);
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(page.getByText("A new conversation")).toBeVisible();
  await expect(composer(page)).toBeFocused();

  // Enter sends, Shift+Enter adds a line.
  await composer(page).fill("hello");
  await composer(page).press("Shift+Enter");
  await composer(page).pressSequentially("world");
  await expect(composer(page)).toHaveValue("hello\nworld");
  await composer(page).press("Enter");
  await expect(page.getByRole("button", { name: "Interrupt" })).toBeVisible();
  await expect(page.getByTestId("chat-status")).toHaveText(/ is thinking…$/);
  await expect(page.locator('[data-scene="thinking"]')).toBeAttached();
  const first = page.getByTestId("assistant-message").first();
  await expect(first).toContainText("Echo: hello");
  // The chat reads as a plain conversation: the model tag lives in the Activity tab.
  await expect(first.getByTestId("model-tag")).toHaveCount(0);
  await expect(composer(page)).toBeEnabled();
  await expect(composer(page)).toBeFocused();

  // Switch model: another harness asks first, then a pending indicator shows until the message is sent.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await page.getByRole("dialog", { name: "Switch to Codex · gpt-5-codex?" }).getByRole("button", { name: "Switch harness" }).click();
  await expect(page.getByTestId("pending-switch")).toContainText("Next message will use Codex · gpt-5-codex");
  await send(page, "second");
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: second");
  await expect(page.getByTestId("system-message")).toHaveText("Switched to Codex · gpt-5-codex");
  await expect(page.getByTestId("pending-switch")).toHaveCount(0);
  // Each turn's model shows in the Activity tab.
  await page.getByRole("tab", { name: "Activity" }).click();
  await expect(page.getByTestId("activity-turn").nth(0).getByTestId("model-tag")).toHaveText("Claude Code · Opus 5.5");
  await expect(page.getByTestId("activity-turn").nth(1).getByTestId("model-tag")).toHaveText("Codex · gpt-5-codex");
  await page.getByRole("tab", { name: "Chat" }).click();

  // History survives a reload.
  await page.reload();
  await expect(page.getByTestId("user-message")).toHaveCount(2);
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: second");

  // Revisiting the assistant resumes the newest conversation instead of the Welcome.
  const chatId = mock.posted[0].chat;
  await page.goto("/chat?project=WEB");
  await expect(page).toHaveURL(new RegExp(`/chat/${chatId}`));
  await expect(page.getByTestId("user-message")).toHaveCount(2);
  await expect(page.getByRole("heading", { name: /what can I help with/ })).toHaveCount(0);
  expect(mock.unhandled).toEqual([]);
});

test("effort is picked per message, kept on the chat, and limited to what the model offers", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await expect(effort(page)).toHaveAttribute("aria-label", "Effort for the next message: Default");
  await send(page, "hello");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello");
  expect(mock.posted.at(-1)?.body).not.toHaveProperty("effort");

  await effort(page).click();
  await expect(page.getByRole("menuitem")).toHaveText(["Default", "Low", "Medium", "High", "Extra high", "Max"]);
  await page.getByRole("menuitem", { name: "Max" }).click();
  await expect(page.getByTestId("pending-effort")).toHaveText("Next message will use max effort");
  await send(page, "think hard");
  await expect(page.getByTestId("assistant-message").nth(1)).toContainText("Echo: think hard");
  expect(mock.posted.at(-1)?.body).toMatchObject({ effort: "max" });
  await expect(page.getByTestId("pending-effort")).toHaveCount(0);
  await expect(effort(page)).toHaveAttribute("aria-label", "Effort for the next message: Max");

  // Unchanged effort isn't resent.
  await send(page, "again");
  await expect(page.getByTestId("assistant-message").nth(2)).toContainText("Echo: again");
  expect(mock.posted.at(-1)?.body).not.toHaveProperty("effort");

  // Codex here offers low–high, with its own default named: max falls back to Default.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await page.getByRole("dialog", { name: "Switch to Codex · gpt-5-codex?" }).getByRole("button", { name: "Switch harness" }).click();
  await expect(effort(page)).toHaveAttribute("aria-label", "Effort for the next message: Default");
  await effort(page).click();
  await expect(page.getByRole("menuitem")).toHaveText(["Default(medium)", "Low", "Medium", "High"]);
  await page.keyboard.press("Escape");
  await send(page, "on codex");
  await expect(page.getByTestId("assistant-message").nth(3)).toContainText("Echo: on codex");
  expect(mock.posted.at(-1)?.body).toMatchObject({ harness: "codex", effort: "default" });

  // A model with no effort control hides the picker.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /OpenCode openai\/gpt-5/ }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Switch harness" }).click();
  await expect(effort(page)).toHaveCount(0);
  expect(mock.unhandled).toEqual([]);
});

test("stop ends a running reply", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "slow: take your time");
  await expect(composer(page)).toBeEnabled();
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(composer(page)).toBeEnabled();
  await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  await expect(page.getByTestId("system-message")).toHaveText("Turn cancelled");
});

test("a message sent while a reply runs interrupts it, and the next reply answers both", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "slow: summarize the plan");
  await expect(page.getByTestId("chat-status")).toBeVisible();
  // The composer stays open; the model and effort wait for the reply to end.
  await expect(composer(page)).toBeEnabled();
  await expect(composer(page)).toHaveAttribute("placeholder", /is replying\. Send a message to interrupt/);
  await expect(picker(page)).toBeDisabled();
  await composer(page).fill("focus on the risks");
  await page.getByRole("button", { name: "Interrupt" }).click();
  await expect(page.getByTestId("assistant-message").last()).toContainText("Echo: focus on the risks (interrupting: slow: summarize the plan)");
  await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  expect(mock.posted.map((p) => p.body.body)).toEqual(["slow: summarize the plan", "focus on the risks"]);
  expect(mock.unhandled).toEqual([]);
});

test("unavailable models are disabled with their reason", async ({ page }) => {
  await installChatApi(page, { codexAvailable: false });
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await picker(page).click();
  const item = page.getByRole("menuitem", { name: /Codex gpt-5-codex/ });
  await expect(item).toHaveAttribute("aria-disabled", "true");
  await expect(item).toContainText("No harness:openai secret");
});

test("[[ picks context as removable chips and sends them as context_refs", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
  await newChat(page);
  // Opened on a conversation, the item is attached to it and ?attach= leaves the URL.
  await page.goto(page.url() + "&attach=page:11111111-1111-4111-8111-111111111111&attach_title=Plan");
  await expect(chips(page)).toHaveCount(1);
  await expect(chips(page).first()).toContainText("Plan");
  await expect(page).not.toHaveURL(/attach/);
  await send(page, "summarise");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: summarise");
  expect(mock.posted.at(-1)?.body.context_refs).toEqual([{ kind: "page", id: "11111111-1111-4111-8111-111111111111" }]);
});

test("sending from the Welcome creates the chat, posts the message and opens it", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await expect(page.getByRole("heading", { name: "Hi Liam, what can I help with?" })).toBeVisible();
  // A suggestion fills the composer; it is not sent.
  await page.getByRole("list", { name: "Suggestions" }).getByRole("button", { name: "Draft tickets from a spec" }).click();
  await expect(page.getByRole("textbox", { name: "Message" })).toHaveValue("Draft tickets from a spec");
  expect(mock.posted).toEqual([]);

  await page.getByRole("textbox", { name: "Message" }).fill("first words");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+\?project=WEB/);
  await expect(page.getByTestId("user-message")).toContainText("first words");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: first words");
  expect(mock.created).toEqual([{ project: "WEB" }]);
  expect(mock.posted).toHaveLength(1);
  expect(mock.posted[0].body).toEqual({ body: "first words" });
  expect(mock.unhandled).toEqual([]);
});

test("Start chat ?attach= on /chat shows chips in the Welcome and sends them with the first message", async ({ page }) => {
  const mock = await installChatApi(page);
  // An existing conversation doesn't hide the Welcome when context was brought along.
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "earlier");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: earlier");

  await page.goto("/chat?project=WEB&attach=page:11111111-1111-4111-8111-111111111111&attach_title=Plan");
  await expect(page.getByRole("heading", { name: /what can I help with/ })).toBeVisible();
  await expect(chips(page)).toHaveCount(1);
  await expect(chips(page).first()).toContainText("Plan");
  await page.getByRole("textbox", { name: "Message" }).fill("summarise");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+\?project=WEB/);
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: summarise");
  expect(mock.posted.at(-1)?.body).toEqual({ body: "summarise", context_refs: [{ kind: "page", id: "11111111-1111-4111-8111-111111111111" }] });
  await expect(page.getByTestId("user-message").getByTestId("sent-chip")).toHaveCount(1);
  expect(mock.unhandled).toEqual([]);
});

test("assistant messages list deduplicated Sources with version and section", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
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
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await picker(page).click();
  const item = page.getByRole("menuitem", { name: /OpenCode anthropic\/claude-sonnet-4-5/ });
  await expect(item).toHaveAttribute("aria-disabled", "true");
  await expect(item).toContainText("OpenCode is not installed");
  await item.click({ force: true });
  await page.keyboard.press("Escape");
  await expect(picker(page)).toHaveAttribute("aria-label", /Claude Code/);
});

test("with no harness available, chat says why before anything is sent", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  // The Anthropic key is removed: every harness becomes unavailable.
  await page.route("**/api/v1/chat-models", (route) =>
    route.fulfill({ json: catalog(false, false).map((g) => ({ ...g, available: false, unavailable_reason: g.unavailable_reason ?? "no harness:anthropic secret" })) }),
  );
  await page.reload();
  const guard = page.getByRole("status").filter({ hasText: "Claude Code is unavailable" });
  await expect(guard).toContainText("no harness:anthropic secret. Pick another model or add the key.");
  await expect(guard.getByRole("link", { name: "Add the key →" })).toHaveAttribute("href", "/settings#secrets");
  await composer(page).fill("hello");
  await expect(page.getByRole("button", { name: "Send" })).toBeDisabled();
  await composer(page).press("Enter");
  expect(mock.posted).toEqual([]);
  // New conversation is refused up front, naming the missing secrets.
  await expect(page.getByRole("button", { name: "New conversation", exact: true }).first()).toBeDisabled();
  await expect(page.locator('p[role="status"]').filter({ hasText: "No harness is available: add harness:anthropic or harness:openai." })).toBeVisible();
  // The shell banner names the same secrets on every screen.
  await expect(page.getByRole("status", { name: /Agents can’t run/ })).toBeVisible();
});

test("a rejected OpenCode post shows the error and keeps the selection", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
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

test("a new chat has no agent picker and names no agent", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await expect(page.getByRole("button", { name: /Agent for new chats/ })).toHaveCount(0);
  await newChat(page);
  expect(mock.created.at(-1)).toEqual({ project: "WEB" });
  await expect(page.getByTestId("chat-agent")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "Breadcrumb" }).getByText("Assistant", { exact: true })).toBeVisible();
  expect(mock.unhandled).toEqual([]);
});

// Next keeps the page we navigated from mounted but hidden, so scope to what is on screen.
const history = (page: Page) => page.locator('aside[aria-label="Chat history"]:visible');
const historyNav = (page: Page) => history(page).getByRole("navigation", { name: "Chats" });

test("rename and archive from the history list", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  // A conversation nobody has named yet reads as New conversation.
  await expect(historyNav(page).getByRole("link", { name: "New conversation" })).toBeVisible();
  await page.getByRole("button", { name: "Rename New conversation" }).click();
  const title = page.getByRole("textbox", { name: "Conversation title" });
  await expect(title).toHaveValue("");
  await expect(title).toHaveAttribute("placeholder", "New conversation");
  await title.fill("Roadmap");
  await title.press("Enter");
  await expect(historyNav(page).getByRole("link", { name: "Roadmap" })).toBeVisible();
  await page.getByRole("button", { name: "Archive Roadmap" }).click();
  await expect(page).toHaveURL(/\/chat\?project=WEB/);
  await expect(historyNav(page).getByText("No conversations yet.")).toBeVisible();
});

test("delete asks first, Cancel keeps the conversation, and deleting the open one leaves it", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "delete me");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: delete me");
  const nav = historyNav(page);
  await nav.getByRole("button", { name: "Delete delete me" }).click();
  const confirm = nav.getByRole("group", { name: "Confirm delete delete me" });
  await expect(confirm).toContainText("This can’t be undone.");
  await confirm.getByRole("button", { name: "Cancel" }).click();
  await expect(nav.getByRole("link", { name: "delete me" })).toBeVisible();

  await nav.getByRole("button", { name: "Delete delete me" }).click();
  await nav.getByRole("group", { name: "Confirm delete delete me" }).getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page).toHaveURL(/\/chat\?project=WEB$/);
  await expect(nav.getByText("No conversations yet.")).toBeVisible();
});

test("archived conversations are listed under Archived and can be restored", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "keep me");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: keep me");
  const nav = historyNav(page);
  await expect(nav.getByRole("link", { name: "keep me" })).toBeVisible();
  await page.getByRole("button", { name: "Archive keep me" }).click();
  await expect(page).toHaveURL(/\/chat\?project=WEB/);
  await expect(nav.getByText("No conversations yet.")).toBeVisible();

  await history(page).getByRole("radio", { name: "Archived" }).click();
  await expect(nav.getByRole("link", { name: "keep me" })).toBeVisible();
  await expect(nav.getByText("Archived conversations are read-only")).toBeVisible();
  await expect(nav.getByRole("button", { name: "Archive keep me" })).toHaveCount(0);

  // Opened, an archived conversation is read-only with Restore in the header and in place of the composer.
  await nav.getByRole("link", { name: "keep me" }).click();
  const chat = page.getByRole("region", { name: "keep me" });
  await expect(chat.getByText("This chat is archived. Restore it to continue.")).toBeVisible();
  await expect(composer(page)).toHaveCount(0);
  await expect(chat.getByRole("button", { name: "Restore keep me" })).toHaveCount(2);
  await chat.getByRole("button", { name: "Restore keep me" }).last().click();
  await expect(composer(page)).toBeVisible();
  // The restored conversation is listed under Active again, next to the open one.
  await expect(history(page).getByRole("radio", { name: "Active" })).toBeChecked();
  await expect(page).not.toHaveURL(/view=archived/);
  await expect(nav.getByRole("link", { name: "keep me" })).toBeVisible();
  await history(page).getByRole("radio", { name: "Archived" }).click();
  await expect(nav.getByText("No archived conversations.")).toBeVisible();
  await history(page).getByRole("radio", { name: "Active" }).click();
  await expect(nav.getByRole("link", { name: "keep me" })).toBeVisible();

  // Restore straight from the list too. Archiving the open chat navigates back to the index first.
  await page.getByRole("button", { name: "Archive keep me" }).click();
  await expect(nav.getByText("No conversations yet.")).toBeVisible();
  await history(page).getByRole("radio", { name: "Archived" }).click();
  await expect(page).toHaveURL(/view=archived/);
  await nav.getByRole("button", { name: "Restore keep me" }).click();
  await expect(nav.getByText("No archived conversations.")).toBeVisible();
});

test("an archived chat opened by URL is read-only and Restore brings back the composer", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "keep me");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: keep me");
  const chatId = mock.posted[0].chat;
  // The UI has no archive control; archive through the API like another client would.
  await page.evaluate(async (id) => {
    const chat = await (await fetch(`/api/v1/chats/${id}`)).json();
    await fetch(`/api/v1/chats/${id}`, {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ revision: chat.revision, archived: true }),
    });
  }, chatId);

  await page.goto(`/chat/${chatId}?project=WEB`);
  const chat = page.getByRole("region", { name: "keep me" });
  await expect(chat.getByText("This chat is archived. Restore it to continue.")).toBeVisible();
  await expect(composer(page)).toHaveCount(0);
  await expect(chat.getByRole("button", { name: "Restore keep me" })).toHaveCount(2);
  await chat.getByRole("button", { name: "Restore keep me" }).last().click();
  await expect(composer(page)).toBeVisible();
  await expect(chat.getByRole("button", { name: "Restore keep me" })).toHaveCount(0);
  expect(mock.unhandled).toEqual([]);
});

test("a running turn shows its newest tool call as a live line with a linked page", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "slow: dig in");
  const live = page.getByTestId("live-activity");
  // The chat says what the persona is doing at a high level, never the exact call;
  // tool calls and their inputs show in the Activity tab.
  await expect(page.getByTestId("chat-status")).toHaveText(/ is looking through the wiki…$/);
  await expect(page.locator('[data-scene="wiki"]')).toBeAttached();
  await expect(page.getByTestId("chat-status")).not.toContainText("Plan");
  await expect(live).toBeHidden();
  await page.getByRole("tab", { name: "Activity" }).click();
  await expect(live).toHaveText("Reading Plan…");
  await expect(live.getByRole("link", { name: /Plan/ })).toHaveAttribute("href", "/wiki/Eng/Plan");
  await expect(page.getByText("Tool calls (1)")).toBeVisible();
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(live).toHaveCount(0);
});

test("switching harness asks first and lists what is lost; same harness doesn't", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  const dialog = page.getByRole("dialog");
  const pending = page.getByTestId("pending-switch");

  // An empty chat has no session to lose.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await expect(picker(page)).toHaveAttribute("aria-label", /Codex, gpt-5-codex/);
  await expect(dialog).toHaveCount(0);
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Claude Code Opus 5.5/ }).click();
  await send(page, "hello");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello");

  // Same harness, new model: no question.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Claude Code Sonnet 5.5/ }).click();
  await expect(pending).toContainText("Claude Code · Sonnet 5.5");
  await expect(dialog).toHaveCount(0);

  // Another harness: what carries over and what is lost; Cancel keeps the previous pick.
  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await expect(dialog).toHaveAccessibleName("Switch to Codex · gpt-5-codex?");
  await expect(dialog.getByRole("region", { name: "Carries over" })).toContainText("The latest messages, up to 40 or 24,000 characters");
  await expect(dialog.getByRole("region", { name: "Is lost" })).toContainText("Claude Code’s hidden reasoning");
  await expect(dialog.getByRole("region", { name: "Is lost" })).toContainText("The prompt cache, so the first reply costs more");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(composer(page)).toBeFocused();
  await expect(pending).toContainText("Claude Code · Sonnet 5.5");

  await picker(page).click();
  await page.getByRole("menuitem", { name: /Codex gpt-5-codex/ }).click();
  await dialog.getByRole("button", { name: "Switch harness" }).click();
  await expect(pending).toContainText("Codex · gpt-5-codex");
  await expect(composer(page)).toBeFocused();
});

test("each reply shows its tokens and cost on hover or focus", async ({ page }) => {
  await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await newChat(page);
  await send(page, "hello");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello");
  await expect(page.getByTestId("assistant-message")).not.toContainText("$0.042");
  await page.getByRole("tab", { name: "Activity" }).click();
  const reply = page.getByTestId("activity-turn");
  await expect(reply).toContainText("Claude Code · Opus 5.5 · $0.042");
  const tooltip = page.getByTestId("usage-tooltip");
  await expect(tooltip).toHaveCount(0);

  await reply.getByTestId("model-tag").focus();
  await expect(tooltip).toContainText("Input48.2K41K cached · 7.2K uncached");
  await expect(tooltip).toContainText("Output1.3K");
  await expect(tooltip).toContainText("Cost$0.042");
  await page.keyboard.press("Escape");
  await expect(tooltip).toHaveCount(0);

  await reply.getByTestId("model-tag").hover();
  await expect(tooltip).toContainText("Cost$0.042");
});
