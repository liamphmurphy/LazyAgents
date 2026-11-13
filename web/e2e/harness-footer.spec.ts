import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installBoardApi } from "./mock-board-api";
import { installChatApi, openEmptyChat } from "./mock-chat-api";

type S = components["schemas"];
const inHours = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString();

function harnesses(over: { claude?: Partial<S["HarnessActivity"]>; codex?: Partial<S["HarnessActivity"]>; agy?: Partial<S["HarnessActivity"]> } = {}): S["HarnessActivity"][] {
  return [
    { harness: "claude-code", label: "Claude", running: 2, usage_available: true, usage_checking: false, usage_checked_at: new Date().toISOString(),
      windows: [{ id: "five_hour", kind: "session", label: "Session", used_percent: 42, resets_at: inHours(2) }, { id: "seven_day", kind: "weekly", label: "Weekly", used_percent: 68 }], ...over.claude },
    { harness: "codex", label: "Codex", running: 1, usage_available: true, usage_checking: false, usage_checked_at: new Date().toISOString(),
      windows: [{ id: "codex_300", kind: "session", label: "Session", used_percent: 18, resets_at: inHours(3) }], ...over.codex },
    { harness: "antigravity", label: "Antigravity", running: 0, usage_available: false, usage_checking: false, windows: [], ...over.agy },
  ];
}

async function installFooter(page: Page, state: { dock: S["PersonaDock"] }, expanded = true) {
  await installBoardApi(page);
  await page.addInitScript((expanded) => {
    if (expanded) localStorage.setItem("lazyagents.sidebar-expanded", "1");
    const w = window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> };
    w.__sse = [];
    class FakeEventSource {
      onopen?: () => void;
      onmessage?: (e: { data: string }) => void;
      constructor() { w.__sse.push(this); setTimeout(() => this.onopen?.(), 0); }
      close() { w.__sse = w.__sse.filter((x) => x !== this); }
    }
    (window as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
  }, expanded);
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: state.dock }));
}

