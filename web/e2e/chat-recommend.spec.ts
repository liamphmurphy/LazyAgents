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
  await expect(site.getByRole("link")).toHaveAttribute("href", "https://trailatlas.example/routes");
  await expect(site.locator("img")).toHaveAttribute("src", "/api/v1/link-previews/site/image");

  await expect(video.getByRole("button", { name: "Summarize video" })).toBeEnabled();
  await expect(site.getByRole("button", { name: "Summarize page" })).toBeEnabled();

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

async function openReply(page: import("@playwright/test").Page) {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill("recommend: tonight");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page.locator('[data-kind="site"]')).toBeVisible();
  return mock;
}

test("Summarize sends a message that links the recommendation", async ({ page }) => {
  const mock = await openReply(page);
  await page.locator('[data-kind="video"]').getByRole("button", { name: "Summarize video" }).click();
  await expect.poll(() => mock.posted.at(-1)?.body).toEqual({ body: "Summarize this video: [Why small rooms sound better](https://www.youtube.com/watch?v=dQw4w9WgXcQ)" });
  await expect(page.getByTestId("user-message").nth(1)).toContainText("Summarize this video: Why small rooms sound better");
});

test("Save to Library posts the link and the card shows Saved", async ({ page }) => {
  await openReply(page);
  const saves: unknown[] = [];
  await page.route("**/api/v1/library/links", (route) => {
    saves.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ id: "l-1", kind: "link", title: "T", filename: "link", content_type: "image/png", size_bytes: 9, created_at: new Date().toISOString() }) });
  });
  const site = page.locator('[data-kind="site"]');
  await site.getByRole("button", { name: "Save to Library" }).click();
  await expect(site.getByText("Saved")).toBeVisible();
  expect(saves).toHaveLength(1);
  expect(saves[0]).toMatchObject({ url: "https://trailatlas.example/routes" });
});
