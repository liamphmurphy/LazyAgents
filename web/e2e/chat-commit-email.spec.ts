import { expect, test } from "@playwright/test";
import { installChatApi, persona } from "./mock-chat-api";

// With several email addresses, a coding chat asks which one its commits are
// authored as: before the first message, and again whenever it's changed.

const repos = ["https://github.com/acme/app.git"];
const coder = () => persona(1, { name: "Dev", is_default: true, capabilities: { code: true, images: false }, repos });
const picker = (page: import("@playwright/test").Page) => page.getByTestId("commit-email-picker");

test("a new coding chat waits for an address, starts with it, and can switch from the header", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [coder()], emails: ["liam@example.com", "liam@phmurphy.com"] });
  await page.goto("/chat?project=WEB");
  await expect(picker(page)).toContainText("Which email should commits use?");
  const options = picker(page).getByRole("radio");
  await expect(options).toHaveCount(2);
  await expect(options.first()).toContainText("Author: Liam <liam@example.com>");
  await expect(options.first()).toContainText("primary");
  await expect(options.first()).toHaveAttribute("aria-checked", "false");

  await page.getByRole("combobox", { name: "Message" }).fill("fix the build");
  const send = page.getByRole("button", { name: "Send" });
  await expect(send).toBeDisabled();
  await picker(page).getByRole("radio", { name: /liam@phmurphy\.com/ }).click();
  await expect(picker(page).getByRole("radio", { name: /liam@phmurphy\.com/ })).toHaveAttribute("aria-checked", "true");
  await send.click();
  await expect(page).toHaveURL(/\/chat\/[^/]+/);
  expect(mock.created[0]).toMatchObject({ repo_url: repos[0], commit_email: "liam@phmurphy.com" });

  // Picked: the picker folds into a header chip that reopens it.
  await expect(picker(page)).toHaveCount(0);
  // Picked: the title shows it as an icon, and the title menu reopens the picker.
  const chip = page.getByTestId("commit-email-chip");
  await expect(page.getByTestId("chat-title-status").getByRole("img", { name: "Commits are authored as liam@phmurphy.com" })).toBeVisible();
  await page.getByTestId("chat-title-menu").click();
  await expect(chip).toContainText("liam@phmurphy.com");
  await chip.click();
  await picker(page).getByRole("radio", { name: /liam@example\.com/ }).click();
  await expect(picker(page)).toHaveCount(0);
  await page.getByTestId("chat-title-menu").click();
  await expect(chip).toContainText("liam@example.com");
  expect(mock.commitEmailRequests).toEqual(["liam@example.com"]);
  expect(mock.unhandled).toEqual([]);
});

test("an existing coding chat with nothing picked asks above the composer without blocking it", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [coder()], emails: ["liam@example.com", "liam@phmurphy.com"] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("radio", { name: /liam@example\.com/ }).click();
  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[^/]+/);
  // The address leaves the chat (as when it's removed from the profile): it asks again.
  delete mock.chats[0].commit_email;
  await page.reload();
  await expect(picker(page)).toContainText("It applies from the next reply.");
  await expect(page.getByTestId("commit-email-chip")).toHaveCount(0);
  const composer = page.getByRole("combobox", { name: "Message" });
  await composer.fill("next");
  await expect(page.getByRole("button", { name: /Send|Interrupt/ })).toBeEnabled();
  await picker(page).getByRole("radio", { name: /liam@phmurphy\.com/ }).click();
  await page.getByTestId("chat-title-menu").click();
  await expect(page.getByTestId("commit-email-chip")).toContainText("liam@phmurphy.com");
});

test("with one address there's nothing to ask", async ({ page }) => {
  const mock = await installChatApi(page, { personas: [coder()] });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill("fix the build");
  await expect(picker(page)).toHaveCount(0);
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page).toHaveURL(/\/chat\/[^/]+/);
  await expect(picker(page)).toHaveCount(0);
  await expect(page.getByTestId("commit-email-chip")).toHaveCount(0);
  expect(mock.created[0]).not.toHaveProperty("commit_email");
});
