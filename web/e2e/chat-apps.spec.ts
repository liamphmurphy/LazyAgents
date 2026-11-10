import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// An ```app fence in a reply runs live in a sandboxed frame the person can use.

test("app fences run in a sandboxed, offline frame with a code view", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill("app: a counter");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");

  const fence = page.getByTestId("app-fence");
  const iframe = fence.locator("iframe");
  await expect(iframe).toHaveAttribute("sandbox", "allow-scripts");
  const app = page.frameLocator('[data-testid="app-fence"] iframe');
  await expect(app.getByRole("button", { name: "Count: 0" })).toBeVisible();
  const button = app.locator("#b");
  await button.click();
  await button.click();
  await expect(app.getByRole("button", { name: "Count: 2" })).toBeVisible();
  // The CSP keeps the app off the network.
  await expect(app.locator("#net")).toHaveText("blocked");

  // Reload restarts it.
  await fence.getByRole("button", { name: "Reload app" }).click();
  await expect(app.getByRole("button", { name: "Count: 0" })).toBeVisible();

  // Code shows the source as a highlighted fence.
  await fence.getByRole("tab", { name: "Code" }).click();
  await expect(iframe).toHaveCount(0);
  await expect(fence.getByTestId("code-fence")).toContainText("Count: 0");
  await fence.getByRole("tab", { name: "App" }).click();
  await expect(app.getByRole("button", { name: "Count: 0" })).toBeVisible();
});
