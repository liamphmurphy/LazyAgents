import { expect, test, type Page } from "@playwright/test";
import { AGENTS, installChatApi, persona } from "./mock-chat-api";

// The persona-led welcome (/chat?new): who you're talking to, a composer, starters, and what to pick up.

const ATLAS = persona(1, { name: "Atlas", is_default: true, chat_count: 4 });
const NOVA = persona(2, { name: "Nova", tone: "friendly", chat_count: 9, capabilities: { code: false, images: true } });
const MAYA = persona(3, { name: "Maya", chat_count: 1 });

const carousel = (page: Page) => page.getByRole("radiogroup", { name: "Who to talk to" });
const checked = (page: Page) => carousel(page).getByRole("radio", { checked: true });

test("the carousel picks who the first message goes to, by click or arrow keys", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [MAYA, NOVA, ATLAS] });
  await page.goto("/chat?new");
  // The default sits in the middle, the most used beside it.
  await expect(checked(page)).toHaveAccessibleName("Atlas");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Atlas");
  await expect(page.getByText("Neutral tone · your default")).toBeVisible();

  await carousel(page).getByRole("radio", { name: "Nova" }).click();
  await expect(checked(page)).toHaveAccessibleName("Nova");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Nova");
  await expect(page.getByText("Friendly tone · makes images")).toBeVisible();

  // Arrow keys step through from the picked one, wrapping round.
  await checked(page).focus();
  await page.keyboard.press("ArrowRight");
  await expect(checked(page)).toHaveAccessibleName("Maya");
  await page.getByRole("button", { name: "Previous persona" }).click();
  await expect(checked(page)).toHaveAccessibleName("Nova");

  await page.getByRole("combobox", { name: "Message" }).fill("hi nova");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+$/);
  expect(mock.created[0].persona_id).toBe(NOVA.id);
  expect(mock.unhandled).toEqual([]);
});

test("personalised starters fill the box without sending", async ({ page }) => {
  const mock = await installChatApi(page, {
    personas: [ATLAS],
    suggestions: [
      { title: "Plan chapter three", prompt: "Help me plan chapter three of the novel", icon: "edit" },
      { title: "Review this week’s spending", prompt: "Review my spending this week", icon: "dollar" },
      { title: "Wind down", prompt: "Help me wind down tonight", icon: "moon" },
      { title: "A fourth", prompt: "Not shown", icon: "bulb" },
    ],
  });
  await page.goto("/chat?new");
  const starters = page.getByRole("list", { name: "Suggestions" }).getByRole("button");
  await expect(starters).toHaveText(["Plan chapter three", "Review this week’s spending", "Wind down"]);
  await starters.first().click();
  await expect(page.getByRole("combobox", { name: "Message" })).toHaveValue("Help me plan chapter three of the novel");
  expect(mock.posted).toEqual([]);
});

test("pick up where you left off: what needs you, unread replies, then recent chats", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS, NOVA] });
  const t = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();
  const chat = (n: number, title: string, at: string, personaId: string) =>
    ({
      id: `c0000000-0000-4000-8000-00000000000${n}`, title, agent_id: AGENTS[0].id, harness: "claude-code", model: "opus-5.5", state: "idle",
      archived: false, revision: 1, message_count: 2, created_at: at, updated_at: at, last_message_at: at, persona_id: personaId,
    }) as (typeof mock.chats)[number];
  mock.chats.push(chat(1, "Sunken temple battlemap", t(30), ATLAS.id), chat(2, "Weekly spending check", t(2), NOVA.id));
  mock.inbox.push({ id: "i1", kind: "approval", title: "WEB-42 needs your approval", summary: "", href: "/tickets/WEB-42", project: "WEB", ticket_key: "WEB-42", created_at: t(1), resolved: false, read: false, agent: { id: AGENTS[0].id, name: "Atlas", handle: "atlas", color_index: 0 } });
  mock.dock.push({
    persona: { id: NOVA.id, name: "Nova", color: NOVA.color }, state: "replied", unread: 1, last_activity_at: t(2),
    chats: [{ id: "c0000000-0000-4000-8000-000000000002", title: "Weekly spending check", state: "replied", harness: "claude-code", unread: 1, last_line: "Was the $84 at REI a gift?", last_seq: 4, waiting_approval: false }],
  });
  await page.goto("/chat?new");
  const cards = page.getByRole("region", { name: "Pick up where you left off" }).getByRole("link");
  await expect(cards).toHaveCount(3);
  await expect(cards.nth(0)).toContainText("Needs you");
  await expect(cards.nth(0)).toContainText("WEB-42 needs your approval");
  await expect(cards.nth(0)).toHaveAttribute("href", "/tickets/WEB-42");
  await expect(cards.nth(1)).toContainText("Replied");
  await expect(cards.nth(1)).toContainText("Was the $84 at REI a gift?");
  await expect(cards.nth(2)).toContainText("Continue");
  await expect(cards.nth(2)).toContainText("Sunken temple battlemap");
  expect(mock.unhandled).toEqual([]);
});

test("with nothing to pick up, the section stays out of the way", async ({ page }) => {
  await installChatApi(page, { personas: [ATLAS] });
  await page.goto("/chat?new");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Atlas");
  await expect(page.getByRole("region", { name: "Pick up where you left off" })).toHaveCount(0);
});
