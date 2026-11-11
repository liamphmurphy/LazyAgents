import { expect, test } from "@playwright/test";
import { AGENTS, installChatApi, persona } from "./mock-chat-api";

// The landing page opens a new, unsaved chat with the persona the person chats with most;
// the Dashboard lives at /dashboard.

const ATLAS = persona(1, { name: "Atlas", is_default: true, chat_count: 2, last_used_at: "2026-11-09T09:00:00.000Z" });
const NOVA = persona(2, { name: "Nova", chat_count: 9, last_used_at: "2026-11-01T09:00:00.000Z" });

test("a fresh sign-in opens setup until the placeholder profile is replaced", async ({ page }) => {
  const api = await installChatApi(page, { emails: ["me@localhost"] });
  await page.goto("/");
  await expect(page).toHaveURL(/\/setup$/);
  await expect(page.getByRole("heading", { name: "Get started with Vesper" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Continue to chat" })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Get started with Vesper" })).toBeVisible();

  await page.getByLabel("Display name").fill("Ada Lovelace");
  await page.getByRole("button", { name: "Save profile", exact: true }).click();
  await page.getByRole("textbox", { name: "Email addresses", exact: true }).fill("ada@example.com");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await page.getByRole("button", { name: "Make primary", exact: true }).click();
  await page.getByRole("button", { name: "Remove me@localhost", exact: true }).click();
  await page.getByRole("button", { name: "Save profile", exact: true }).click();
  await expect(page.getByRole("link", { name: "Continue to chat" })).toBeVisible();
  expect(api.profileRequests.some((r) => r.includes("ada@example.com"))).toBe(true);
  await page.getByRole("link", { name: "Continue to chat" }).click();
  await page.goto("/");
  await expect(page).toHaveURL(/\/chat\?new$/);
});

test("an empty workspace opens setup even with a configured profile, until Continue", async ({ page }) => {
  await installChatApi(page);
  await page.route("**/api/v1/bootstrap", async (route) => {
    await route.fulfill({ json: {
      workspace: { id: "fresh", name: "My workspace", slug: "default" },
      me: { id: "owner", name: "Liam", email: "liam@example.com", initials: "L", role_title: "" },
      users: [], agents: [], projects: [], epics: [], labels: [], counts: { inbox: 0, wiki_suggestions: 0, live_agents: 0 },
    } });
  });
  await page.goto("/");
  await expect(page).toHaveURL(/\/setup$/);
  await page.getByRole("link", { name: "Continue to chat" }).click();
  await page.goto("/");
  await expect(page).toHaveURL(/\/chat\?new$/);
});

test("/ opens an empty chat with the most used persona instead of resuming the newest", async ({ page }) => {
  const api = await installChatApi(page, { personas: [ATLAS, NOVA] });
  const t = "2026-11-09T09:00:00.000Z";
  api.chats.push({
    id: "c0000000-0000-4000-8000-000000000001", title: "Earlier chat", agent_id: AGENTS[0].id, harness: "claude-code", model: "claude-sonnet-5-5", state: "idle",
    archived: false, revision: 1, message_count: 2, created_at: t, updated_at: t, last_message_at: t, persona_id: ATLAS.id,
  } as (typeof api.chats)[number]);

  await page.goto("/");
  await expect(page).toHaveURL(new RegExp(`/chat\\?new&persona=${NOVA.id}$`));
  await expect(page.getByRole("radiogroup", { name: "Who to talk to" }).getByRole("radio", { checked: true })).toHaveAccessibleName("Nova");
  expect(api.created).toHaveLength(0);
});

test("with no chats yet, / opens an empty chat with the default persona", async ({ page }) => {
  await installChatApi(page, { personas: [{ ...ATLAS, chat_count: 0 }, { ...NOVA, chat_count: 0 }] });
  await page.goto("/");
  await expect(page).toHaveURL(new RegExp(`/chat\\?new&persona=${ATLAS.id}$`));
  await expect(page.getByRole("radiogroup", { name: "Who to talk to" }).getByRole("radio", { checked: true })).toHaveAccessibleName("Atlas");
});
