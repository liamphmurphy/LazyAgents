import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// A reply's Sources start as one row and open into cards, Vesper and web together.

async function send(page: import("@playwright/test").Page, text: string) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill(text);
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
}

test("sources start minimized, expand to cards, and the URL list leaves the message", async ({ page }) => {
  await send(page, "web: tts");
  const reply = page.getByTestId("assistant-message");
  await expect(reply).toContainText("The answer.");
  await expect(reply).not.toContainText("https://example.com/pricing");

  const sources = reply.getByTestId("sources");
  const row = sources.getByRole("button", { name: /3 sources/ });
  await expect(row).toContainText("1 in Vesper · 2 from the web");
  await expect(sources.getByTestId("source")).toHaveCount(0);

  await row.click();
  await expect(sources.getByTestId("source")).toHaveCount(3);
  const page1 = sources.getByRole("link", { name: /Pricing page/ });
  await expect(page1).toHaveAttribute("href", "https://example.com/pricing");
  await expect(page1).toContainText("example.com");

  await expect(sources.getByTestId("source-more")).toHaveCount(0);
  await sources.getByRole("button", { name: /1 more search result, not used/ }).click();
  await expect(sources.getByTestId("source-more")).toContainText("Unused forum");

  await sources.getByRole("button", { name: "1 search" }).click();
  await expect(sources.getByTestId("source-query")).toHaveText("tts pricing");

  await sources.getByRole("button", { name: /3 sources/ }).click();
  await expect(sources.getByTestId("source")).toHaveCount(0);
});

test("a search the reply didn't use says so and still opens", async ({ page }) => {
  await send(page, "webnone: tts");
  const sources = page.getByTestId("assistant-message").getByTestId("sources");
  const row = sources.getByRole("button", { name: /Searched the web/ });
  await expect(row).toContainText("none used");
  await row.click();
  await expect(sources.getByTestId("source")).toHaveCount(0);
  await sources.getByRole("button", { name: /3 more search results/ }).click();
  await expect(sources.getByTestId("source-more")).toHaveCount(3);
});
