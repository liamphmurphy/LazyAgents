import { expect, test } from "@playwright/test";
import { deckWith, installDeck } from "./mock-deck-api";

// LAZYAGENTS-160 on a phone: the thumbnails stay as a strip under the slide,
// and the dock opens as a sheet over it.

test("thumbnails show under the slide and Chat opens as a sheet", async ({ page }) => {
  await installDeck(page, [{ seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" }]);
  await expect(page.getByRole("navigation", { name: "Slides" }).getByRole("button", { name: /Slide 2/ })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Structure" })).toHaveCount(0);

  await page.getByRole("navigation", { name: "Panel rail" }).getByRole("button", { name: "Open Chat" }).click();
  await expect(page.getByRole("dialog").getByRole("combobox", { name: "Message" })).toBeVisible();
});
