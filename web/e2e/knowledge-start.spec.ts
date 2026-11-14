import { expect, test, type Route } from "@playwright/test";
import { DECK, deckWith, installDeck } from "./mock-deck-api";

// Each kind has a start page in Knowledge (/knowledge?kind=…): what Docs, Slides and Designs had as
// their own homes before they became pages. Home's Start row leads there, the page's own tabs switch
// kinds in one Knowledge tab, and its gallery lists that kind only, wherever each one is filed.
test.use({ viewport: { width: 1440, height: 900 } });

const json = (route: Route, data: unknown) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(data) });
const at = "2026-11-14T12:00:00Z";
const row = (id: string, title: string, path: string, kind: string, canvas_id?: string) => ({ id, title, path, audience: "private", summary: "", properties: {}, updated_at: at, version_id: "v1", pending: 0, unresolved: 0, kind, canvas_id });
const ROWS = [
  row("11111111-1111-4111-8111-111111111111", "Reading list", "Reading list", "document"),
  row(`a${DECK.slice(1)}`, "Q4 review", "Work/Q4 review", "deck", DECK),
  row("22222222-2222-4222-8222-222222222222", "Checkout flow", "Designs/Checkout flow", "design", "33333333-3333-4333-8333-333333333333"),
];

test("Home's Start row opens a kind's start page, whose tabs switch kinds in one tab and list only that kind", async ({ page }) => {
  await installDeck(page, [{ seq: 1, body: JSON.stringify(deckWith("Q4 review")), author: "user" }]);
  const asked: (string | undefined)[] = [];
  await page.route(/\/api\/v1\/knowledge\/query/, (route) => {
    const kind = route.request().postDataJSON()?.kind as string | undefined;
    asked.push(kind);
    return json(route, { columns: [], rows: ROWS.filter((r) => !kind || r.kind === kind), has_more: false });
  });
  await page.route(/\/api\/v1\/slides$/, (route) => json(route, { decks: [{ id: DECK, title: "Q4 review", chat_id: "c", updated_at: at, slide_count: 12, cover: JSON.stringify(deckWith("Q4 review")) }] }));
  await page.goto("/knowledge");

  const start = page.getByRole("region", { name: "Start" });
  await expect(start.getByRole("link", { name: "All decks · 1 →" })).toBeVisible();
  await start.getByRole("link", { name: "All decks · 1 →" }).click();
  await expect(page).toHaveURL(/\/knowledge\?kind=deck$/);
  await expect(page.getByRole("heading", { level: 1, name: "Decks" })).toBeVisible();
  // Only decks, with the folder each is filed in and its slide count from the deck list.
  const decks = page.getByRole("list", { name: "Decks" });
  await expect(decks.getByRole("link")).toHaveCount(1);
  await expect(decks.getByRole("link", { name: /Q4 review.*Work · 12 slides/ })).toBeVisible();
  expect(asked).toContain("deck");

  const tabs = page.getByRole("tablist", { name: "Open pages" });
  await page.getByRole("navigation", { name: "Kinds" }).getByRole("link", { name: "Designs" }).click();
  await expect(page).toHaveURL(/\/knowledge\?kind=design$/);
  await expect(page.getByRole("list", { name: "Designs" }).getByRole("link", { name: /Checkout flow/ })).toBeVisible();
  // Switching kinds retitled the one start tab rather than opening another beside it.
  await expect(tabs.getByRole("tab", { name: /Designs/ })).toHaveAttribute("aria-selected", "true");
  await expect(tabs.getByRole("tab", { name: /Decks/ })).toHaveCount(0);

  await page.getByRole("navigation", { name: "Kinds" }).getByRole("link", { name: "Documents" }).click();
  await expect(page.getByRole("list", { name: "Documents" }).getByRole("link")).toHaveCount(1);
  await expect(page.getByRole("list", { name: "Documents" }).getByRole("link", { name: /Reading list/ })).toBeVisible();
});

test("a description on the Decks start page makes an untitled deck and asks its persona to draft it", async ({ page }) => {
  await installDeck(page, [{ seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" }]);
  const made: unknown[] = [];
  await page.route(/\/api\/v1\/knowledge\/query/, (route) => json(route, { columns: [], rows: [], has_more: false }));
  await page.route(/\/api\/v1\/slides$/, (route) => {
    if (route.request().method() === "GET") return json(route, { decks: [] });
    made.push(route.request().postDataJSON());
    return json(route, { id: DECK, kind: "deck", title: "", chat_id: "c", chat_title: "c", updated_at: at, reviewed_seq: 0 });
  });
  await page.goto("/knowledge?kind=deck");

  await expect(page.getByText("No decks yet")).toBeVisible();
  const box = page.getByRole("textbox", { name: "New deck title or brief" });
  await box.fill("Q4 review");
  await expect(page.getByRole("button", { name: "New deck" })).toBeVisible();
  await box.fill("Make a deck about the homelab for the family");
  await page.getByRole("button", { name: "✦ Draft it" }).click();
  await expect(page).toHaveURL(new RegExp(`/knowledge/a${DECK.slice(1)}`));
  expect(made).toEqual([{ title: "", theme: "light" }]);
});

test("the old Slides, Designs and Docs addresses open their start pages", async ({ page }) => {
  await page.route(/\/api\/v1\/knowledge\/query/, (route) => json(route, { columns: [], rows: [], has_more: false }));
  for (const [from, kind] of [["/apps/slides", "deck"], ["/apps/designs", "design"], ["/apps/docs", "document"]]) {
    await page.goto(from);
    await expect(page).toHaveURL(new RegExp(`/knowledge\\?kind=${kind}$`));
  }
});
