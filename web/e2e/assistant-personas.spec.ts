import { expect, test, type Page } from "@playwright/test";
import { installChatApi, type MockPersona } from "./mock-chat-api";

// Assistant personas in the Assistant view (LAZYAGENTS-86): the sidebar's persona
// dropdown, the persona's identity on replies, and the persona of a new conversation.

const ATLAS: MockPersona = { id: "bbbbbbbb-0000-4000-8000-000000000001", name: "Atlas", tone: "friendly", color: "#4fd1c5", custom_instructions: "", is_default: true, revision: 1 };
const NOVA: MockPersona = { id: "bbbbbbbb-0000-4000-8000-000000000002", name: "Nova", tone: "playful", color: "#f472b6", custom_instructions: "", is_default: false, revision: 1 };

const history = (page: Page) => page.getByRole("navigation", { name: "Chats" });
const filter = (page: Page) => page.getByRole("button", { name: /^Show conversations for:/ });
const switcher = (page: Page) => page.getByRole("button", { name: /^Persona for this conversation:/ });
const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });

test("persona sidebar filter, reply identity, live edits and new-conversation persona", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat?project=WEB");

  // Welcome: the first conversation goes to the default persona.
  await expect(page.getByRole("heading", { name: "Hi Liam, what can I help with?" })).toBeVisible();
  await expect(switcher(page)).toHaveAccessibleName("Persona for this conversation: Atlas");
  await page.getByRole("textbox", { name: "Message" }).fill("hello atlas");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  expect(mock.created[0].persona_id).toBe(ATLAS.id);

  // The reply carries the persona's avatar and its name in its color.
  const identity = page.getByTestId("assistant-identity").first();
  await expect(identity).toContainText("Atlas");
  await expect(identity.getByText("Atlas", { exact: true })).toHaveCSS("color", "rgb(79, 209, 197)");

  // A persona edit reaches the open conversation over SSE.
  const atlasChat = page.url();
  await mock.updatePersona(ATLAS.id, { name: "Atlas Prime" });
  await expect(identity).toContainText("Atlas Prime");

  // New conversation: the default persona, switchable before the first message.
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(switcher(page)).toHaveAccessibleName("Persona for this conversation: Atlas Prime");
  expect(mock.created[1].persona_id).toBe(ATLAS.id);
  await switcher(page).click();
  await page.getByRole("menuitemradio", { name: /Nova/ }).click();
  await expect(page.getByRole("heading", { name: /I’m Nova/ })).toBeVisible();
  expect(mock.created[2].persona_id).toBe(NOVA.id);
  await composer(page).fill("hi nova");
  await composer(page).press("Enter");
  await expect(page.getByTestId("assistant-identity").first()).toContainText("Nova");
  // Once a message is sent the persona is fixed.
  await expect(switcher(page)).toHaveCount(0);

  // The sidebar opens on the default persona: only its conversations.
  await expect(filter(page)).toHaveAccessibleName("Show conversations for: Atlas Prime (default)");
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toHaveCount(0);

  // Another persona, then every persona.
  await filter(page).click();
  await page.getByRole("menuitemradio", { name: /Nova/ }).click();
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toHaveCount(0);
  await filter(page).click();
  await page.getByRole("menuitemradio", { name: "All personas" }).click();
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toBeVisible();
  expect(mock.personaRequests).toEqual(expect.arrayContaining(["", NOVA.id, "all"]));

  // The pick lasts for the session.
  await page.goto(atlasChat);
  await expect(filter(page)).toHaveAccessibleName("Show conversations for: All personas");
  expect(mock.unhandled).toEqual([]);
});

test("without personas the built-in Assistant is the default and there's no switcher", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await expect(filter(page)).toHaveAccessibleName("Show conversations for: Assistant (default)");
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(composer(page)).toBeVisible();
  await expect(switcher(page)).toHaveCount(0);
  expect(mock.created[0].persona_id).toBeUndefined();
  expect(mock.unhandled).toEqual([]);
});
