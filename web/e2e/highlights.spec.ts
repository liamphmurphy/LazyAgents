import { expect, test } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";
import type { components } from "../lib/api/schema";

type Bucket = components["schemas"]["HighlightsBucket"];
const base: Bucket = { persona_id: "p1", persona: "VesperBot", harness: "codex", model: "model-a", effort: "high", effort_level: "xhigh", reasons: ["confirmation"], turns: 3, duration_ms: 9000, timed_turns: 3 };

test("Highlights shows Auto routing and keeps scope when switching tabs", async ({ page }) => {
  await installBoardApi(page);
  await page.route("**/api/v1/highlights?*", (route) => route.fulfill({ json: { buckets: [base, { ...base, persona_id: "p2", persona: "Guest", model: "model-b", effort: "medium", effort_level: "", reasons: [], turns: 2, duration_ms: 0, timed_turns: 0 }, { ...base, persona_id: "", persona: "Unknown persona", effort: "", effort_level: "", turns: 1, duration_ms: 0, timed_turns: 0 }] } }));
  await page.goto("/runs?tab=highlights&project=WEB");
  const main = page.getByRole("main");
  await expect(main.getByRole("region", { name: "Reply turns", exact: true })).toContainText("6");
  await expect(main.getByRole("region", { name: "Automatic effort", exact: true })).toContainText("50.0%");
  await expect(main.getByRole("region", { name: "Average turn time" })).toContainText("3.0s");
  await expect(main.getByRole("region", { name: "Model usage" })).toContainText("do not identify automatic model selection");
  await page.getByRole("combobox", { name: "Persona", exact: true }).selectOption("p2");
  await expect(main.getByRole("region", { name: "Reply turns", exact: true })).toContainText("2 chosen effort");
  await page.getByRole("radio", { name: "Auto only", exact: true }).click();
  await expect(main.getByText("No reply turns in this selection")).toBeVisible();
  await page.getByRole("combobox", { name: "Persona", exact: true }).selectOption("p1");
  await expect(main.getByRole("region", { name: "Requested Auto effort" })).toContainText("Extra high");
  await expect(main.getByRole("region", { name: "Reasons effort was raised" })).toContainText("Go-ahead");
  await expect(page.getByRole("link", { name: "Costs", exact: true })).toHaveAttribute("href", "/runs?tab=costs&project=WEB");
  await expect(page.getByRole("link", { name: "Highlights", exact: true })).toHaveAttribute("href", "/runs?tab=highlights&project=WEB");
  await page.screenshot({ path: "/tmp/routing-highlights.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(main.getByRole("region", { name: "Requested Auto effort" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: "/tmp/routing-highlights-mobile.png", fullPage: true });
});

test("Highlights handles empty history, request errors and invalid dates", async ({ page }) => {
  await installBoardApi(page);
  let fail = true;
  await page.route("**/api/v1/highlights?*", (route) => route.fulfill(fail ? { status: 500, json: { message: "Unavailable" } } : { json: { buckets: [] } }));
  await page.goto("/runs?tab=highlights&project=WEB");
  await expect(page.getByText("Couldn’t load highlights")).toBeVisible({ timeout: 15000 });
  fail = false;
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByText("No reply turns in this selection")).toBeVisible();
  await page.goto("/runs?tab=highlights&project=WEB&range=custom&from=2026-11-10&to=2026-11-01");
  await expect(page.getByText("The end day must not be before the start day.")).toBeVisible();
});
