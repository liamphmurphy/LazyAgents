import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Themed diagrams, step-throughs and charts in a chat reply.

async function send(page: Page, text: string) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill(text);
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
}

const decoded = (src: string | null) => decodeURIComponent((src ?? "").split(",").slice(1).join(","));

test("a palette diagram follows the theme and keeps its alt text", async ({ page }) => {
  await send(page, "viz: all");
  const fence = page.getByTestId("svg-fence").first();
  const img = fence.getByRole("img", { name: "Client and server" });
  await expect(img).toBeVisible();
  await page.evaluate(() => document.documentElement.setAttribute("data-theme", "dark"));
  await expect.poll(async () => decoded(await img.getAttribute("src"))).toContain('fill="#1F1D2C"');
  await page.evaluate(() => document.documentElement.setAttribute("data-theme", "light"));
  await expect.poll(async () => decoded(await img.getAttribute("src"))).toContain('fill="#F3F1F8"');
  expect(decoded(await img.getAttribute("src"))).not.toContain("--viz-");

  await expect(fence.getByRole("link", { name: "Download SVG" })).toHaveAttribute("href", /%23F3F1F8/);
  await fence.getByRole("button", { name: "Zoom" }).click();
  const dialog = page.getByRole("dialog", { name: "SVG image" });
  await expect(dialog.getByRole("img", { name: "Client and server" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("a step-through reveals its frames with captions", async ({ page }) => {
  await send(page, "viz: all");
  const fence = page.getByTestId("svg-fence").nth(1);
  await expect(fence.getByText("The client starts alone")).toBeVisible();
  const first = fence.getByRole("img", { name: /Step 1 of 2/ });
  expect(decoded(await first.getAttribute("src"))).toContain('[data-step="2"]{display:none}');
  await expect.poll(() => first.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);
  await expect(fence.getByRole("button", { name: "Previous step" })).toBeDisabled();
  await fence.getByRole("button", { name: "Next step" }).click();
  await expect(fence.getByText("The server appears")).toBeVisible();
  const second = fence.getByRole("img", { name: /Step 2 of 2/ });
  expect(decoded(await second.getAttribute("src"))).not.toContain("display:none");
  await fence.getByRole("tab", { name: "Code" }).click();
  await expect(fence).toContainText("data-step");
});

test("a chart draws from its spec, with a tooltip and data table", async ({ page }) => {
  await send(page, "viz: all");
  const chart = page.getByTestId("chart");
  await expect(chart.getByRole("img", { name: "Requests by region" })).toBeVisible();
  await expect(chart).toContainText("Requests by region");
  await expect(chart.getByRole("listitem")).toHaveText(["2025", "2026"]);
  await page.screenshot({ path: "test-results/chat-visuals.png", fullPage: true });

  await chart.getByRole("group").focus();
  await page.keyboard.press("ArrowRight");
  await expect(chart.getByRole("status")).toContainText("160 K");
  await expect(chart.getByRole("status")).toContainText("us");

  await chart.getByRole("tab", { name: "Data" }).click();
  await expect(chart.getByRole("table")).toContainText("110 K");
});

test("a chart that can't be drawn says why and shows its source", async ({ page }) => {
  await send(page, "badchart: nope");
  await expect(page.getByText("type must be bar")).toBeVisible();
  await expect(page.getByTestId("code-fence")).toContainText("pie");
});
