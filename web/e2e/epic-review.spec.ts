import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

const summaryOf = (page: Page) => page.locator('[data-testid="board-screen"]:visible').getByRole("region", { name: "Epic summary" });

test("an epic can add its final review ticket early", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB&epic=epic-1");
  const review = summaryOf(page).getByRole("region", { name: "Final review" });
  await expect(review).toContainText("Created automatically when the last ticket finishes.");
  await review.getByRole("button", { name: "Add final review now" }).click();

  const link = review.getByRole("link", { name: "WEB-7" });
  await expect(link).toHaveAttribute("href", "/tickets/WEB-7");
  await expect(review).toContainText("Final review: Kanban board");
  await expect(review).toContainText("Runs after the epic's other tickets finish");
  await expect(review.getByRole("button", { name: "Add final review now" })).toHaveCount(0);
  expect(api.epicWrites.map((w) => [w.method, w.path])).toEqual([["POST", "/epics/epic-1/review"]]);
  expect(api.unhandled).toEqual([]);
});

test("a closed epic without a review shows no final review section", async ({ page }) => {
  const api = await installBoardApi(page);
  api.epics[0].status = "done";
  await page.goto("/board?project=WEB&epic=epic-1");
  await expect(summaryOf(page)).toContainText("Status: Done");
  await expect(summaryOf(page).getByRole("region", { name: "Final review" })).toHaveCount(0);
});
