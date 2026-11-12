import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Designs from chat (LAZYAGENTS-151, docs/design/designs-from-chat.svg): the + menu and
// /design ask the chat's persona for a design, the design it starts opens beside the chat,
// and the reply's Design card shows a thumbnail, counts, Present/Open and the review badge.

test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

const D = "44444444-4444-4444-8444-444444444444";
const at = "2026-11-12T12:00:00Z";
const acceptance = readFileSync(join(__dirname, "../../docs/design/fixtures/acceptance-design.json"), "utf8");
const PNG = readFileSync(join(__dirname, "../../docs/design/fixtures/landscape.png"));

async function install(page: Page) {
  const s = { created: false, pending: 2, paused: false, settings: [] as Record<string, unknown>[] };
  await installChatApi(page, { replyDelayMs: 10 });
  const json = (route: Route, data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases(\/|\?|$)/, (route) =>
    json(route, { canvases: s.created ? [{ id: D, kind: "design", title: "Onboarding", seq: 1, created_at: at, updated_at: at }] : [] }),
  );
  await page.route(/\/api\/v1\/designs\//, (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname.replace(`/api/v1/designs/${D}`, "");
    if (p === "/render") return route.fulfill({ status: 200, contentType: "image/png", body: PNG });
    if (p === "/versions/1") return json(route, { seq: 1, note: "", author_type: "agent", created_at: at, body: acceptance });
    if (p === "/proposals") return json(route, { proposals: [] });
    if (p === "/settings") {
      const b = req.postDataJSON() as { persona_paused?: boolean };
      s.settings.push(b);
      if (b.persona_paused !== undefined) s.paused = b.persona_paused;
      return json(route, { direct_edit: false, persona_paused: s.paused });
    }
    if (p === "") return json(route, { id: D, kind: "design", title: "Onboarding", chat_id: "x", chat_title: "x", updated_at: at, reviewed_seq: 0, seq: 1, settings: { direct_edit: false, persona_paused: s.paused }, pending_proposals: s.pending });
    return json(route, { code: "not_found", message: "not found" }, 404);
  });
  await page.goto("/chat?project=WEB");
  return s;
}

const box = (page: Page) => page.getByRole("combobox", { name: "Message" });

test("the + menu and /design ask the persona for a design", async ({ page }) => {
  await install(page);
  await page.getByRole("button", { name: "Add context" }).click();
  const create = page.getByTestId("create-with");
  await expect(create).toContainText(/Create with/);
  await expect(create.getByRole("button")).toHaveText([/Doc/, /Slides/, /Design.*Flow, diagram or wireframe.*\/design/]);
  await create.getByRole("button", { name: /Design/ }).click();
  await expect(box(page)).toHaveValue("/design ");
  await box(page).pressSequentially("onboarding for the Designs app");
  await box(page).press("Enter");
  await expect(page.locator(":text-is(\"Create a design: onboarding for the Designs app\"):visible").first()).toBeVisible();
  await expect(box(page)).toHaveValue("");

  await box(page).fill("/design ");
  await box(page).press("Enter");
  await expect(page.getByTestId("command-error")).toHaveText("Say what to design: /design <what to design>");
});

test("a design the persona starts opens beside the chat and leaves a Design card", async ({ page }) => {
  const s = await install(page);
  // The chat exists before the persona's design_create does.
  await box(page).fill("Now sketch onboarding");
  await box(page).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[^/?]+/);
  await expect(page.getByTestId("design-chat-card")).toHaveCount(0);
  s.created = true;
  await box(page).fill(`canvas: ${D}`);
  await box(page).press("Enter");

  const panel = page.getByTestId("design-panel");
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("heading", { name: "Onboarding" })).toBeVisible();
  await expect(panel.getByRole("toolbar", { name: "Tools" }).getByRole("button")).toHaveCount(6);

  const card = page.getByTestId("design-chat-card");
  await expect(card).toContainText("Onboarding");
  await expect(card.getByTestId("design-card-counts")).toHaveText("4 frames · 3 links · 3 pages");
  await expect(card.getByTestId("design-card-thumb")).toHaveAttribute("src", `/api/v1/designs/${D}/render?v=1`);
  await expect(card.getByTestId("design-card-review")).toHaveText("2 to review");

  // Pause stops the persona's patches until resumed.
  await panel.getByTestId("design-pause").click();
  await expect.poll(() => s.settings).toEqual([{ persona_paused: true }]);
  await expect(panel.getByTestId("design-pause")).toHaveText("Resume");
  await panel.getByTestId("design-pause").click();
  await expect.poll(() => s.settings.at(-1)).toEqual({ persona_paused: false });

  // Accepting the suggestions elsewhere clears the badge on the next refresh.
  s.pending = 0;
  await panel.getByRole("button", { name: "Close the design" }).click();
  await expect(panel).toHaveCount(0);
  await box(page).fill(`canvas: ${D}`);
  await box(page).press("Enter");
  await expect(card.first().getByTestId("design-card-review")).toHaveCount(0);

  await card.first().getByRole("button", { name: "Open", exact: true }).click();
  await expect(page.getByTestId("design-panel")).toBeVisible();
  await card.first().getByRole("button", { name: "Present" }).click();
  await expect(page).toHaveURL(new RegExp(`/present/designs/${D}`));
});

