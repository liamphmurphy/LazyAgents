import { expect, test, type Page } from "@playwright/test";
import { installChatApi, openEmptyChat } from "./mock-chat-api";

// VESPER-12: at 390×844 the composer is the message field over one row (+, model chip, Send),
// and + opens a bottom sheet listing every option with its value.
const sheet = (page: Page) => page.getByRole("dialog", { name: "Options" });
const openSheet = async (page: Page) => {
  await page.getByRole("button", { name: "Composer options" }).click();
  await expect(sheet(page)).toBeVisible();
};

test("one row under the message field: +, the model chip and Send", async ({ page }) => {
  await installChatApi(page);
  await openEmptyChat(page);
  const row = page.getByTestId("composer-compact-row");
  await expect(row.getByRole("button")).toHaveCount(3);
  await expect(row.getByRole("button").first()).toHaveAccessibleName("Composer options");
  await expect(row.getByRole("button", { name: /^Model and effort/ })).toBeVisible();
  await expect(row.getByRole("button", { name: "Send" })).toBeVisible();
  // The row is a single line: every button shares one vertical band.
  const boxes = await row.getByRole("button").evaluateAll((els) => els.map((el) => el.getBoundingClientRect()));
  for (const b of boxes) expect(b.top).toBeLessThan(boxes[0].bottom);
  // The desktop option row is gone.
  await expect(page.getByRole("button", { name: "Research" })).toHaveCount(0);
});

test("the sheet lists every option with its value, and closes on Escape, scrim tap and swipe down", async ({ page }) => {
  await installChatApi(page);
  await openEmptyChat(page);
  await openSheet(page);
  const rows = sheet(page).getByTestId("sheet-option");
  const ids = await rows.evaluateAll((els) => els.map((el) => el.getAttribute("data-option")));
  expect(ids).toEqual(expect.arrayContaining(["context", "attach", "research", "model", "project"]));
  for (const row of await rows.all()) {
    expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(56);
    await expect(row.getByTestId("sheet-option-value")).not.toBeEmpty();
  }
  await expect(sheet(page).locator('[data-option="research"]').getByTestId("sheet-option-value")).toHaveText("Off");

  // A tap on the row flips the same toggle the desktop row has.
  await sheet(page).locator('[data-option="research"]').click();
  await expect(sheet(page).locator('[data-option="research"]').getByTestId("sheet-option-value")).toHaveText("On");

  // Its bottom edge sits on the viewport's bottom (inside the safe-area padding).
  const box = (await sheet(page).boundingBox())!;
  expect(Math.round(box.y + box.height)).toBe(844);
  await expect(sheet(page)).toHaveClass(/\bpb-safe\b/);

  await page.keyboard.press("Escape");
  await expect(sheet(page)).toHaveCount(0);

  await openSheet(page);
  await page.mouse.click(195, 40);
  await expect(sheet(page)).toHaveCount(0);

  await openSheet(page);
  const grab = (await sheet(page).getByTestId("sheet-grabber").boundingBox())!;
  await page.mouse.move(grab.x + grab.width / 2, grab.y + grab.height / 2);
  await page.mouse.down();
  await page.mouse.move(grab.x + grab.width / 2, grab.y + 160, { steps: 6 });
  await page.mouse.up();
  await expect(sheet(page)).toHaveCount(0);
});

test("attaching context through the sheet, then sending it", async ({ page }) => {
  const mock = await installChatApi(page);
  await openEmptyChat(page);
  await page.getByRole("combobox", { name: "Message" }).fill("see this");
  await openSheet(page);
  // The row opens the same picker as the desktop Add context button.
  await sheet(page).locator('[data-option="context"]').click();
  await page.getByRole("combobox", { name: "Search pages, artifacts and tickets" }).fill("Q3");
  await page.getByRole("option", { name: /Q3 report/ }).click();
  await expect(sheet(page)).toHaveCount(0);
  await expect(page.getByTestId("context-chip")).toHaveCount(1);

  await openSheet(page);
  await expect(sheet(page).locator('[data-option="context"]').getByTestId("sheet-option-value")).toHaveText("1 added");
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByTestId("assistant-message").first()).toContainText("Echo: see this");
  expect(mock.posted.at(-1)?.body.context_refs).toHaveLength(1);
});