async function refresh(page: Page) {
  await page.evaluate(() => {
    const data = JSON.stringify({ type: "chat.updated", ids: [] });
    for (const s of (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse) s.onmessage?.({ data });
  });
}

const footer = (page: Page) => page.getByRole("region", { name: "Harness activity" });
const drawer = (page: Page) => page.getByRole("dialog", { name: "Harness usage" });

test("the flat footer replaces the profile and floating orb; settings sit beside collapse", async ({ page }) => {
  await installFooter(page, { dock: { personas: [], harnesses: harnesses() } });
  await page.goto("/board?project=WEB");
  await expect(footer(page).getByRole("button", { name: "Harness usage: 2 working" })).toBeVisible();
  await expect(footer(page).getByRole("button", { name: /show usage/ })).toHaveCount(3);
  await expect(page.locator(".orb-root")).toHaveCount(0);
  const menu = page.getByRole("complementary", { name: "Main menu" });
  await expect(menu.getByText("Liam", { exact: true })).toHaveCount(0);
  const settings = await menu.getByRole("link", { name: "Settings", exact: true }).boundingBox();
  const customization = await menu.getByRole("link", { name: "Customization", exact: true }).boundingBox();
  const collapse = await menu.getByRole("button", { name: "Collapse menu" }).boundingBox();
  expect(Math.abs(settings!.y - collapse!.y)).toBeLessThan(5);
  expect(Math.abs(customization!.y - collapse!.y)).toBeLessThan(5);
  const bounds = await menu.boundingBox();
  const bottom = await footer(page).boundingBox();
  expect(Math.abs(bottom!.x - bounds!.x)).toBeLessThan(1);
  expect(Math.abs(bottom!.y + bottom!.height - bounds!.y - bounds!.height)).toBeLessThan(1);
  expect(await footer(page).evaluate((el) => getComputedStyle(el).borderRadius)).toBe("0px");
  await expect(footer(page).locator(".harness-character").first()).toHaveCSS("width", "44px");
});

test("characters open real usage details and Escape restores focus to the clicked character", async ({ page }) => {
  await installFooter(page, { dock: { personas: [], harnesses: harnesses() } });
  await page.goto("/board?project=WEB");
  const trigger = footer(page).getByRole("button", { name: "Codex, working; show usage" });
  await trigger.click();
  await expect(drawer(page)).toBeVisible();
  await expect(drawer(page)).toContainText("3 agents working");
  await expect(drawer(page).getByRole("progressbar", { name: "Claude allowance used" })).toHaveAttribute("aria-valuenow", "42");
  await expect(drawer(page).getByRole("progressbar", { name: "Codex allowance used" })).toHaveAttribute("aria-valuenow", "18");
  await expect(drawer(page)).toContainText("weekly 68%");
  await expect(drawer(page)).toContainText(/5h · resets/);
  await expect(drawer(page)).toContainText("Antigravity doesn’t report an allowance");
  await expect(drawer(page).getByRole("link", { name: "Usage settings →" })).toHaveAttribute("href", "/settings/runner");
  await page.keyboard.press("Escape");
  await expect(drawer(page)).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await footer(page).getByRole("button", { name: /Harness usage:/ }).click();
  await expect(drawer(page)).toBeHidden();
  await trigger.click();
  await page.mouse.click(1000, 70);
  await expect(drawer(page)).toBeHidden();
});

test("working follows live harness activity; the approved characters idle at rest", async ({ page }) => {
  const state = { dock: { personas: [], harnesses: harnesses({ claude: { running: 0 }, codex: { running: 0 } }) } };
  await installFooter(page, state);
  await page.goto("/board?project=WEB");
  const body = (kind: string) => footer(page).locator(`.harness-character.${kind} .body`);
  await expect(body("robot")).toHaveCSS("animation-name", "harness-breathe");
  await expect(body("comet")).toHaveCSS("animation-name", "harness-float");
  await expect(body("jet")).toHaveCSS("animation-name", "harness-float");
  state.dock.harnesses = harnesses({ agy: { running: 1 } });
  await refresh(page);
  await expect(body("robot")).toHaveCSS("animation-name", "harness-tap");
  await expect(body("comet")).toHaveCSS("animation-name", "harness-comet");
  await expect(body("jet")).toHaveCSS("animation-name", "harness-boost");
  await expect(footer(page).getByRole("button", { name: "Harness usage: 3 working" })).toBeVisible();
  state.dock.harnesses = harnesses({ codex: { limited_until: inHours(1) } });
  await refresh(page);
  await expect(footer(page).getByRole("button", { name: "Codex, limited; show usage" })).toBeVisible();
  await expect(body("comet")).toHaveCSS("animation-name", "harness-float");
});

test("zero, unavailable, checking, warning and limited readings stay distinct", async ({ page }) => {
  const state = { dock: { personas: [], harnesses: harnesses({ claude: { windows: [{ id: "s", kind: "session" as const, label: "Session", used_percent: 0 }], usage_checking: true }, codex: { usage_available: false, usage_checking: true, windows: [] } }) } };
  await installFooter(page, state);
  await page.goto("/board?project=WEB");
  await footer(page).getByRole("button", { name: /Claude, working/ }).click();
  const claude = drawer(page).locator('[data-harness="claude-code"].harness-usage-row');
  const codex = drawer(page).locator('[data-harness="codex"].harness-usage-row');
  await expect(claude).toContainText("0%");
  await expect(claude.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");
  await expect(codex).toContainText("Checking usage…");
  await expect(codex.getByRole("progressbar")).toHaveCount(0);
  await expect(codex.locator(".harness-usage-track")).toHaveAttribute("data-unavailable", "true");
  state.dock.harnesses = harnesses({ claude: { usage_checking: true, windows: [{ id: "s", kind: "session", label: "Session", used_percent: 86 }] }, codex: { limited_until: inHours(1) } });
  await refresh(page);
  await expect(claude).toContainText("86%");
  await expect(claude).toHaveAttribute("data-tone", "warn");
  await expect(codex).toHaveAttribute("data-tone", "limited");
  await expect(codex).toContainText("100%");
  await expect(codex).toContainText(/Limited until .*falls back to Claude/);
});

test("the collapsed rail keeps the footer and opens a drawer inside the viewport", async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 650 });
  await installFooter(page, { dock: { personas: [], harnesses: [] } }, false);
  await page.goto("/board?project=WEB");
  await expect(footer(page)).toHaveAttribute("data-rail", "true");
  await expect(footer(page).getByRole("button", { name: /show usage/ })).toHaveCount(3);
  await expect(footer(page).locator(".harness-character").first()).toHaveCSS("width", "30px");
  await footer(page).getByRole("button", { name: /Antigravity, idle/ }).click();
  await expect(drawer(page)).toBeVisible();
  await expect(drawer(page).getByRole("progressbar")).toHaveCount(0);
  await expect.poll(async () => (await drawer(page).boundingBox())!.x).toBeGreaterThanOrEqual(50);
  const bounds = await drawer(page).boundingBox();
  expect(bounds!.y).toBeGreaterThanOrEqual(11);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(639);
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Expand menu", exact: true }).click();
  await expect(footer(page)).toHaveAttribute("data-rail", "false");
  await expect(footer(page).locator(".harness-character").first()).toHaveCSS("width", "44px");
});

test("reduced motion holds all three characters still and opens the drawer without animation", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await installFooter(page, { dock: { personas: [], harnesses: harnesses({ agy: { running: 1 } }) } });
  await page.goto("/board?project=WEB");
  const animations = await footer(page).locator(".harness-character svg *").evaluateAll((els) => els.map((e) => getComputedStyle(e).animationName));
  expect(animations.length).toBeGreaterThan(0);
  expect(new Set(animations)).toEqual(new Set(["none"]));
  await footer(page).getByRole("button", { name: /Antigravity, working/ }).click();
  await expect(drawer(page)).toHaveCSS("animation-name", "none");
});

for (const width of [1280, 390]) {
  test(`the visible chat is marked read after a reply at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 });
    await installChatApi(page);
    const id = await openEmptyChat(page);
    const chat: S["PersonaDockChat"] = { id, title: "Read tracking", state: "replied", harness: "codex", unread: 1, last_line: "A reply", last_seq: 3, waiting_approval: false, last_activity_at: new Date().toISOString() };
    const state: S["PersonaDock"] = { personas: [{ persona: { id: "00000000-0000-4000-8000-000000000001", name: "Assistant", color: "#9fd8c4" }, state: "replied", unread: 1, last_activity_at: chat.last_activity_at, chats: [chat] }], harnesses: [] };
    const reads: number[] = [];
    await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: state }));
    await page.route(`**/api/v1/chats/${id}/read`, (route) => {
      reads.push(route.request().postDataJSON().seq);
      chat.unread = 0;
      return route.fulfill({ status: 204 });
    });
    await refresh(page);
    await expect.poll(() => reads).toContain(3);
    chat.unread = 1;
    chat.last_seq = 4;
    await refresh(page);
    await expect.poll(() => reads).toContain(4);
  });
}
