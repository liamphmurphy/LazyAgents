import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installBoardApi } from "./mock-board-api";

// The activity orb: the one floating element, replacing the persona dock.
type S = components["schemas"];

const inHours = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString();
const vesperBot: S["PersonaSummary"] = { id: "00000000-0000-4000-8000-000000000001", name: "VesperBot", color: "#9fd8c4" };
const nora: S["PersonaSummary"] = { id: "00000000-0000-4000-8000-000000000002", name: "Nora", color: "#c9b8ff" };

function chat(id: string, title: string, state: S["PersonaDockState"], harness: string, unread = 0): S["PersonaDockChat"] {
  return { id, title, state, harness, unread, last_line: "", last_seq: 3, waiting_approval: false, last_activity_at: new Date().toISOString() };
}

function entry(persona: S["PersonaSummary"], c: S["PersonaDockChat"]): S["PersonaDockEntry"] {
  return { persona, state: c.state, unread: c.unread, last_activity_at: c.last_activity_at, chats: [c] };
}

function harnesses(over: { claude?: Partial<S["HarnessActivity"]>; codex?: Partial<S["HarnessActivity"]> } = {}): S["HarnessActivity"][] {
  return [
    { harness: "claude-code", label: "Claude", running: 1, plan: "max", usage_available: true, usage_checking: false, usage_checked_at: new Date().toISOString(),
      windows: [{ id: "five_hour", kind: "session", label: "Session", used_percent: 38, resets_at: inHours(3) }, { id: "seven_day", kind: "weekly", label: "Weekly", used_percent: 61, resets_at: inHours(80) }], ...over.claude },
    { harness: "codex", label: "Codex", running: 0, plan: "plus", usage_available: true, usage_checking: false, usage_checked_at: new Date().toISOString(),
      windows: [{ id: "codex_300", kind: "session", label: "Session", used_percent: 46, resets_at: inHours(3) }], ...over.codex },
  ];
}

/** Serves the dock from `state` and lets the test push a chat event so the orb refetches. */
async function installOrb(page: Page, state: { dock: S["PersonaDock"] }) {
  await installBoardApi(page);
  await page.addInitScript(() => {
    const w = window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> };
    w.__sse = [];
    class FakeEventSource {
      onopen?: () => void;
      onerror?: () => void;
      onmessage?: (e: { data: string }) => void;
      constructor() {
        w.__sse.push(this);
        setTimeout(() => this.onopen?.(), 0);
      }
      close() {
        w.__sse = w.__sse.filter((x) => x !== this);
      }
    }
    (window as unknown as { EventSource: unknown }).EventSource = FakeEventSource;
  });
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: state.dock }));
}

async function pushChatUpdated(page: Page) {
  await page.evaluate(() => {
    const data = JSON.stringify({ type: "chat.updated", ids: [] });
    for (const s of (window as unknown as { __sse: Array<{ onmessage?: (e: { data: string }) => void }> }).__sse) s.onmessage?.({ data });
  });
}

const orb = (page: Page) => page.getByRole("region", { name: "Activity" });
const orbButton = (page: Page) => orb(page).getByRole("button", { name: /^Activity:/ });

