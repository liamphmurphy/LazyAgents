import { expect, test, type Page } from "@playwright/test";
import { DECK, deckWith, installDeck as install } from "./mock-deck-api";

// LAZYAGENTS-160: the Slides editor on the shared EditorLayout. Thumbnails on
// the left, Chat and Review in the dock, Present and Share ▾ in the header.

const dock = (page: Page) => page.getByRole("complementary", { name: "Panel", exact: true });
const tabs = (page: Page) => dock(page).getByRole("tablist", { name: "Panel tabs" });
const rail = (page: Page) => page.getByRole("navigation", { name: "Panel rail" });
const mod = process.platform === "darwin" ? "Meta" : "Control";

test.use({ viewport: { width: 1440, height: 900 } });

test("the deck renders in the editor layout and its dock collapses and reopens", async ({ page }) => {
  await install(page, [{ seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" }]);

  // Thumbnails on the left, chat in the dock, no Hide chat.
  await expect(page.getByRole("complementary", { name: "Structure" }).getByRole("navigation", { name: "Slides" })).toBeVisible();
  await expect(tabs(page).getByRole("tab", { name: /Chat/ })).toHaveAttribute("aria-selected", "true");
  await expect(tabs(page).getByRole("tab", { name: /Review/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Hide chat/ })).toHaveCount(0);
  await expect(page.getByTestId("deck-save-state")).toHaveText(/Saved · v1/);

  await dock(page).getByRole("button", { name: "Collapse panel" }).click();
  await expect(dock(page)).toHaveCount(0);
  await expect(rail(page)).toBeVisible();
  await rail(page).getByRole("button", { name: "Expand panel" }).click();
  await expect(tabs(page).getByRole("tab", { name: /Chat/ })).toHaveAttribute("aria-selected", "true");

  await page.locator("body").click();
  await page.keyboard.press(`${mod}+Backslash`);
  await expect(rail(page)).toBeVisible();
  await page.keyboard.press(`${mod}+j`);
  await expect(dock(page)).toBeVisible();
  await expect(dock(page).getByRole("combobox", { name: "Message" })).toBeFocused();

  // The collapsed state is remembered across a reload.
  await tabs(page).getByRole("tab", { name: /Chat/ }).click();
  await expect(rail(page)).toBeVisible();
  await page.reload();
  await expect(page.getByTestId("deck-save-state")).toBeVisible();
  await expect(rail(page)).toBeVisible();
});

test("Share holds both exports and Present stays in the header", async ({ page }) => {
  await install(page, [{ seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" }]);

  for (const [item, file] of [[/PDF/, "Layout.pdf"], [/PowerPoint/, "Layout.pptx"]] as const) {
    await page.getByRole("button", { name: "Share" }).click();
    const download = page.waitForEvent("download");
    await page.getByRole("menuitem", { name: item }).click();
    expect((await download).suggestedFilename()).toBe(file);
  }

  await page.getByRole("button", { name: /Present/ }).click();
  await expect(page).toHaveURL(new RegExp(`/present/slides/${DECK}\\?slide=1`));
});

test("a persona's suggestions open in the Review tab and can be accepted or rejected", async ({ page }) => {
  const s = await install(page, [
    { seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" },
    { seq: 2, body: JSON.stringify(deckWith("Q4 business review")), author: "agent" },
  ]);

  await expect(tabs(page).getByRole("tab", { name: /Review/ })).toHaveAttribute("aria-selected", "true");
  const review = page.getByTestId("deck-review");
  await expect(review.getByText(/suggested 1 change/)).toBeVisible();
  await expect(page.getByText("Editing is paused until you decide them in Review.")).toBeVisible();

  // Rejecting records a version that keeps the person's title.
  await review.getByRole("button", { name: "Reject", exact: true }).click();
  await expect.poll(() => s.reviewed).toEqual([3]);
  expect(JSON.parse(s.versions[2].body).slides[0].elements[0].paragraphs[0].runs[0].text).toBe("Quarterly review");
});

test("accepting every suggestion marks the review done without a new version", async ({ page }) => {
  const s = await install(page, [
    { seq: 1, body: JSON.stringify(deckWith("Quarterly review")), author: "user" },
    { seq: 2, body: JSON.stringify(deckWith("Q4 business review")), author: "agent" },
  ]);
  const review = page.getByTestId("deck-review");
  await review.getByRole("button", { name: "Accept all" }).click();
  await expect.poll(() => s.reviewed).toEqual([2]);
  expect(s.versions).toHaveLength(2);
  // The review ends: the dock is back on Chat and the slide is editable.
  await expect(tabs(page).getByRole("tab", { name: /Review/ })).toHaveCount(0);
  await expect(tabs(page).getByRole("tab", { name: /Chat/ })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("toolbar", { name: "Slide formatting" }).getByRole("button", { name: "T Text" })).toBeEnabled();
});
