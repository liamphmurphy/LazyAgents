import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

const AIQ = "11111111-1111-4111-8111-111111111111";
const SOLO = "22222222-2222-4222-8222-222222222222";
const at = (minutesAgo: number) => new Date(Date.now() - minutesAgo * 60_000).toISOString();

const space = (id: string, name: string) => ({
  id, name, description: "", color: "green", instructions: "", connector_ids: [], space_memory: true,
  chat_count: 8, source_count: 0, memory_count: 0, inline_tokens: 0, created_at: at(900), updated_at: at(900),
});
const activity = (id: string, space_id: string, title: string, state: string, startedMinutesAgo?: number) => ({
  id, space_id, title, state, last_activity_at: at(1), ...(startedMinutesAgo === undefined ? {} : { started_at: at(startedMinutesAgo) }),
});

async function install(page: Page, spaceChats: unknown[]) {
  await installChatApi(page);
  await page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1"));
  await page.route("**/api/v1/chat-spaces", (route) => route.fulfill({ json: { spaces: [space(AIQ, "AIQ"), space(SOLO, "Solo")] } }));
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: { personas: [], space_chats: spaceChats, harnesses: [] } }));
}

const nav = (page: Page) => page.getByRole("region", { name: "Spaces" }).first();

test("a space with several working chats shows a badge, a flyout and a jump to the latest started", async ({ page }) => {
  await install(page, [
    activity("c1", AIQ, "Houston multi-model epic", "working", 20),
    activity("c2", AIQ, "Prompt research", "working", 60),
    activity("c3", AIQ, "Which ending do you want", "needs_you"),
  ]);
  await page.goto("/chat?project=WEB");
  const badge = nav(page).getByRole("button", { name: /AIQ: 2 working, 1 waiting on you/ });
  await expect(badge).toHaveText("3");

  // Clicking the name still opens the space, not a chat.
  await expect(nav(page).getByRole("link", { name: "AIQ", exact: true })).toHaveAttribute("href", `/spaces/${AIQ}`);

  await expect(nav(page).getByRole("link", { name: /Jump to Houston multi-model epic/ })).toHaveAttribute("href", "/chat/c1");

  await badge.click();
  const flyout = page.getByRole("dialog", { name: "Chats active in AIQ" });
  await expect(flyout.getByRole("link")).toHaveText([/Houston multi-model epic.*latest/, /Prompt research/, /Which ending do you want.*waiting on you/, /Open AIQ/]);
  await expect(flyout.getByRole("link", { name: /Prompt research/ })).toHaveAttribute("href", "/chat/c2");
});

test("a space with one active chat opens it straight from the badge and has no jump button", async ({ page }) => {
  await install(page, [activity("c9", SOLO, "Only one", "working", 5)]);
  await page.goto("/chat?project=WEB");
  await expect(nav(page).getByRole("link", { name: /Solo: 1 working; open Only one/ })).toHaveAttribute("href", "/chat/c9");
  await expect(nav(page).getByRole("link", { name: /Jump to/ })).toHaveCount(0);
});

test("an idle space shows only its chat count", async ({ page }) => {
  await install(page, []);
  await page.goto("/chat?project=WEB");
  await expect(nav(page).getByRole("link", { name: "AIQ", exact: true })).toBeVisible();
  await expect(nav(page).locator("[data-space-activity]")).toHaveCount(0);
});
