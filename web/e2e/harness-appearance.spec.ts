import { expect, test, type Page } from "@playwright/test";
import { PALETTES, contrast } from "../lib/palettes";
import { installChatApi } from "./mock-chat-api";

const footer = (page: Page) => page.getByRole("region", { name: "Harness activity" });
const drawer = (page: Page) => page.getByRole("dialog", { name: "Harness usage" });
const toggle = (page: Page) => page.getByRole("switch", { name: "Show harness activity", exact: true });

async function install(page: Page) {
  await installChatApi(page);
  await page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1"));
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: { personas: [], harnesses: [
    { harness: "claude-code", label: "Claude", running: 2, usage_available: true, usage_checking: false, windows: [{ id: "s", kind: "session", label: "Session", used_percent: 42 }] },
    { harness: "codex", label: "Codex", running: 1, usage_available: true, usage_checking: false, windows: [{ id: "s", kind: "session", label: "Session", used_percent: 86 }] },
    { harness: "antigravity", label: "Antigravity", running: 0, usage_available: false, usage_checking: false, limited_until: new Date(Date.now() + 3600000).toISOString(), windows: [] },
  ] } }));
}

test("the Appearance switch reclaims history space, persists and survives palette import/reset", async ({ page }) => {
  await install(page);
  await page.goto("/settings#appearance");
  await expect(toggle(page)).toBeChecked();
  await expect(footer(page)).toBeVisible();
  const history = page.getByRole("navigation", { name: "Chats", exact: true });
  const before = (await history.boundingBox())!.height;
  await toggle(page).click();
  await expect(footer(page)).toHaveCount(0);
  await expect.poll(async () => (await history.boundingBox())!.height).toBeGreaterThan(before + 80);
  await page.getByRole("radio", { name: "Nord", exact: true }).check();
  await page.getByRole("button", { name: "Reset palette", exact: true }).click();
  await expect(toggle(page)).not.toBeChecked();
  await page.getByLabel("Palette JSON file").setInputFiles({ name: "nord.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify({ version: 1, palette: "nord", showHarnessActivity: true })) });
  await expect(page.getByRole("radio", { name: "Nord", exact: true })).toBeChecked();
  await expect(toggle(page)).not.toBeChecked();
  await page.reload();
  await expect(toggle(page)).not.toBeChecked();
  await expect(footer(page)).toHaveCount(0);
  await expect(page.locator("html")).toHaveAttribute("data-harness-activity", "hidden");
  await page.goto("/chat?new");
  await expect(footer(page)).toHaveCount(0);
  await page.goto("/settings#appearance");
  await toggle(page).click();
  await expect(footer(page)).toBeVisible();
  await page.getByRole("button", { name: "Collapse menu", exact: true }).click();
  await expect(footer(page)).toHaveAttribute("data-rail", "true");
  await toggle(page).click();
  await expect(footer(page)).toHaveCount(0);
});

test("hiding activity in another tab also closes its drawer and frees the history", async ({ page, context }) => {
  await install(page);
  await page.goto("/settings#appearance");
  const other = await context.newPage();
  await install(other);
  await other.goto("/chat?new");
  await footer(other).getByRole("button", { name: /Claude, working/ }).click();
  await expect(drawer(other)).toBeVisible();
  await toggle(page).click();
  await expect(footer(other)).toHaveCount(0);
  await expect(drawer(other)).toHaveCount(0);
  await toggle(page).click();
  await expect(footer(other)).toBeVisible();
  await footer(other).getByRole("button", { name: /Claude, working/ }).click();
  await expect(drawer(other)).toBeVisible();
});

function hex(rgb: string) {
  return "#" + rgb.match(/[\d.]+/g)!.slice(0, 3).map((n) => Math.round(Number(n)).toString(16).padStart(2, "0")).join("");
}

async function colors(page: Page) {
  return page.evaluate(() => {
    const sample = (selector: string, key: string) => getComputedStyle(document.querySelector(selector)!).getPropertyValue(key);
    const token = (key: string) => {
      const probe = document.createElement("i");
      probe.style.color = `var(--${key})`;
      document.body.appendChild(probe);
      const value = getComputedStyle(probe).color;
      probe.remove();
      return value;
    };
    const paints = [...document.querySelectorAll('.harness-footer .harness-character svg, .harness-footer .harness-character svg *')].map((el) => {
      const s = getComputedStyle(el);
      return [s.color, s.fill, s.stroke];
    });
    return {
      paints,
      footer: sample(".harness-footer", "background-color"), sidebar: token("bg-sidebar"),
      label: sample(".harness-name", "color"), secondary: token("text-secondary"),
      surface: sample(".harness-drawer", "background-color"), raised: token("surface-raised"),
      value: sample('.harness-usage-row[data-tone="ok"] .harness-usage-value', "color"), text: token("text"),
      bar: sample('.harness-usage-row[data-tone="ok"] .harness-usage-track i', "background-color"), accent: token("accent"),
      track: sample(".harness-usage-track", "background-color"), sunken: token("surface-sunken"),
      warn: sample('.harness-usage-row[data-tone="warn"] .harness-usage-value', "color"), human: token("pill-human-fg"),
      limited: sample('.harness-usage-row[data-tone="limited"] .harness-usage-value', "color"), blocked: token("pill-blocked-fg"),
    };
  });
}

test("every palette and mode theme the UI while all character paints stay constant", async ({ page }) => {
  test.setTimeout(60_000);
  await install(page);
  await page.goto("/settings#appearance");
  await expect(footer(page)).toBeVisible();
  let paints: string[][] | undefined;
  for (const palette of PALETTES) {
    await page.getByRole("radio", { name: palette.name, exact: true }).check();
    for (const mode of ["Dark", "Light"]) {
      await page.getByRole("radio", { name: mode, exact: true }).click();
      await expect(page.locator("html")).toHaveAttribute("data-theme", mode.toLowerCase());
      await footer(page).getByRole("button", { name: /Claude, working/ }).click();
      await expect(drawer(page)).toBeVisible();
      const c = await colors(page);
      paints ??= c.paints;
      expect(c.paints, `${palette.name} ${mode} character colors`).toEqual(paints);
      expect(c.footer).toBe(c.sidebar);
      expect(c.label).toBe(c.secondary);
      expect(c.surface).toBe(c.raised);
      expect(c.value).toBe(c.text);
      expect(c.bar).toBe(c.accent);
      expect(c.track).toBe(c.sunken);
      expect(c.warn).toBe(c.human);
      expect(c.limited).toBe(c.blocked);
      for (const fg of [c.value, c.warn, c.limited]) expect(contrast(hex(fg), hex(c.surface)), `${palette.name} ${mode} value contrast`).toBeGreaterThanOrEqual(4.5);
      expect(contrast(hex(c.label), hex(c.footer)), `${palette.name} ${mode} label contrast`).toBeGreaterThanOrEqual(4.5);
      for (const fg of [c.bar, c.warn, c.limited]) expect(contrast(hex(fg), hex(c.track)), `${palette.name} ${mode} bar contrast`).toBeGreaterThanOrEqual(3);
      await page.keyboard.press("Escape");
    }
  }
  await page.getByRole("button", { name: "Customize this palette", exact: true }).click();
  for (const [mode, value] of [["Dark", "#88ccee"], ["Light", "#245266"]]) {
    await page.getByRole("radio", { name: `${mode} colors`, exact: true }).click();
    await page.getByRole("button", { name: "Accent", exact: true }).click();
    await page.getByRole("textbox", { name: "Hex color", exact: true }).fill(value);
    await page.getByRole("radio", { name: mode, exact: true }).click();
    await footer(page).getByRole("button", { name: /Claude, working/ }).click();
    const c = await colors(page);
    expect(c.paints).toEqual(paints);
    expect(hex(c.bar)).toBe(value);
    await page.keyboard.press("Escape");
  }
});