// The chat's panels share the apps' dock (LAZYAGENTS-165): a tab per open panel plus Activity.
const dock = (page: Page) => page.getByRole("complementary", { name: "Panel" });
const rail = (page: Page) => page.getByRole("navigation", { name: "Panel rail" });
const dockTab = (page: Page, name: string) => dock(page).getByRole("tablist", { name: "Panel tabs" }).getByRole("tab", { name: new RegExp(name) });
const mod = process.platform === "darwin" ? "Meta" : "Control";

async function openDesign(page: Page) {
  const s = await install(page);
  await box(page).fill("Now sketch onboarding");
  await box(page).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[^/?]+/);
  s.created = true;
  await box(page).fill(`canvas: ${D}`);
  await box(page).press("Enter");
  // It opens by itself when the chat saw it appear; if the chat's canvases first loaded with it there, open it from its card.
  const card = page.getByTestId("design-chat-card").first();
  await expect(card).toBeVisible();
  if (!(await page.getByTestId("design-panel").isVisible())) await card.getByRole("button", { name: "Open", exact: true }).click();
  await expect(page.getByTestId("design-panel")).toBeVisible();
  return s;
}

test("with a design open, the dock collapses to its rail and reopens, and the URL keeps it", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openDesign(page);
  await expect(dockTab(page, "Design")).toHaveAttribute("aria-selected", "true");
  await expect(dockTab(page, "Activity")).toBeVisible();
  await expect(rail(page)).toHaveCount(0);

  // ⇥ collapses it in place; the rail's icon reopens that tab.
  await dock(page).getByRole("button", { name: "Collapse panel" }).click();
  await expect(dock(page)).toHaveCount(0);
  await expect(rail(page).getByRole("button", { name: "Open Design" })).toBeVisible();
  await rail(page).getByRole("button", { name: "Open Design" }).click();
  await expect(page.getByTestId("design-panel")).toBeVisible();

  // Clicking the active tab collapses it; ⌘\ reopens it.
  await dockTab(page, "Design").click();
  await expect(rail(page)).toBeVisible();
  await page.keyboard.press(`${mod}+Backslash`);
  await expect(dockTab(page, "Design")).toHaveAttribute("aria-selected", "true");

  // Activity sits beside the conversation, and a reload keeps the tab and the open panels.
  await dockTab(page, "Activity").click();
  await expect(page.getByTestId("chat-activity")).toBeVisible();
  await expect(page.getByRole("log", { name: "Conversation" })).toBeVisible();
  await page.keyboard.press(`${mod}+Backslash`);
  await expect(rail(page)).toBeVisible();
  await page.reload();
  await expect(rail(page).getByRole("button", { name: "Open Design" })).toBeVisible();
  await expect(dock(page)).toHaveCount(0);
  await rail(page).getByRole("button", { name: "Open Activity" }).click();
  await expect(dockTab(page, "Activity")).toHaveAttribute("aria-selected", "true");
  await page.reload();
  await expect(dockTab(page, "Activity")).toHaveAttribute("aria-selected", "true");
  await expect(dockTab(page, "Design")).toBeVisible();

  // Closing the design removes its tab.
  await dockTab(page, "Design").click();
  await page.getByTestId("design-panel").getByRole("button", { name: "Close the design" }).click();
  await expect(dockTab(page, "Design")).toHaveCount(0);
  await expect(dockTab(page, "Activity")).toHaveAttribute("aria-selected", "true");
});

test("⌘. hides the Vesper sidebar and the dock, and restores both without touching the preference", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openDesign(page);
  const menu = page.getByRole("complementary", { name: "Main menu" });
  await expect(menu).toBeVisible();
  await expect(dock(page)).toBeVisible();

  await page.keyboard.press(`${mod}+Period`);
  await expect(menu).toHaveCount(0);
  await expect(dock(page)).toHaveCount(0);
  await expect(rail(page)).toHaveCount(0);
  await expect(page.getByRole("log", { name: "Conversation" })).toBeVisible();

  await page.keyboard.press(`${mod}+Period`);
  await expect(menu).toBeVisible();
  await expect(menu).toHaveAttribute("data-expanded", "true");
  await expect(dockTab(page, "Design")).toHaveAttribute("aria-selected", "true");
  expect(await page.evaluate(() => localStorage.getItem("lazyagents.sidebar-expanded"))).toBe("1");
});

test("a design out of sight in the dock doesn't take editing keys", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const writes: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET" && r.url().includes(`/api/v1/designs/${D}`) && !r.url().endsWith("/settings")) writes.push(`${r.method()} ${r.url()}`);
  });
  await openDesign(page);
  const panel = page.getByTestId("design-panel");
  const selectAll = async () => {
    await panel.locator("svg").first().click({ position: { x: 5, y: 5 } });
    await page.keyboard.press(`${mod}+a`);
  };

  // Behind the Activity tab, Backspace with the focus on the page deletes nothing.
  await selectAll();
  await dockTab(page, "Activity").click();
  await page.getByRole("log", { name: "Conversation" }).click({ position: { x: 20, y: 20 } });
  await page.keyboard.press("Backspace");
  await page.keyboard.press("ArrowRight");
  // Nor in the collapsed dock.
  await page.keyboard.press(`${mod}+Backslash`);
  await expect(rail(page)).toBeVisible();
  await page.keyboard.press("Backspace");
  await page.waitForTimeout(1500);
  expect(writes).toEqual([]);

  // In sight it does (so the check above would have seen a deletion).
  await rail(page).getByRole("button", { name: /Open Design/ }).click();
  await selectAll();
  await page.keyboard.press("Backspace");
  await expect.poll(() => writes.length, { timeout: 5000 }).toBeGreaterThan(0);
});
