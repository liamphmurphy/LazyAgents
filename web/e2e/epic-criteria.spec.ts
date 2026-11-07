import { expect, test, type Locator, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

const board = (page: Page) => page.locator('[data-testid="board-screen"]:visible').getByRole("region", { name: "Epic summary" });
const editor = (scope: Locator) => scope.getByRole("region", { name: "Final acceptance criteria" });
const rows = (scope: Locator) => editor(scope).getByRole("listitem");

test("criteria can be added, edited, reordered, checked and deleted from the board epic header", async ({ page }, testInfo) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB&epic=epic-1");
  const summary = board(page);
  await expect(summary).toContainText("5 / 10 pts done");
  await expect(rows(summary)).toHaveText([/1\. Board shows all statuses/, /2\. Cards can move/]);

  const draft = editor(summary).getByLabel("New acceptance criterion");
  await draft.fill("Epic can close");
  await editor(summary).getByRole("button", { name: "Add criterion" }).click();
  await expect(rows(summary)).toHaveCount(3);
  await expect(rows(summary).nth(2)).toContainText("3. Epic can close");
  await expect(draft).toHaveValue("");
  await expect(summary).toContainText("1 / 3 met");

  await editor(summary).getByRole("button", { name: "Edit criterion 1" }).click();
  await editor(summary).getByLabel("Criterion 1 text").fill("Discarded");
  await editor(summary).getByRole("button", { name: "Cancel" }).click();
  await expect(rows(summary).nth(0)).toContainText("1. Board shows all statuses");

  await editor(summary).getByRole("button", { name: "Edit criterion 3" }).click();
  await editor(summary).getByLabel("Criterion 3 text").fill("Epic closes cleanly");
  await editor(summary).getByRole("button", { name: "Save" }).click();
  await expect(rows(summary).nth(2)).toContainText("3. Epic closes cleanly");

  await expect(editor(summary).getByRole("button", { name: "Move criterion 1 up" })).toBeDisabled();
  await expect(editor(summary).getByRole("button", { name: "Move criterion 3 down" })).toBeDisabled();
  await editor(summary).getByRole("button", { name: "Move criterion 3 up" }).click();
  await expect(rows(summary)).toHaveText([/1\. Board shows all statuses/, /2\. Epic closes cleanly/, /3\. Cards can move/]);

  await editor(summary).getByLabel("Criterion 2 met").check();
  await expect(summary).toContainText("2 / 3 met");
  await expect(editor(summary).getByLabel("Criterion 2 met")).toBeChecked();

  await editor(summary).getByRole("button", { name: "Delete criterion 1" }).click();
  await expect(rows(summary)).toHaveText([/1\. Epic closes cleanly/, /2\. Cards can move/]);

  expect(api.epicWrites.map((w) => [w.method, w.path, w.body])).toEqual([
    ["POST", "/epics/epic-1/criteria", { text: "Epic can close" }],
    ["PATCH", "/epic-criteria/criterion-3", { text: "Epic closes cleanly" }],
    ["PATCH", "/epic-criteria/criterion-3", { position: 2 }],
    ["PATCH", "/epic-criteria/criterion-3", { done: true }],
    ["DELETE", "/epic-criteria/criterion-1", undefined],
  ]);

  // Pending rows lock actions; failures show inline and keep the draft.
  const release = api.holdNextPatch();
  await editor(summary).getByLabel("Criterion 2 met").check();
  await expect(rows(summary).nth(1)).toHaveAttribute("aria-busy", "true");
  await expect(editor(summary).getByRole("button", { name: "Delete criterion 1" })).toBeDisabled();
  release();
  await expect(rows(summary).nth(1)).toHaveAttribute("aria-busy", "false");
  api.failCriteria("text: must be at most 1000 characters");
  await draft.fill("Too long");
  await editor(summary).getByRole("button", { name: "Add criterion" }).click();
  await expect(editor(summary).getByRole("alert")).toHaveText("Couldn't add criterion: text: must be at most 1000 characters");
  await expect(draft).toHaveValue("Too long");
  api.failCriteria(undefined);

  // Another tab's edit arrives via SSE epic.updated.
  api.criteria.find((c) => c.id === "criterion-2")!.text = "Cards move between columns";
  await api.emit("epic.updated");
  await expect(rows(summary).nth(1)).toContainText("2. Cards move between columns");

  await page.setViewportSize({ width: 390, height: 900 });
  await page.screenshot({ path: testInfo.outputPath("criteria-narrow.png"), fullPage: true });
  expect(api.unhandled).toEqual([]);
});

