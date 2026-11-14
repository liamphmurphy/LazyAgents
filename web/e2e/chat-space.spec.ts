import { expect, test } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });
const NOVA = persona(2, { name: "Nova", color: "#f6ad55" });

test("chat lives above projects: no switcher, and a default project can be set and cleared", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await page.goto("/chat");
  await expect(page.getByRole("button", { name: /^Switch project/ })).toHaveCount(0);

  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+$/);
  await expect(page.getByTestId("assistant-message").first()).toBeVisible();
  expect(mock.created.at(-1)?.project).toBeUndefined();

  // The default project lives in the title's ▾ menu.
  const titleMenu = page.getByTestId("chat-title-menu");
  const project = page.getByTestId("chat-project");
  await titleMenu.click();
  await expect(project).toContainText("No default");
  await project.click();
  await page.getByRole("menuitemradio", { name: "Web app" }).click();
  await expect.poll(() => mock.chats[0].project).toBe("WEB");

  await titleMenu.click();
  await expect(project).toContainText("Web app");
  await project.click();
  await page.getByRole("menuitemradio", { name: "No default" }).click();
  await expect.poll(() => mock.chats[0].project).toBeUndefined();
  await titleMenu.click();
  await expect(project).toContainText("No default");
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
