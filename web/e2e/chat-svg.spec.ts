import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// An ```svg-image fence in a reply shows as a static image with a code view.

async function send(page: import("@playwright/test").Page, text: string) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill(text);
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
}

test("svg fences render as an image, without running scripts", async ({ page }) => {
  await send(page, "svg: a box");
  const fence = page.getByTestId("svg-fence");
  const img = fence.getByRole("img", { name: "SVG image" });
  await expect(img).toBeVisible();
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBe(40);
  expect(await page.title()).not.toBe("ran");
  await expect(fence.getByRole("link", { name: "Download SVG" })).toHaveAttribute("download", "image.svg");

  await fence.getByRole("tab", { name: "Code" }).click();
  await expect(img).toHaveCount(0);
  await expect(fence).toContainText("<rect");
});

test("an svg the browser can't draw falls back to its source", async ({ page }) => {
  await send(page, "badsvg: broken");
  const fence = page.getByTestId("svg-fence");
  await expect(fence.getByRole("alert")).toContainText("couldn't be displayed");
  await expect(fence).toContainText("<rect>");
});
