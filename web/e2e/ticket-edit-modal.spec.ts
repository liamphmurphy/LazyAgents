import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

const screen = (page: Page) => page.locator('[data-testid="board-screen"]:visible');
const card = (page: Page, key = "WEB-1") => screen(page).getByRole("article", { name: `Ticket ${key}`, exact: true });
const modal = (page: Page) => page.getByRole("dialog");

async function open(page: Page) {
  await card(page).getByRole("button", { name: /^Edit WEB-1/ }).click();
  await expect(modal(page).getByLabel("Title")).toHaveValue("Build ticket modal");
}

test("edits every field through one PATCH carrying revision", async ({ page }) => {
  const api = await installBoardApi(page);
  Object.assign(api.tickets[0], {
    estimated_by: { type: "agent", id: "agent-builder", name: "builder", color_index: 0 },
    estimated_at: new Date(Date.now() - 2 * 3600_000).toISOString(),
    estimate_rationale: "Modal plus conflict flow", estimate: "L · ~6h",
  });
  await page.goto("/board?project=WEB");
  await open(page);
  const m = modal(page);
  await expect(m).toContainText("Estimated by");
  await expect(m).toContainText("@builder");
  await expect(m).toContainText("2h ago");
  await expect(m).toContainText("“Modal plus conflict flow”");
  await expect(m).toContainText("Legacy note: L · ~6h");
  await expect(m.getByRole("radio", { name: "3", exact: true })).toHaveAttribute("aria-checked", "true");

  await m.getByLabel("Title").fill("Build the edit modal");
  await m.getByLabel("Description (markdown)").fill("New body");
  await m.getByRole("button", { name: /^Todo/ }).click();
  await page.getByRole("menuitem", { name: "In Review" }).click();
  await m.getByRole("button", { name: /^High/ }).click();
  await page.getByRole("menuitem", { name: "Urgent" }).click();
  await m.getByRole("radio", { name: "8", exact: true }).click();
  await m.getByRole("button", { name: /^Kanban board/ }).click();
  await page.getByRole("menuitem", { name: "Other epic" }).click();
  await expect(m).toContainText("6 unsaved changes");
  await m.getByRole("button", { name: "Save", exact: true }).click();

  await expect(m).toHaveCount(0);
  expect(api.patches).toEqual([{ key: "WEB-1", body: {
    revision: 1, title: "Build the edit modal", description_md: "New body", status: "in_review", priority: "urgent", estimate_points: 8, epic_id: "epic-2",
  } }]);
  expect(api.unhandled).toEqual([]);
});

test("a 409 shows the latest ticket and lets the person reapply their edits", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB");
  await open(page);
  const m = modal(page);
  await m.getByLabel("Title").fill("Mine");
  await m.getByRole("radio", { name: "5", exact: true }).click();
  api.editTicket("WEB-1", { priority: "urgent", title: "Theirs" });
  await m.getByRole("button", { name: "Save", exact: true }).click();

  await expect(m).toContainText("Someone else changed this ticket (now rev 2)");
  await expect(m).toContainText("2 unsaved edits (title, estimate)");
  await expect(m.getByLabel("Title")).toHaveValue("Theirs");
  await expect(m.getByRole("button", { name: /^Urgent/ })).toBeVisible();
  await expect(m.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
  await m.getByRole("button", { name: "Reapply mine" }).click();
  await expect(m.getByLabel("Title")).toHaveValue("Mine");
  await expect(m.getByRole("button", { name: /^Urgent/ })).toBeVisible();
  await m.getByRole("button", { name: "Save", exact: true }).click();

  await expect(m).toHaveCount(0);
  expect(api.patches.map((p) => p.body)).toEqual([
    { revision: 1, title: "Mine", estimate_points: 5 },
    { revision: 2, title: "Mine", estimate_points: 5 },
  ]);
  expect(api.tickets[0]).toMatchObject({ title: "Mine", priority: "urgent", estimate_points: 5, revision: 3 });
});

test("discarding after a 409 keeps the latest version", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB");
  await open(page);
  const m = modal(page);
  await m.getByLabel("Title").fill("Mine");
  api.editTicket("WEB-1", { title: "Theirs" });
  await m.getByRole("button", { name: "Save", exact: true }).click();
  await m.getByRole("button", { name: "Discard mine" }).click();
  await expect(m.getByLabel("Title")).toHaveValue("Theirs");
  await expect(m).not.toContainText("unsaved");
});

test("a 422 is shown on the field it names", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB");
  await open(page);
  const m = modal(page);
  await m.getByRole("radio", { name: "13", exact: true }).click();
  api.rejectNext(422, "estimate_points must be on the scale", "estimate_points");
  await m.getByRole("button", { name: "Save", exact: true }).click();
  const err = m.getByRole("alert").filter({ hasText: "estimate_points must be on the scale" });
  await expect(err).toBeVisible();
  await expect(m.getByRole("radiogroup", { name: "Estimate (points)" })).toContainText("13");
  await expect(m).not.toContainText("Couldn't save");
  await m.getByRole("radio", { name: "8", exact: true }).click();
  await expect(err).toHaveCount(0);
});

test("criteria are added, edited, ticked and removed without a ticket PATCH", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB");
  await open(page);
  const m = modal(page);
  const section = m.getByRole("region", { name: "Acceptance criteria" });
  await expect(section).toContainText("1 of 1");

  await section.getByLabel("Add criterion").fill("Second check");
  await section.getByLabel("Add criterion").press("Enter");
  await expect(section).toContainText("2. Second check");
  await section.getByLabel("Criterion 2 met").click();
  await expect(section.getByLabel("Criterion 2 met")).toBeChecked();
  await expect(section).toContainText("2 of 2");
  await section.getByRole("listitem").filter({ hasText: "Second check" }).hover();
  await section.getByRole("button", { name: "Edit criterion 2" }).click();
  await section.getByLabel("Edit criterion 2").fill("Should not save");
  await section.getByLabel("Edit criterion 2").press("Escape");
  await expect(m).toBeVisible();
  await expect(section).toContainText("2. Second check");
  await expect(section).not.toContainText("Should not save");
  await section.getByRole("button", { name: "Edit criterion 2" }).click();
  await section.getByLabel("Edit criterion 2").fill("Second check, edited");
  await section.getByLabel("Edit criterion 2").press("Enter");
  await expect(section).toContainText("2. Second check, edited");
  await section.getByRole("listitem").filter({ hasText: "Second check, edited" }).hover();
  await section.getByRole("button", { name: "Remove criterion 2" }).click();
  await expect(section).not.toContainText("Second check");

  expect(api.criteriaRequests.map((r) => r.split(" ")[0])).toEqual(["POST", "PATCH", "PATCH", "DELETE"]);
  expect(api.patches).toEqual([]);
  expect(api.tickets[0].revision).toBe(1);
  // The modal never conflicts with its own criteria edits.
  await m.getByLabel("Title").fill("After criteria");
  await m.getByRole("button", { name: "Save", exact: true }).click();
  await expect(m).toHaveCount(0);
  expect(api.patches.map((p) => p.body)).toEqual([{ revision: 1, title: "After criteria" }]);
});
