import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

const refused = { status: 401, json: { code: "unauthenticated", message: "Sign in with the link from `make signin`." } };

test("a browser the API refuses lands on the sign-in page", async ({ page }) => {
  await installChatApi(page);
  await page.route("**/api/v1/**", (route) => route.fulfill(refused));
  await page.goto("/chat");
  await expect(page).toHaveURL(/\/sign-in$/);
  await expect(page.getByRole("heading", { name: "Sign in to Vesper" })).toBeVisible();
  await expect(page.getByText("make signin")).toBeVisible();
});

test("a used or expired link explains itself", async ({ page }) => {
  await page.goto("/sign-in?error=invalid");
  await expect(page.getByRole("heading", { name: "That sign-in link didn't work" })).toBeVisible();
  await expect(page.getByText(/already used or has expired/)).toBeVisible();
});