test("marking an epic done while blocked shows the server's missing items and keeps the status", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/board?project=WEB&epic=epic-1");
  const summary = board(page);
  const status = summary.getByLabel("Epic status");
  await expect(status).toHaveValue("open");
  await status.selectOption("done");
  const blocked = summary.getByRole("alert", { name: "Epic cannot be marked done" });
  await expect(blocked).toContainText("Epic remains Open");
  await expect(blocked).toContainText("epic has unchecked criteria [2] and open tickets [WEB-1 WEB-2 WEB-5]");
  await expect(blocked.getByRole("list", { name: "Unchecked criteria" })).toHaveText("2. Cards can move");
  await expect(blocked.getByRole("list", { name: "Open tickets" }).getByRole("link")).toHaveText(["WEB-1 ↗", "WEB-2 ↗", "WEB-5 ↗"]);
  await expect(blocked.getByRole("link", { name: "WEB-1 ↗" })).toHaveAttribute("href", "/tickets/WEB-1");
  await expect(status).toHaveValue("open");
  expect(api.epics[0].status).toBe("open");

  // Resolve everything elsewhere; the gate now passes.
  api.criteria.forEach((c) => { c.done = true; });
  for (const key of ["WEB-1", "WEB-2", "WEB-5"]) api.editTicket(key, { status: "done" });
  await api.emit("epic.updated");
  await expect(summary).toContainText("Ready to mark done");
  await status.selectOption("done");
  await expect(status).toHaveValue("done");
  await expect(blocked).toHaveCount(0);
  await expect(editor(summary)).toContainText("Reopen the epic to edit its criteria.");
  await expect(editor(summary).getByRole("button", { name: "Edit criterion 1" })).toBeDisabled();
  expect(api.epicWrites.filter((w) => w.method === "PATCH").map((w) => w.body)).toEqual([{ status: "done" }, { status: "done" }]);
  expect(api.unhandled).toEqual([]);
});

test("an epic without criteria prompts for them before it can be closed", async ({ page }) => {
  const api = await installBoardApi(page);
  api.criteria.splice(0);
  await page.goto("/board?project=WEB&epic=epic-1");
  const summary = board(page);
  await expect(editor(summary)).toContainText("Add final acceptance criteria before this epic can be marked done.");
  await expect(summary.getByRole("option", { name: "Done" })).toHaveAttribute("disabled");
  await editor(summary).getByLabel("New acceptance criterion").fill("Board ships");
  await editor(summary).getByRole("button", { name: "Add criterion" }).click();
  await expect(rows(summary)).toHaveText([/1\. Board ships/]);
  await expect(summary.getByRole("option", { name: "Done" })).not.toHaveAttribute("disabled");
  expect(api.unhandled).toEqual([]);
});

test("the epic detail panel has the same criteria editor and done gate", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/epics");
  const panel = page.getByRole("region", { name: "Epic completion" });
  await expect(rows(panel)).toHaveText([/1\. Board shows all statuses/, /2\. Cards can move/]);
  await expect(panel).toContainText("5 / 10 pts done");
  await editor(panel).getByLabel("Criterion 2 met").check();
  await expect(panel).toContainText("2 / 2 met");
  await panel.getByLabel("Epic status").selectOption("done");
  const blocked = panel.getByRole("alert", { name: "Epic cannot be marked done" });
  await expect(blocked.getByRole("list", { name: "Open tickets" })).toContainText("WEB-1");
  await expect(blocked.getByRole("list", { name: "Unchecked criteria" })).toHaveCount(0);
  await expect(panel.getByLabel("Epic status")).toHaveValue("open");
  expect(api.epicWrites.map((w) => w.body)).toEqual([{ done: true }, { status: "done" }]);
});
