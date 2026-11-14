import { expect, test, type Page } from "@playwright/test";
import { PALETTES, contrast } from "../lib/palettes";
import { installChatApi } from "./mock-chat-api";

const footer = (page: Page) => page.getByRole("region", { name: "Harness activity" });
const bubble = (page: Page, label: string) => page.getByRole("dialog", { name: `${label} usage` });
const toggle = (page: Page) => page.getByRole("switch", { name: "Show harness activity", exact: true });

async function install(page: Page) {
  await installChatApi(page);
  await page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1"));
  await page.route("**/api/v1/persona-dock", (route) => route.fulfill({ json: { personas: [], harnesses: [
    { harness: "claude-code", label: "Claude", connected: true, running: 2, usage_available: true, usage_checking: false, windows: [{ id: "s", kind: "session", label: "Session", used_percent: 42 }] },
    { harness: "codex", label: "Codex", connected: true, running: 1, usage_available: true, usage_checking: false, windows: [{ id: "s", kind: "session", label: "Session", used_percent: 86 }] },
    { harness: "antigravity", label: "Antigravity", connected: true, running: 0, usage_available: false, usage_checking: false, limited_until: new Date(Date.now() + 3600000).toISOString(), windows: [] },
  ] } }));
}

test("the Appearance switch hides the rail's characters, persists and survives palette import/reset", async ({ page }) => {
  await install(page);
  await page.goto("/settings#appearance");
  await expect(toggle(page)).toBeChecked();
  // The characters live at the foot of the rail on every screen.
  await expect(footer(page)).toHaveAttribute("data-rail", "true");
  await toggle(page).click();
  await expect(footer(page)).toHaveCount(0);
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
  await expect(footer(page)).toHaveAttribute("data-rail", "true");
  await toggle(page).click();
  await expect(footer(page)).toHaveCount(0);
});

test("hiding activity in another tab also closes its bubble and frees the history", async ({ page, context }) => {
  await install(page);
  await page.goto("/settings#appearance");
  const other = await context.newPage();
  await install(other);
  await other.goto("/chat?new");
  await footer(other).getByRole("button", { name: /Claude, working/ }).click();
  await expect(bubble(other, "Claude")).toBeVisible();
  await toggle(page).click();
  await expect(footer(other)).toHaveCount(0);
  await expect(bubble(other, "Claude")).toHaveCount(0);
  await toggle(page).click();
  await expect(footer(other)).toBeVisible();
  await footer(other).getByRole("button", { name: /Claude, working/ }).click();
  await expect(bubble(other, "Claude")).toBeVisible();
});

function hex(rgb: string) {
  return "#" + rgb.match(/[\d.]+/g)!.slice(0, 3).map((n) => Math.round(Number(n)).toString(16).padStart(2, "0")).join("");
}

/** Footer chrome and the open bubble's colors, beside the tokens they should match. */
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
      surface: sample(".harness-bubble", "background-color"), raised: token("surface-raised"),
      value: sample(".harness-bubble .harness-usage-value", "color"),
      bar: sample(".harness-bubble .harness-usage-track i", "background-color"),
      track: sample(".harness-bubble .harness-usage-track", "background-color"), sunken: token("surface-sunken"),
      text: token("text"), accent: token("accent"), human: token("pill-human-fg"), blocked: token("pill-blocked-fg"),
    };
  });
}

/** Opens one harness's bubble and returns its colors, closing it again. */
async function bubbleColors(page: Page, label: string) {
  await footer(page).getByRole("button", { name: new RegExp(`^${label},`) }).click();
  await expect(bubble(page, label)).toBeVisible();
  const c = await colors(page);
  await page.keyboard.press("Escape");
  await expect(bubble(page, label)).toHaveCount(0);
  return c;
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
      // Claude is ok, Codex near its limit (warn), Antigravity limited.
      const ok = await bubbleColors(page, "Claude"), warn = await bubbleColors(page, "Codex"), limited = await bubbleColors(page, "Antigravity");
      paints ??= ok.paints;
      expect(ok.paints, `${palette.name} ${mode} character colors`).toEqual(paints);
      expect(ok.footer).toBe(ok.sidebar);
      expect(ok.label).toBe(ok.secondary);
      expect(ok.surface).toBe(ok.raised);
      expect(ok.value).toBe(ok.text);
      expect(ok.bar).toBe(ok.accent);
      expect(ok.track).toBe(ok.sunken);
      expect(warn.value).toBe(warn.human);
      expect(warn.bar).toBe(warn.human);
      expect(limited.value).toBe(limited.blocked);
      for (const fg of [ok.value, warn.value, limited.value]) expect(contrast(hex(fg), hex(ok.surface)), `${palette.name} ${mode} value contrast`).toBeGreaterThanOrEqual(4.5);
      expect(contrast(hex(ok.label), hex(ok.footer)), `${palette.name} ${mode} label contrast`).toBeGreaterThanOrEqual(4.5);
      for (const fg of [ok.bar, warn.bar, limited.bar]) expect(contrast(hex(fg), hex(ok.track)), `${palette.name} ${mode} bar contrast`).toBeGreaterThanOrEqual(3);
    }
  }
  await page.getByRole("button", { name: "Customize this palette", exact: true }).click();
  for (const [mode, value] of [["Dark", "#88ccee"], ["Light", "#245266"]]) {
    await page.getByRole("radio", { name: `${mode} colors`, exact: true }).click();
    await page.getByRole("button", { name: "Accent", exact: true }).click();
    await page.getByRole("textbox", { name: "Hex color", exact: true }).fill(value);
    await page.getByRole("radio", { name: mode, exact: true }).click();
    const c = await bubbleColors(page, "Claude");
    expect(c.paints).toEqual(paints);
    expect(hex(c.bar)).toBe(value);
  }
});
