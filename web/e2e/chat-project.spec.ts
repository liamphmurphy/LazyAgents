import { expect, test } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

const ATLAS = persona(1, { name: "Atlas", tone: "friendly", color: "#4fd1c5", is_default: true });

test("chat lives above projects: no switcher, and a project can be attached and removed", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [ATLAS] });
  await page.goto("/chat");
  await expect(page.getByRole("button", { name: /^Switch project/ })).toHaveCount(0);

  await page.getByRole("textbox", { name: "Message" }).fill("hello");
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page).toHaveURL(/\/chat\/[0-9a-f-]+$/);
  await expect(page.getByTestId("assistant-message").first()).toBeVisible();
  expect(mock.created.at(-1)?.project).toBeUndefined();

  const picker = page.getByTestId("chat-project");
  await expect(picker).toHaveText("Add project");
  await picker.click();
  await page.getByRole("menuitemradio", { name: "Web app" }).click();
  await expect(picker).toHaveText("Web app");

  await picker.click();
  await page.getByRole("menuitemradio", { name: "No project" }).click();
  await expect(picker).toHaveText("Add project");
});
