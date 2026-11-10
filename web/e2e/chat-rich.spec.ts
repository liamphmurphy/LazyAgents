import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Citations, math, diagrams, video moments and choices in a reply.

async function open(page: Page) {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/youtube-nocookie/, (route) => route.fulfill({ status: 200, contentType: "text/html", body: "ok" }));
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill("rich: everything");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText("A claim");
  return mock;
}

test("citation markers open Sources at their card; unknown numbers stay text", async ({ page }) => {
  await open(page);
  const reply = page.getByTestId("assistant-message");
  await expect(reply.getByTestId("cite")).toHaveCount(2);
  await expect(reply).toContainText("[9]");
  await expect(reply).not.toContainText("Sources:");

  const sources = reply.getByTestId("sources");
  await expect(sources.getByTestId("source")).toHaveCount(0);
  await reply.getByTestId("cite").first().click();
  await expect(sources.locator('[data-cite="1"]')).toContainText("Pricing page");
  await expect(sources.locator('[data-cite="2"]')).toBeVisible();
});

test("math renders with double dollars and leaves prices alone", async ({ page }) => {
  await open(page);
  const reply = page.getByTestId("assistant-message");
  await expect(reply.locator(".katex")).toHaveCount(1);
  await expect(reply).toContainText("it costs $5 or $10.");
});

test("mermaid draws as an image; a broken one falls back to its code", async ({ page }) => {
  await open(page);
  const reply = page.getByTestId("assistant-message");
  const img = reply.getByTestId("mermaid").getByRole("img", { name: "Diagram" });
  await expect(img).toBeVisible({ timeout: 15000 });
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);
  await expect(reply).toContainText("this is not a diagram !!");
});

test("a moment link jumps the chat's player", async ({ page }) => {
  await open(page);
  const reply = page.getByTestId("assistant-message");
  await expect(reply.locator('[data-kind="link"], [data-kind="video"]').first()).toBeVisible();
  await reply.locator('[data-kind="video"]').waitFor();
  await reply.getByTestId("moment").click();
  await expect(reply.locator('[data-kind="video"] iframe')).toHaveAttribute("src", /embed\/dQw4w9WgXcQ\?autoplay=1&rel=0&start=65/);
});

test("a choice sends one message, then records the pick", async ({ page }) => {
  const mock = await open(page);
  const choices = page.getByTestId("assistant-message").first().getByTestId("choices");
  await choices.getByRole("button", { name: /Kokoro/ }).click();
  await expect.poll(() => mock.posted.at(-1)?.body).toEqual({ body: "Going with: Kokoro" });
  await expect(choices.getByRole("button", { name: /Kokoro/ })).toHaveAttribute("aria-pressed", "true");
  await expect(choices.getByRole("button", { name: /Cartesia/ })).toBeDisabled();
});

test("a summary reply can be saved with its link", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill("recommend: tonight");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await page.locator('[data-kind="site"]').getByRole("button", { name: "Summarize page" }).click();
  const summary = page.getByTestId("assistant-message").nth(1);
  const saves: unknown[] = [];
  await page.route("**/api/v1/library/links/summary", (route) => {
    saves.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ id: "l", kind: "link", title: "T", filename: "link", content_type: "image/png", size_bytes: 9, created_at: new Date().toISOString() }) });
  });
  await summary.getByRole("button", { name: "Save summary to Library" }).click();
  await expect(summary.getByText("Summary saved")).toBeVisible();
  expect(saves[0]).toMatchObject({ url: "https://trailatlas.example/routes" });
});
