import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// Code a coding persona wrote: the Changes block under a reply and highlighted fences in the markdown.

test("changes block is collapsed, expands to file diffs; long fences collapse", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill("code: add the thing");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");

  const block = page.getByTestId("code-changes");
  const toggle = block.getByRole("button", { name: /Changed 3 files/ });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(toggle).toContainText("+5");
  await expect(toggle).toContainText("−1");
  await expect(page.getByTestId("code-change-file")).toHaveCount(0);

  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  const files = page.getByTestId("code-change-file");
  await expect(files).toHaveCount(3);
  await expect(files.nth(0)).toContainText("src/app.ts");
  await expect(files.nth(0)).toContainText("+2");
  await expect(files.nth(1)).toContainText("cmd/main.go");
  // Three files or fewer start open.
  await expect(files.nth(0).locator('[data-diff-row="add"]')).toHaveCount(3);
  await expect(files.nth(0).locator('[data-diff-row="hunk"]')).toHaveCount(2);
  await expect(files.nth(2)).toContainText("Binary file changed");

  // A file collapses on its own.
  await files.nth(1).getByRole("button", { name: /^Added cmd\/main\.go/ }).click();
  await expect(files.nth(1).locator('[data-diff-row="add"]')).toHaveCount(0);

  // The fence over 20 lines is collapsed behind a show-more button.
  const fence = page.getByTestId("code-fence");
  await expect(fence).toContainText("ts");
  // The block's <code> is not an inline chip: no border, background or padding
  // (those drew as empty pills around the first and last lines).
  const code = fence.locator("pre > code");
  await expect(code).toHaveCSS("border-top-width", "0px");
  await expect(code).toHaveCSS("padding-left", "0px");
  await expect(code).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(code).toHaveCSS("display", "block");
  await fence.screenshot({ path: "test-results/code-fence.png" });
  await expect(fence.getByRole("button", { name: "Show 7 more lines" })).toBeVisible();
  await expect(fence).not.toContainText("const v24");
  await fence.getByRole("button", { name: /more lines/ }).click();
  await expect(fence).toContainText("const v24");
});
