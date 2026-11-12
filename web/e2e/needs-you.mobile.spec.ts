import { expect, test } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

// LAZYAGENTS-162: on a phone the Needs you tab opens the same tray above the tab bar.
test("the Needs you tab opens the tray", async ({ page }) => {
  const api = await installBoardApi(page);
  api.counts.needs_you = 1;
  await page.route("**/api/v1/inbox", (route) => route.fulfill({ json: { items: [{
    id: "review:1", kind: "review", needs_you: true, title: "Ledger partitions", summary: "Ready for your review", href: "/tickets/WEB-1",
    project: "WEB", ticket_key: "WEB-1", created_at: new Date().toISOString(), resolved: false, read: false,
  }] } }));
  await page.goto("/tickets");
  const bar = page.getByRole("navigation", { name: "Primary" });
  await bar.getByRole("button", { name: "Needs you, 1 waiting" }).click();
  const tray = page.getByRole("dialog", { name: "Needs you" });
  await expect(tray.getByRole("link", { name: "Open review" })).toBeVisible();
  const box = await tray.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  await tray.getByRole("link", { name: "Open all →" }).click();
  await expect(page).toHaveURL(/\/runs(\/[^#]+)?#needs-you$/);
});
