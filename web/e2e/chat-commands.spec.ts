import { expect, test, type Page } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });
const menu = (page: Page) => page.getByTestId("command-suggestions");

async function openChat(page: Page) {
  await page.goto("/chat?project=WEB");
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(composer(page)).toBeVisible();
  await expect(composer(page)).toBeFocused();
}

test("/ lists the commands and /memory opens the Memory page without sending", async ({ page }) => {
  const mock = await installChatApi(page);
  await openChat(page);
  await composer(page).pressSequentially("/");
  await expect(menu(page).getByRole("option")).toHaveCount(8);
  await composer(page).pressSequentially("mem");
  await expect(menu(page).getByRole("option")).toHaveCount(1);
  await composer(page).press("Enter");
  await expect(page.getByRole("region", { name: "Memory", exact: true })).toBeVisible();
  expect(mock.posted).toEqual([]);
  expect(mock.unhandled).toEqual([]);
});

test("/effort picks from its arguments and applies to the next message", async ({ page }) => {
  await installChatApi(page);
  await openChat(page);
  await composer(page).pressSequentially("/eff");
  await composer(page).press("Enter");
  await expect(composer(page)).toHaveValue("/effort ");
  await composer(page).pressSequentially("high");
  await composer(page).press("Enter");
  await expect(composer(page)).toHaveValue("");
  await expect(page.getByRole("button", { name: "Effort for the next message: High" })).toBeVisible();
});

test("an unknown command stays in the composer with an error; // sends it as text", async ({ page }) => {
  const mock = await installChatApi(page);
  await openChat(page);
  await composer(page).fill("/nope");
  await composer(page).press("Enter");
  await expect(page.getByTestId("command-error")).toContainText("Unknown command /nope");
  await expect(composer(page)).toHaveValue("/nope");
  expect(mock.posted).toEqual([]);
  await composer(page).fill("//nope");
  await expect(page.getByTestId("command-error")).toHaveCount(0);
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: /nope");
});

test("a path-like message is sent as text", async ({ page }) => {
  await installChatApi(page);
  await openChat(page);
  await composer(page).fill("/usr/bin is missing");
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: /usr/bin is missing");
});

test("/persona starts a new chat and keeps the one with messages", async ({ page }) => {
  const mock = await installChatApi(page);
  mock.personas.push(persona(1, { name: "Atlas", is_default: true }), persona(2, { name: "Nova" }));
  await openChat(page);
  await composer(page).fill("hello");
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: hello");
  const before = page.url();
  await composer(page).fill("/persona nova");
  await expect(menu(page).getByRole("option")).toHaveCount(1);
  await composer(page).press("Enter");
  await expect.poll(() => page.url()).not.toBe(before);
  await expect(composer(page)).toHaveValue("");
  expect(mock.chats.length).toBe(2);
});

test("Memory and Personas live under Assistant in the main menu", async ({ page }) => {
  await installChatApi(page);
  await openChat(page);
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(page.getByRole("button", { name: /^Memory/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Personas/ })).toHaveCount(0);
  await nav.getByRole("link", { name: "Personas", exact: true }).click();
  await expect(page.getByRole("region", { name: "Personas", exact: true })).toBeVisible();
  await nav.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(page.getByRole("region", { name: "Memory", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Personas", exact: true })).toHaveCount(0);
  await expect(page).toHaveURL(/\/memory$/);
});

