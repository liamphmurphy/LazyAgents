import { expect, test } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installChatApi, persona } from "./mock-chat-api";

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });
const NOVA = persona(2, { name: "Nova", color: "#f6ad55" });

const space = (id: string, name: string, over: Partial<components["schemas"]["ChatSpace"]> = {}): components["schemas"]["ChatSpace"] => ({
  id, name, description: "", color: "blue", instructions: "", connector_ids: [], space_memory: false, chat_count: 0, source_count: 0, memory_count: 0,
  inline_tokens: 0, created_at: "2026-11-01T09:00:00.000Z", updated_at: "2026-11-01T09:00:00.000Z", ...over,
});
const VESPER = space("11111111-1111-4111-8111-000000000001", "Vesper", { project: "WEB", work: { key: "WEB", default_branch: "main", archived: false, revision: 1 } });
const HEALTH = space("11111111-1111-4111-8111-000000000002", "Health", { color: "green" });
const OLD = space("11111111-1111-4111-8111-000000000003", "Old work", { work: { key: "OLD", default_branch: "main", archived: true, revision: 2 } });

test("a chat moves into a space and out again: no project switcher, and archived work isn't offered (LAZYAGENTS-213)", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS], spaces: [VESPER, HEALTH, OLD] });
  await page.goto("/chat");
  await expect(page.getByRole("button", { name: /^Switch (project|space)/ })).toHaveCount(0);

  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+$/);
  await expect(page.getByTestId("assistant-message").first()).toBeVisible();
  expect(mock.created.at(-1)?.project).toBeUndefined();

  // The header's "In" control names the chat's space and moves it.
  const inSpace = page.getByTestId("chat-space");
  await expect(inSpace).toHaveAccessibleName("Put this chat in a space");
  await inSpace.click();
  await expect(page.getByRole("menuitemradio", { name: /Old work/ })).toHaveCount(0);
  await page.getByRole("menuitemradio", { name: /Vesper/ }).click();
  await expect.poll(() => mock.chats[0].space?.id).toBe(VESPER.id);
  // Vesper tracks work, so the chat files its tickets there now.
  expect(mock.chats[0].project).toBe("WEB");
  await expect(inSpace).toHaveAccessibleName("In Vesper; move to another space");

  // The title menu offers the same move; Health files nowhere, so the chat keeps WEB.
  await page.getByTestId("chat-title-menu").click();
  await expect(page.getByTestId("chat-space-menu")).toContainText("Vesper");
  await page.getByTestId("chat-space-menu").click();
  await page.getByRole("menuitemradio", { name: /Health/ }).click();
  await expect.poll(() => mock.chats[0].space?.id).toBe(HEALTH.id);
  expect(mock.chats[0].project).toBe("WEB");

  await inSpace.click();
  await page.getByRole("menuitemradio", { name: "No space" }).click();
  await expect.poll(() => mock.chats[0].space).toBeUndefined();
  await expect(inSpace).toHaveAccessibleName("Put this chat in a space");
  expect(mock.unhandled).toEqual([]);
});

test("the title menu renames the chat, lists a group's participants and shows non-default settings as icons", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat");
  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toBeVisible();
  const status = page.getByTestId("chat-title-status");
  await expect(status.getByRole("img")).toHaveCount(0);

  await page.getByTestId("chat-title-menu").click();
  await page.getByRole("menuitem", { name: "Rename…" }).click();
  await page.getByRole("textbox", { name: "Conversation title" }).fill("Pricing review");
  await page.getByRole("button", { name: "Save title" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Pricing review" })).toBeVisible();
  expect(mock.chats[0].title).toBe("Pricing review");

  mock.chats[0].workspace_path = "/Users/me/vesper/workspaces/0199aaaa-run";
  mock.chats[0].guest_persona_ids = [mock.personas[1].id];
  await page.reload();
  await expect(status.getByRole("img", { name: /^Workspace folder: / })).toBeVisible();
  await page.getByTestId("chat-title-menu").click();
  await expect(page.getByTestId("chat-participants")).toContainText("Atlas");
  await expect(page.getByTestId("chat-participants")).toContainText("Nova");
  expect(mock.unhandled).toEqual([]);
});
