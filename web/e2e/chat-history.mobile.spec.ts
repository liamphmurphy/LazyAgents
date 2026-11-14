import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installChatApi, openEmptyChat, persona } from "./mock-chat-api";

// VESPER-10: on a phone (project "mobile", 390×844) the chat header is ≡, the title, panels and ···,
// and the history lives in a drawer from the left.
const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const header = (page: Page) => page.getByTestId("compact-chat-header");
const drawer = (page: Page) => page.getByRole("dialog", { name: "History" });

async function chatAbout(page: Page, text: string) {
  const id = await openEmptyChat(page);
  await composer(page).fill(text);
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText(`Echo: ${text}`);
  return id;
}

test("the compact header has history, title, panels and menu, each a 44px target", async ({ page }) => {
  await installChatApi(page);
  await chatAbout(page, "groceries");
  await expect(header(page).getByRole("heading", { level: 1 })).toHaveText("groceries");
  await expect(header(page).getByTestId("chat-subtitle")).toHaveText("Assistant");
  for (const name of ["Chat history", "Panels", "Chat menu"]) {
    const box = await header(page).getByRole("button", { name, exact: true }).boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
  }
  // The desktop tabs moved into the menu.
  await expect(page.getByRole("tablist", { name: "Chat view" })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
});

test("the drawer lists history and switching chats closes it", async ({ page }) => {
  await installChatApi(page);
  const first = await chatAbout(page, "first chat");
  await chatAbout(page, "second chat");
  const opener = header(page).getByRole("button", { name: "Chat history" });
  await opener.click();
  const d = drawer(page);
  await expect(d).toBeVisible();
  await expect(d.getByRole("button", { name: "New chat" })).toBeVisible();
  await expect(d.getByRole("searchbox", { name: "Search chats" })).toBeVisible();
  // No personas of your own here, so no persona filter (assistant-persona-view covers it).
  await expect(d.getByRole("button", { name: /^Show conversations for:/ })).toHaveCount(0);
  // Row actions show without hover.
  await expect(d.getByRole("button", { name: "Rename first chat" })).toBeVisible();

  await d.getByRole("link", { name: /first chat/ }).click();
  await expect(page).toHaveURL(new RegExp(`/chat/${first}`));
  await expect(drawer(page)).toHaveCount(0);
  await expect(header(page).getByRole("heading", { level: 1 })).toHaveText("first chat");
});

test("the drawer traps focus, closes on Escape, scrim and swipe, and returns focus", async ({ page }) => {
  await installChatApi(page);
  await chatAbout(page, "focus");
  const opener = header(page).getByRole("button", { name: "Chat history" });

  await opener.click();
  await expect(drawer(page)).toBeVisible();
  for (let i = 0; i < 12; i++) await page.keyboard.press("Tab");
  expect(await drawer(page).evaluate((d) => d.contains(document.activeElement))).toBe(true);
  await page.keyboard.press("Escape");
  await expect(drawer(page)).toHaveCount(0);
  await expect(opener).toBeFocused();

  // A tap on the scrim beside the drawer, once it has opened (it takes focus when it has).
  await opener.click();
  await expect(drawer(page)).toBeVisible();
  await expect.poll(() => drawer(page).evaluate((d) => d.contains(document.activeElement))).toBe(true);
  await page.mouse.click(370, 400);
  await expect(drawer(page)).toHaveCount(0);
  await expect(opener).toBeFocused();

  // A swipe to the left.
  await opener.click();
  const box = (await drawer(page).boundingBox())!;
  await drawer(page).dispatchEvent("pointerdown", { pointerType: "touch", clientX: box.width - 20, clientY: 500 });
  await drawer(page).dispatchEvent("pointerup", { pointerType: "touch", clientX: 40, clientY: 510 });
  await expect(drawer(page)).toHaveCount(0);
});

const WEBAPP: components["schemas"]["ChatSpace"] = {
  id: "11111111-1111-4111-8111-000000000001", name: "Web app", description: "", color: "blue", instructions: "", connector_ids: [], space_memory: false,
  chat_count: 0, source_count: 0, memory_count: 0, inline_tokens: 0, created_at: "2026-11-01T09:00:00.000Z", updated_at: "2026-11-01T09:00:00.000Z",
  project: "WEB", work: { key: "WEB", default_branch: "main", archived: false, revision: 1 },
};

test("Activity, space, rename and archive are in the chat menu", async ({ page }) => {
  const mock = await installChatApi(page, { spaces: [WEBAPP] });
  await chatAbout(page, "menu chat");
  const menuButton = header(page).getByRole("button", { name: "Chat menu" });

  await menuButton.click();
  await page.getByRole("menuitemcheckbox", { name: "Activity" }).click();
  await expect(page.getByRole("tabpanel", { name: "Activity" })).toBeVisible();
  await menuButton.click();
  await page.getByRole("menuitemcheckbox", { name: "Activity" }).click();
  await expect(composer(page)).toBeVisible();

  await menuButton.click();
  // Moving the chat into a space that tracks work files its tickets there (LAZYAGENTS-213).
  await page.getByRole("menuitemradio", { name: /Web app/ }).click();
  await expect.poll(() => mock.chats.at(-1)?.space?.id).toBe(WEBAPP.id);
  expect(mock.chats.at(-1)?.project).toBe("WEB");

  await menuButton.click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const title = header(page).getByRole("textbox", { name: "Conversation title" });
  await expect(title).toBeFocused();
  await title.fill("Renamed");
  await title.press("Enter");
  await expect(header(page).getByRole("heading", { level: 1 })).toHaveText("Renamed");

  await menuButton.click();
  await page.getByRole("menuitem", { name: "Archive" }).click();
  await expect(page).toHaveURL(/\/chat$/);
});

test("Repository in the chat menu opens the composer's repository picker", async ({ page }) => {
  await installChatApi(page, { personas: [persona(1, { name: "Coder", is_default: true, capabilities: { code: true, images: false } })] });
  await openEmptyChat(page);
  await header(page).getByRole("button", { name: "Chat menu" }).click();
  await page.getByRole("menuitem", { name: /Repository/ }).click();
  // The compact composer keeps the repository picker in its options sheet, which the menu opens.
  const sheet = page.getByTestId("composer-sheet");
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: /^Repository/ }).click();
  await page.getByRole("button", { name: "Other repository…" }).click();
  const box = page.getByRole("textbox", { name: "Repository name or GitHub URL" });
  await expect(box).toBeVisible();
  await box.fill("https://github.com/acme/app");
  await box.press("Enter");
  await expect(header(page).getByTestId("chat-subtitle")).toHaveText("Coder · acme/app");
});

test("the drawer's foot shows the harness characters a phone has no rail for, opening usage above", async ({ page }) => {
  await installChatApi(page);
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: { personas: [], space_chats: [], harnesses: [
    { harness: "claude-code", label: "Claude", connected: true, running: 1, usage_available: false, usage_checking: false, windows: [] },
  ] } }));
  await chatAbout(page, "first chat");
  await header(page).getByRole("button", { name: "Chat history" }).click();
  const footer = drawer(page).getByRole("region", { name: "Harness activity" });
  await expect(footer).toHaveAttribute("data-rail", "false");
  await footer.getByRole("button", { name: /Claude, working/ }).click();
  await expect(page.getByRole("dialog", { name: "Claude usage" })).toBeVisible();
});
