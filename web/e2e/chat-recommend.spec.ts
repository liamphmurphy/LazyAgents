import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// A ```recommend fence shows as a preview card; a video only contacts YouTube once played.

test("recommend fences become video, site and compact cards", async ({ page }) => {
  const external: string[] = [];
  await page.route(/youtube|ytimg|trailatlas/, (route) => {
    external.push(route.request().url());
    return route.fulfill({ status: 200, contentType: "text/html", body: "<title>player</title>" });
  });
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("recommend: tonight");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");

  const reply = page.getByTestId("assistant-message");
  const video = reply.locator('[data-kind="video"]');
  await expect(video).toContainText("Why small rooms sound better");
  await expect(video).toContainText("Room Acoustics Lab");
  await expect(video).toContainText("Short and useful.");

  const site = reply.locator('[data-kind="site"]');
  await expect(site).toContainText("A field guide to quiet bike routes");
  await expect(site).toHaveAttribute("href", "https://trailatlas.example/routes");
  await expect(site.locator("img")).toHaveAttribute("src", "/api/v1/link-previews/site/image");

  const compact = reply.locator('[data-kind="link"]');
  await expect(compact).toContainText("notes.example/grinders");
  await expect(compact).toContainText("Best comparison.");

  // The two neighbouring cards sit side by side.
  const [a, b] = [await video.boundingBox(), await site.boundingBox()];
  expect(a!.y).toBe(b!.y);
  expect(external).toEqual([]);

  await video.getByRole("button", { name: /Play/ }).click();
  await expect(video.locator("iframe")).toHaveAttribute("src", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?autoplay=1&rel=0");
  await expect.poll(() => external.length).toBeGreaterThan(0);
  expect(external.every((u) => u.includes("youtube-nocookie.com"))).toBe(true);
});
