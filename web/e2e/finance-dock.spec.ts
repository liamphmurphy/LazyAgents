import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// LAZYAGENTS-161 (mockup "Finance on the shared EditorLayout: Chat dock tab"): Finance renders
// through the shared editor layout with one Chat tab, its Ask chat, in place of the old drawer.
const dock = (page: Page) => page.getByRole("complementary", { name: "Panel", exact: true });
const rail = (page: Page) => page.getByRole("navigation", { name: "Panel rail" });
const mod = process.platform === "darwin" ? "Meta" : "Control";
const PRIVACY = "Only you see this, and only Ask, here in Finance, can read it.";

test.use({ viewport: { width: 1440, height: 900 } });

const overview = {
  space_id: "55555555-5555-4555-8555-555555555555",
  categories_all: ["Groceries", "Dining"],
  accounts: [{ id: "a1", name: "Checking", kind: "checking", mask: "1234", transaction_count: 40, from_ofx: false }],
  months: [
    { month: "2026-10-01", inflow_cents: 500000, outflow_cents: 320000, transfer_out_cents: 0 },
    { month: "2026-11-01", inflow_cents: 500000, outflow_cents: 341200, transfer_out_cents: 0 },
  ],
  category_month: "2026-11-01",
  categories: [
    { category: "Groceries", spent_cents: 61200, average_cents: 60000 },
    { category: "Dining", spent_cents: 48800, average_cents: 47000 },
  ],
  review: [],
  review_count: 0,
  transaction_count: 40,
  recurring: [],
};

async function install(page: Page) {
  const api = await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/finance(\?|$)/, (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(overview) }));
  return api;
}

test("Finance's Ask is the dock's Chat tab, with the privacy note, and collapses to a rail", async ({ page }) => {
  await install(page);
  await page.goto("/apps/finance");
  await expect(page.locator('[data-editor-layout="finance"]')).toBeVisible();
  // No left panel; one Chat tab.
  await expect(page.getByRole("complementary", { name: "Structure" })).toHaveCount(0);
  await expect(dock(page).getByRole("tab")).toHaveText(["Chat"]);
  await expect(dock(page).getByText(PRIVACY)).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);

  // Collapse to the rail; the header's Ask reopens Chat.
  await dock(page).getByRole("button", { name: "Collapse panel" }).click();
  await expect(rail(page)).toBeVisible();
  await page.getByRole("button", { name: "✦ Ask" }).click();
  await expect(dock(page).getByRole("tablist", { name: "Panel tabs" }).getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");

  // Collapsed state is remembered across a reload; ⌘J opens Chat and focuses the composer.
  await dock(page).getByRole("button", { name: "Collapse panel" }).click();
  await page.reload();
  await expect(rail(page)).toBeVisible();
  await page.locator("body").click();
  await page.keyboard.press(`${mod}+j`);
  await expect(dock(page)).toBeVisible();
  await expect(dock(page).getByRole("textbox", { name: "Ask about your money" })).toBeFocused();
});

test("asking about a spending section sends it to the Finance chat in the Chat tab", async ({ page }) => {
  const api = await install(page);
  await page.goto("/apps/finance");
  await dock(page).getByRole("button", { name: "Collapse panel" }).click();
  await expect(rail(page)).toBeVisible();

  const question = /What made up my Groceries spending in November\?/;
  await page.getByRole("button", { name: /Groceries/ }).first().click();
  await expect(dock(page).getByRole("tablist", { name: "Panel tabs" }).getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");
  await expect.poll(() => api.posted.map((p) => p.body.body)).toEqual([expect.stringMatching(question)]);
  expect(api.created).toEqual([expect.objectContaining({ space_id: overview.space_id })]);
  await expect(dock(page).getByText(question).first()).toBeVisible();
  await expect(dock(page).getByText(PRIVACY)).toBeVisible();
});