test("shows who's working; the panel sprouts with personas and usage, and closes on Escape", async ({ page }) => {
  const state = { dock: { personas: [entry(vesperBot, chat("c1", "Harness usage spike", "working", "claude-code"))], harnesses: harnesses() } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");

  await expect(orbButton(page)).toHaveAccessibleName("Activity: VesperBot working");
  await expect(orbButton(page)).toHaveClass(/\blive\b/);
  const panel = orb(page).locator("#orb-panel");
  await expect(panel).toBeHidden();

  await orbButton(page).click();
  await expect(orbButton(page)).toHaveAttribute("aria-expanded", "true");
  await expect(panel).toBeVisible();
  const row = panel.getByRole("button", { name: /VesperBot/ });
  await expect(row).toContainText("Harness usage spike");
  await expect(row).toContainText("Working");
  await expect(panel).toContainText("1 working");
  await expect(panel.locator(".orb-urow").first()).toContainText("Claude");
  await expect(panel.locator(".orb-urow").first()).toContainText("38%");
  await expect(panel.locator(".orb-usub").first()).toContainText("weekly 61%");
  await expect(panel.locator(".orb-urow").nth(1)).toContainText("46%");
  await expect(panel.getByRole("link", { name: "Usage settings →" })).toHaveAttribute("href", "/settings/runner");

  await page.keyboard.press("Escape");
  await expect(orbButton(page)).toHaveAttribute("aria-expanded", "false");
  await expect(panel).toBeHidden();
  await expect(orbButton(page)).toBeFocused();
});

test("a persona that finishes sends a chip out of the orb; the chip opens its chat", async ({ page }) => {
  const state = { dock: { personas: [entry(vesperBot, chat("c1", "Harness usage spike", "working", "claude-code"))], harnesses: harnesses() } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");
  await expect(orbButton(page)).toHaveAccessibleName("Activity: VesperBot working");

  state.dock = { personas: [entry(vesperBot, chat("c1", "Harness usage spike", "replied", "claude-code", 1))], harnesses: harnesses({ claude: { running: 0 } }) };
  await pushChatUpdated(page);

  const chip = orb(page).locator(".orb-chip");
  await expect(chip).toHaveCount(1);
  await expect(chip).toContainText("VesperBot finished");
  await expect(chip).toContainText("Harness usage spike");
  await expect(chip).toContainText("Open");
  await expect(orb(page).locator(".orb-badge")).toHaveText("1");
  await expect(orb(page).locator("[aria-live]")).toHaveText("VesperBot finished");
  await expect(orbButton(page)).not.toHaveClass(/\blive\b/);

  await chip.click();
  await expect(page).toHaveURL(/\/chat\/c1$/);
});

test("needs-you is amber with Review, and a limited harness says when it's back", async ({ page }) => {
  const state = { dock: { personas: [entry(nora, chat("c2", "Sunken temple battlemap", "working", "codex"))], harnesses: harnesses({ claude: { running: 0 }, codex: { running: 1 } }) } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");
  await expect(orbButton(page)).toHaveAccessibleName("Activity: Nora working");

  state.dock = { personas: [entry(nora, chat("c2", "Sunken temple battlemap", "needs_you", "codex"))], harnesses: harnesses({ claude: { running: 0 }, codex: { running: 0, limited_until: inHours(2) } }) };
  await pushChatUpdated(page);

  const chip = orb(page).locator(".orb-chip.h");
  await expect(chip).toContainText("Nora needs you");
  await expect(chip).toContainText("Review");
  await expect(orb(page).locator(".orb-badge.h")).toHaveText("!");

  // Opening the panel clears the chips; the limited harness is red with its fallback.
  await orbButton(page).click();
  await expect(chip).toHaveCount(0);
  const panel = orb(page).locator("#orb-panel");
  await expect(panel.getByRole("button", { name: /Nora/ })).toContainText("Needs you");
  await expect(panel.locator(".orb-usub.red")).toContainText(/Limited until .* · falls back to Claude/);
  await expect(panel.locator(".orb-urow").nth(1)).toContainText("100%");
});

test("with nothing going on the orb dims and the panel says so", async ({ page }) => {
  const state = { dock: { personas: [], harnesses: harnesses({ claude: { running: 0, usage_available: false, usage_checked_at: undefined, windows: [] }, codex: { usage_available: false, usage_checked_at: undefined, windows: [] } }) } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");
  await expect(orbButton(page)).toHaveAccessibleName("Activity: nothing running");
  await expect(orbButton(page)).toHaveClass(/\bquiet\b/);
  await orbButton(page).click();
  const panel = orb(page).locator("#orb-panel");
  await expect(panel).toContainText("No personas working right now");
  await expect(panel).toContainText("not checked yet");
  await expect(panel.locator(".orb-usub").first()).toHaveText("Usage not checked yet");
});

// The motion was approved as a mockup; these pin its timings so a refactor can't drift from it.
test("motion keeps the approved mockup's keyframes and timings", async ({ page }) => {
  const state = { dock: { personas: [entry(vesperBot, chat("c1", "Harness usage spike", "working", "claude-code"))], harnesses: harnesses() } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");
  await expect(orbButton(page)).toHaveClass(/\blive\b/);

  const timing = (sel: string) => page.evaluate((sel) => document.querySelector(sel)!.getAnimations().map((a) => {
    const t = (a.effect as KeyframeEffect).getTiming();
    const k = (a.effect as KeyframeEffect).getKeyframes();
    return { duration: t.duration, delay: t.delay, easing: t.easing, offsets: k.map((f) => Number(f.computedOffset.toFixed(2))), first: String(k[0].clipPath ?? ""), last: String(k.at(-1)!.clipPath ?? "") };
  }), sel);

  await orbButton(page).click();
  const [sprout] = await timing(".orb-panel");
  expect(sprout).toMatchObject({ duration: 540, delay: 0, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)", offsets: [0, 0.42, 1] });
  expect(sprout.first).toMatch(/^inset\(\d+px 0px 0px \d+px round 23px\)$/);
  expect(sprout.last).toBe("inset(0px round 14px)");
  const rows = await page.evaluate(() => [...document.querySelectorAll("#orb-panel [data-stg]")].map((el) => { const t = (el.getAnimations()[0].effect as KeyframeEffect).getTiming(); return [t.duration, t.delay]; }));
  expect(rows.slice(0, 3)).toEqual([[280, 220], [280, 265], [280, 310]]);

  await page.keyboard.press("Escape");
  const [fold] = await timing(".orb-panel");
  expect(fold).toMatchObject({ duration: 380, easing: "cubic-bezier(0.4, 0, 0.2, 1)", offsets: [0, 0.58, 1] });

  state.dock = { personas: [entry(vesperBot, chat("c1", "Harness usage spike", "replied", "claude-code", 1))], harnesses: harnesses({ claude: { running: 0 } }) };
  await page.waitForTimeout(450);
  await pushChatUpdated(page);
  await expect(orb(page).locator(".orb-chip")).toHaveCount(1);
  const chipMotion = await timing(".orb-chip");
  expect(chipMotion[0]).toMatchObject({ duration: 440, delay: 120, easing: "cubic-bezier(0.2, 0.9, 0.25, 1.05)" });
  const kids = await page.evaluate(() => [...document.querySelectorAll(".orb-chip .orb-chip-k")].map((el) => (el.getAnimations()[0].effect as KeyframeEffect).getTiming().delay));
  expect(kids).toEqual([300, 360, 420]);
  const burst = await page.evaluate(() => document.querySelector(".orb-btn")!.getAnimations().filter((a) => a instanceof CSSAnimation === false).map((a) => (a.effect as KeyframeEffect).getTiming().duration));
  expect(burst).toContain(480);
});

test("rings wear each harness's colour, warn near the limit, and sweep while a check runs", async ({ page }) => {
  const claudeNearLimit = { running: 0, windows: [{ id: "five_hour", kind: "session" as const, label: "Session", used_percent: 85, resets_at: inHours(1) }] };
  const state = { dock: { personas: [], harnesses: harnesses({ claude: claudeNearLimit, codex: { usage_available: false, usage_checked_at: undefined, usage_checking: true, windows: [] } }) } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");

  const rings = orb(page).locator(".orb-btn .orb-ring");
  await expect(rings).toHaveCount(2);
  await expect(rings.nth(0)).toHaveAttribute("data-tone", "warn");
  await expect(rings.nth(1)).toHaveAttribute("data-tone", "ok");
  // Only Codex is being checked; its ring has no figure yet.
  await expect(orb(page).locator(".orb-btn .orb-sweep")).toHaveCount(1);
  expect(await rings.nth(1).evaluate((el) => getComputedStyle(el).opacity)).toBe("0");

  await orbButton(page).click();
  const panel = orb(page).locator("#orb-panel");
  await expect(panel.locator(".orb-sec").nth(1)).toContainText("checking…");
  await expect(panel.locator(".orb-urow").nth(0).locator(".orb-track i")).toHaveClass("warn");
  await expect(panel.locator(".orb-urow").nth(1)).toContainText("…");
  await expect(panel.locator(".orb-usub").nth(1)).toHaveText("Checking usage…");

  // The reading lands: the sweep stops and the ring eases to it in Codex's colour.
  state.dock = { personas: [], harnesses: harnesses({ claude: claudeNearLimit }) };
  await pushChatUpdated(page);
  await expect(orb(page).locator(".orb-btn .orb-sweep")).toHaveCount(0);
  await expect(panel.locator(".orb-urow").nth(1)).toContainText("46%");
  const codexRing = await rings.nth(1).evaluate((el) => {
    const probe = document.createElement("span");
    probe.style.color = "var(--orb-codex)";
    el.closest(".orb-root")!.appendChild(probe);
    const codex = getComputedStyle(probe).color;
    probe.remove();
    const cs = getComputedStyle(el);
    return { transition: cs.transitionDuration, stroke: cs.stroke, codex };
  });
  expect(codexRing.transition.split(", ")[0]).toBe("0.9s");
  expect(codexRing.stroke).toBe(codexRing.codex);
});

test("with reduced motion the checking sweep is hidden rather than frozen", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const state = { dock: { personas: [], harnesses: harnesses({ codex: { usage_available: false, usage_checked_at: undefined, usage_checking: true, windows: [] } }) } };
  await installOrb(page, state);
  await page.goto("/board?project=WEB");
  await expect(orb(page).locator(".orb-btn .orb-sweep")).toBeHidden();
});
