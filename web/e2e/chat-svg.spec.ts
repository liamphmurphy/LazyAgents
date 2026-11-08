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

test("a diagram is saved to the Library only when asked", async ({ page }) => {
  const saves: { message_id: string; svg: string }[] = [];
  await send(page, "svg: a box");
  // After send: the chat mock's catch-all would win over an earlier route.
  await page.route("**/api/v1/library/diagrams", (route) => {
    saves.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ id: "d-1", kind: "diagram", title: "T", filename: "diagram.svg", content_type: "image/svg+xml", size_bytes: 9, created_at: new Date().toISOString() }) });
  });
  const fence = page.getByTestId("svg-fence");
  await expect(fence.getByRole("img", { name: "SVG image" })).toBeVisible();
  expect(saves).toHaveLength(0);

  await fence.getByRole("button", { name: "Save to Library" }).click();
  await expect(fence.getByRole("button", { name: "Saved" })).toBeDisabled();
  expect(saves).toHaveLength(1);
  expect(saves[0].svg).toContain('xmlns="http://www.w3.org/2000/svg"');
  expect(saves[0].message_id).toBeTruthy();
});

test("an svg the browser can't draw falls back to its source", async ({ page }) => {
  await send(page, "badsvg: broken");
  const fence = page.getByTestId("svg-fence");
  await expect(fence.getByRole("alert")).toContainText("couldn't be displayed");
  await expect(fence.getByRole("button", { name: "Save to Library" })).toHaveCount(0);
  await expect(fence).toContainText("<rect>");
});
