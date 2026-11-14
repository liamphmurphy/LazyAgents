import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// History lives in the expanded menu.
test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

// Search, edit and retry, copying, selection quotes and following new replies.

const box = (page: Page) => page.getByRole("combobox", { name: "Message" }).or(page.getByRole("combobox", { name: "Message" })).first();

async function say(page: Page, text: string) {
  await box(page).fill(text);
  await box(page).press("Enter");
}

async function open(page: Page, first = "plain hello", delay = 10) {
  const mock = await installChatApi(page, { replyDelayMs: delay });
  await page.goto("/chat?project=WEB");
  await say(page, first);
  await expect(page.getByTestId("assistant-message").first()).not.toBeEmpty();
  await expect(page.getByTestId("reply-actions").first()).toBeAttached();
  return mock;
}

test("search finds a message and jumps to it", async ({ page }) => {
  await open(page, "the kokoro comparison");
  await page.getByRole("button", { name: "Find in chats" }).click();
  await page.getByRole("textbox", { name: "Find in chats" }).fill("kokoro");
  const hit = page.getByTestId("chat-search-hit").first();
  await expect(hit.locator("mark")).toHaveText("kokoro");
  await hit.getByRole("link").click();
  await expect(page).toHaveURL(/message=/);
  await expect(page.locator('[data-testid="user-message"].ring-2')).toBeVisible();
});

test("Edit replaces the last message and redoes its reply", async ({ page }) => {
  await open(page);
  const mine = page.getByTestId("user-message").last();
  await mine.getByRole("button", { name: "Edit" }).click();
  await mine.getByRole("textbox", { name: "Edit message" }).fill("better question");
  await mine.getByRole("button", { name: "Save and resend" }).click();
  await expect(page.getByTestId("assistant-message").last()).toContainText("Redone: better question");
  await expect(page.getByTestId("assistant-message")).toHaveCount(1);
});

test("Retry asks first, and only the newest reply offers it", async ({ page }) => {
  await open(page);
  await say(page, "second");
  await expect(page.getByTestId("assistant-message")).toHaveCount(2);
  // Retry is in the newest reply's ⋯ menu only.
  await page.getByTestId("assistant-message").first().getByRole("button", { name: "More message actions" }).click();
  await expect(page.getByRole("menuitem", { name: "Retry" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await page.getByTestId("assistant-message").last().getByRole("button", { name: "More message actions" }).click();
  await page.getByRole("menuitem", { name: "Retry" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Retry" }).click();
  await expect(page.getByTestId("assistant-message").last()).toContainText("Redone: second");
});

test("Copy puts the reply's markdown on the clipboard", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await open(page, "gfm: sample");
  const reply = page.getByTestId("assistant-message").first();
  await reply.getByRole("button", { name: "Copy" }).click();
  await expect(reply.getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toContain("| Option | Cost |");
});

test("selecting reply text offers Ask about this, which sends from its own box", async ({ page }) => {
  await open(page);
  await box(page).fill("my draft");
  await page.getByTestId("assistant-message").first().locator("p").first().selectText();
  await page.getByTestId("ask-about-selection").click();
  const ask = page.getByRole("textbox", { name: "Ask about this" });
  await expect(ask).toBeFocused();
  await ask.fill("why is that?");
  await ask.press("Enter");
  await expect(ask).toHaveCount(0);
  const mine = page.getByTestId("user-message").last();
  await expect(mine.locator("blockquote")).toContainText("Echo: plain hello");
  await expect(mine).toContainText("why is that?");
  await expect(box(page)).toHaveValue("my draft");
});

test("Escape closes the Ask about this box without sending", async ({ page }) => {
  await open(page);
  await page.getByTestId("assistant-message").first().locator("p").first().selectText();
  await page.getByTestId("ask-about-selection").click();
  const ask = page.getByRole("textbox", { name: "Ask about this" });
  await ask.fill("never mind");
  await ask.press("Escape");
  await expect(ask).toHaveCount(0);
  await expect(page.getByTestId("user-message")).toHaveCount(1);
});

test("the composer starts at one line and grows", async ({ page }) => {
  await open(page);
  const start = (await box(page).boundingBox())!.height;
  expect(start).toBeLessThan(50);
  await box(page).fill("a\nb\nc\nd\ne");
  await expect.poll(async () => (await box(page).boundingBox())!.height).toBeGreaterThan(start + 30);
});

test("a chat row's actions leave the title its full width until hovered", async ({ page }) => {
  await open(page);
  const history = page.getByRole("complementary", { name: "Chat history" });
  const rename = history.getByRole("button", { name: /^Rename/ }).first();
  await page.mouse.move(800, 400);
  await box(page).focus();
  await expect(rename.locator("xpath=..")).toHaveCSS("opacity", "0");
  await rename.locator("xpath=../..").hover();
  await expect(rename.locator("xpath=..")).toHaveCSS("opacity", "1");
});

test("a running reply that grows keeps the view at the bottom", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 500 });
  await open(page, "rich: diagrams and cards");
  // The narration and step card stream in after the turn starts, growing the view without adding a message.
  await say(page, "narrated: go");
  await expect(page.getByTestId("thinking")).toContainText("On it.");
  const panel = page.getByRole("tabpanel", { name: "Chat" });
  await expect.poll(() => panel.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight)).toBeLessThan(24);
});

test("a reply arriving while scrolled up offers a New reply pill instead of moving the view", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 500 });
  await open(page, "rich: long reply", 900);
  await say(page, "another");
  const panel = page.getByRole("tabpanel", { name: "Chat" });
  await page.waitForTimeout(150);
  // A wheel, not a scripted scrollTop: only the person's input lets go of the end.
  await panel.hover();
  await page.mouse.wheel(0, -5000);
  await expect.poll(() => panel.evaluate((el) => el.scrollTop)).toBeLessThan(50);
  const pill = page.getByRole("button", { name: "New reply" });
  await expect(pill).toBeVisible({ timeout: 5000 });
  expect(await panel.evaluate((el) => el.scrollTop)).toBeLessThan(50);
  await pill.click();
  await expect(pill).toHaveCount(0);
});

test("each message has one toolbar: Reply quotes it into the box, Copy and ⋯ for the rest", async ({ page }) => {
  await open(page);
  await box(page).fill("my draft");
  const reply = page.getByTestId("assistant-message").first();
  await reply.hover();
  const bar = reply.getByRole("toolbar", { name: "Message actions" });
  await expect(bar).toBeVisible();
  await expect(bar.getByRole("button")).toHaveCount(3);
  await bar.getByRole("button", { name: "Reply" }).click();
  await expect(box(page)).toBeFocused();
  await expect(box(page)).toHaveValue(/^> .+\n\nmy draft$/s);
  // Your own message: Reply, Copy and Edit; no Save.
  const mine = page.getByTestId("user-message").last();
  await mine.hover();
  await expect(mine.getByRole("toolbar").getByRole("button", { name: "Edit" })).toBeVisible();
  await expect(mine.getByRole("button", { name: "More message actions" })).toHaveCount(0);
  // Save lives in ⋯.
  await reply.hover();
  await bar.getByRole("button", { name: "More message actions" }).click();
  await expect(page.getByRole("menuitem")).toHaveText(["Retry", "Save to wiki", "Save as artifact"]);
});
