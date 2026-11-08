import { expect, test, type Page } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

// Assistant personas in the Assistant view (LAZYAGENTS-86): the sidebar's persona
// dropdown, the persona's identity on replies, and the persona of a new conversation.

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });
const NOVA = persona(2, { name: "Nova", tone: "playful", color: "#f472b6" });

const history = (page: Page) => page.getByRole("navigation", { name: "Chats" });
const strip = (page: Page) => page.getByRole("group", { name: "Show conversations for" });
const picked = (page: Page) => strip(page).getByRole("button", { pressed: true });
const pick = (page: Page, name: string | RegExp) => strip(page).getByRole("button", { name }).click();
const switcher = (page: Page) => page.getByRole("button", { name: /^Persona for this conversation:/ });
const composer = (page: Page) => page.getByRole("combobox", { name: "Message" });

// History lives in the expanded menu.
test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

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

  // New conversation: the default persona (the sidebar shows it), switchable before the first message.
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
  await expect(picked(page)).toHaveAccessibleName("Atlas Prime (default)");
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toHaveCount(0);
  // A row only needs its persona's picture when the list mixes personas.
  await expect(history(page).getByRole("link", { name: /hello atlas/ }).locator(":scope > [aria-hidden]")).toHaveCount(0);

  // Another persona, then every persona.
  await pick(page, /Nova/);
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toHaveCount(0);
  // New conversation follows the sidebar's persona.
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(switcher(page)).toHaveAccessibleName("Persona for this conversation: Nova");
  expect(mock.created[3].persona_id).toBe(NOVA.id);
  await pick(page, "All personas");
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toBeVisible();
  await expect(history(page).getByRole("link", { name: /hello atlas/ }).locator(":scope > [aria-hidden]")).toHaveCount(1);
  // All personas falls back to the default.
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(switcher(page)).toHaveAccessibleName("Persona for this conversation: Atlas Prime");
  expect(mock.created[4].persona_id).toBe(ATLAS.id);
  expect(mock.chatPersonaFilters).toEqual(expect.arrayContaining(["", NOVA.id, "all"]));

  // The pick lasts for the session.
  await page.goto(atlasChat);
  await expect(picked(page)).toHaveAccessibleName("All personas");
  expect(mock.unhandled).toEqual([]);
});

test("picking a persona in the sidebar opens its newest conversation", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("hello atlas");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  const atlasChat = new URL(page.url()).pathname;

  // A persona with no conversations leaves the open one alone and offers to start one.
  await pick(page, /Nova/);
  await expect(history(page).getByText("No conversations with Nova yet.")).toBeVisible();
  expect(new URL(page.url()).pathname).toBe(atlasChat);
  await history(page).getByRole("button", { name: "Start one with Nova →" }).click();
  await expect(page).not.toHaveURL(new RegExp(atlasChat));
  await composer(page).fill("hi nova");
  await composer(page).press("Enter");
  await expect(history(page).getByRole("link", { name: /hi nova/ })).toBeVisible();
  const novaChat = new URL(page.url()).pathname;

  // Switching personas follows to each one's newest conversation.
  await pick(page, /Atlas/);
  await expect(page).toHaveURL(new RegExp(atlasChat));
  await pick(page, /Nova/);
  await expect(page).toHaveURL(new RegExp(novaChat));

  // All personas keeps the open conversation.
  await pick(page, "All personas");
  await expect(history(page).getByRole("link", { name: /hello atlas/ })).toBeVisible();
  expect(new URL(page.url()).pathname).toBe(novaChat);
  expect(mock.unhandled).toEqual([]);
});

test("without personas the built-in Assistant is the default and there's no switcher", async ({ page }) => {
  const mock = await installChatApi(page);
  await page.goto("/chat?project=WEB");
  await expect(picked(page)).toHaveAccessibleName("Assistant (default)");
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(composer(page)).toBeVisible();
  await expect(switcher(page)).toHaveCount(0);
  expect(mock.created[0].persona_id).toBeUndefined();
  expect(mock.unhandled).toEqual([]);
});

test("the collapsed menu lists personas and opens each one's newest conversation", async ({ page }) => {
  await page.addInitScript(() => localStorage.removeItem("lazyagents.sidebar-expanded"));
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("hello atlas");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  const atlasChat = new URL(page.url()).pathname;

  const rail = page.getByRole("group", { name: "Open a persona’s conversations" });
  await expect(page.getByRole("complementary", { name: "Chat history" })).toHaveCount(0);
  // No conversations yet: the rail has no list to offer one from, so it starts one.
  await rail.getByRole("button", { name: "Nova" }).click();
  await expect.poll(() => mock.created.at(-1)?.persona_id).toBe(NOVA.id);
  await expect(page).not.toHaveURL(new RegExp(atlasChat));

  await rail.getByRole("button", { name: /^Atlas/ }).click();
  await expect(page).toHaveURL(new RegExp(atlasChat));
  expect(mock.unhandled).toEqual([]);
});

test("a new conversation opens at once, without waiting for the lists to refresh", async ({ page }) => {
  await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("hello atlas");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(page.getByTestId("assistant-identity").first()).toContainText("Atlas");
  const first = new URL(page.url()).pathname;

  await page.route(/\/api\/v1\/chats\?/, async (route) => {
    await new Promise((r) => setTimeout(r, 4000));
    await route.fallback();
  });
  await page.getByRole("button", { name: "New conversation", exact: true }).first().click();
  await expect(page).not.toHaveURL(new RegExp(first), { timeout: 2500 });
});

test("the menu keeps its width whatever the titles and persona filter", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("hello atlas");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+/);
  await expect(page.getByTestId("assistant-identity").first()).toContainText("Atlas");
  mock.chats[0].title = "A title still being generated that keeps going and going well past the width of the menu";
  await page.reload();
  await expect(history(page).getByRole("link", { name: /A title still being generated/ })).toBeVisible();

  const menu = page.getByRole("complementary", { name: "Main menu" });
  const width = async () => (await menu.boundingBox())!.width;
  expect(await width()).toBe(232);
  await pick(page, "All personas");
  await expect(picked(page)).toHaveAccessibleName("All personas");
  expect(await width()).toBe(232);
  await pick(page, /^Atlas/);
  await expect(picked(page)).toHaveAccessibleName("Atlas (default)");
  expect(await width()).toBe(232);
});
