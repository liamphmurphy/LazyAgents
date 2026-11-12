import { expect, test, type Page } from "@playwright/test";
import { installBoardApi } from "./mock-board-api";

/** Bulk edits PATCH each ticket without a revision; WEB-4 has a live run, so canceling it is a 409. */
async function installBulkPatches(page: Page, api: Awaited<ReturnType<typeof installBoardApi>>, waitForRelease?: Promise<void>) {
  const patched: Array<{ key: string; body: Record<string, unknown> }> = [];
  await page.route("**/api/v1/tickets/*", async (route) => {
    const req = route.request();
    const key = new URL(req.url()).pathname.split("/").at(-1)!;
    const t = api.tickets.find((x) => x.key === key);
    if (req.method() !== "PATCH" || !t) return route.fallback();
    const body = req.postDataJSON() as Record<string, unknown>;
    patched.push({ key, body });
    await waitForRelease;
    if (body.status === "canceled" && key === "WEB-4") {
      return route.fulfill({ status: 409, json: { code: "conflict", message: "cancel the run first" } });
    }
    if (typeof body.status === "string") t.status = body.status as typeof t.status;
    if ("estimate_points" in body) t.estimate_points = (body.estimate_points as number | null) ?? undefined;
    return route.fulfill({ json: t });
  });
  return patched;
}

const row = (page: Page, key: string) => page.locator(`[data-ticket-row="${key}"]`);

test("rows show estimate, criteria, blocked and PR; grouping lives in the URL", async ({ page }) => {
  const api = await installBoardApi(page);
  api.tickets[4].pr = { number: 41, state: "open" };
  await page.goto("/tickets?project=WEB");

  await expect(row(page, "WEB-1")).toContainText("Blocked by 1");
  await expect(row(page, "WEB-1")).toContainText("1/1");
  await expect(row(page, "WEB-1").getByTitle("3 points")).toBeVisible();
  await expect(row(page, "WEB-2").getByTitle("Not estimated")).toBeVisible();
  await expect(row(page, "WEB-5")).toContainText("In review · PR #41");

  await page.getByRole("button", { name: "Group by Status" }).click();
  await page.getByRole("menuitemradio", { name: "Epic" }).click();
  await expect(page).toHaveURL(/group=epic/);
  await expect(page.getByRole("region", { name: "Kanban board, 5" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Other epic, 1" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("button", { name: "Group by Epic" })).toBeVisible();

  for (const [name, region] of [["Assignee", "Builder, 3"], ["Priority", "Urgent, 1"]] as const) {
    await page.getByRole("button", { name: /Group by/ }).click();
    await page.getByRole("menuitemradio", { name }).click();
    await expect(page).toHaveURL(new RegExp(`group=${name.toLowerCase()}`));
    await expect(page.getByRole("region", { name: region })).toBeVisible();
  }
  // Status is the default and leaves the URL.
  await page.getByRole("button", { name: /Group by/ }).click();
  await page.getByRole("menuitemradio", { name: "Status" }).click();
  await expect(page).not.toHaveURL(/group=/);

  await page.getByRole("radio", { name: "Compact rows" }).click();
  await expect(row(page, "WEB-1")).toHaveClass(/h-8/);
  await page.reload();
  await expect(row(page, "WEB-1")).toHaveClass(/h-8/);
  expect(api.unhandled).toEqual([]);
});

test("keyboard selection and bulk status report per-ticket failures", async ({ page }) => {
  const api = await installBoardApi(page);
  const patched = await installBulkPatches(page, api);
  await page.goto("/tickets?project=WEB");
  await expect(row(page, "WEB-4")).toBeVisible();

  // Status order: WEB-4 (in progress), WEB-5 (in review), WEB-1 (todo), …
  await page.keyboard.press("j");
  await expect(row(page, "WEB-4")).toHaveAttribute("data-focused", "true");
  await page.keyboard.press("x");
  await page.keyboard.press("j");
  await page.keyboard.press("j");
  await expect(row(page, "WEB-1")).toHaveAttribute("data-focused", "true");
  await page.keyboard.press("Shift+X");
  const bar = page.getByRole("toolbar", { name: "Bulk actions" });
  await expect(bar).toContainText("3 selected");
  await page.keyboard.press("k");
  await expect(row(page, "WEB-5")).toHaveAttribute("data-focused", "true");

  await bar.getByRole("button", { name: "Status" }).click();
  await page.getByRole("menuitem", { name: "Canceled" }).click();
  const result = page.getByRole("alert", { name: "Bulk result" });
  await expect(result).toContainText("Status → Canceled");
  await expect(result).toContainText("2 updated, 1 failed");
  await expect(result).toContainText("WEB-4");
  await expect(result).toContainText("409");
  await expect(result).toContainText("Cancel the run first: a run is live on this ticket.");
  await expect(result.getByRole("link", { name: "Open ticket →" })).toHaveAttribute("href", "/tickets/WEB-4");
  // Only the failure stays selected.
  await expect(bar).toContainText("1 selected");
  expect(patched.map((p) => p.key).sort()).toEqual(["WEB-1", "WEB-4", "WEB-5"]);
  expect(patched.every((p) => !("revision" in p.body))).toBe(true);

  await bar.getByRole("button", { name: "Estimate" }).click();
  await page.getByRole("menuitem", { name: "5 points" }).click();
  await expect(page.getByRole("status", { name: "Bulk result" })).toContainText(/Estimate → 5\s*· 1 updated/);
  expect(patched.at(-1)).toEqual({ key: "WEB-4", body: { estimate_points: 5 } });

  await page.keyboard.press("j");
  await page.keyboard.press("x");
  await expect(bar).toContainText("1 selected");
  await page.keyboard.press("Escape");
  await expect(bar).toHaveCount(0);
  expect(api.unhandled).toEqual([]);
});

test("saved views are created, edited, renamed and deleted from the sidebar", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1"));
  const api = await installBoardApi(page);
  await page.goto("/tickets?project=WEB&view=active&group=epic");
  const views = page.getByRole("group", { name: "Saved views" });

  await views.getByRole("button", { name: "Save view" }).click();
  const dialog = page.getByRole("dialog", { name: "Save view" });
  await expect(dialog).toContainText("Saves: View Active · Group Epic · Agent any · Search none");
  await dialog.getByLabel("Name").fill("UX epic triage");
  await dialog.getByRole("button", { name: "Save view" }).click();
  await expect(page).toHaveURL(/sv=view-1/);
  await expect(views.getByRole("link", { name: "UX epic triage" })).toHaveAttribute("aria-current", "page");
  expect(api.savedViews[0]).toMatchObject({ name: "UX epic triage", query: "view=active&group=epic" });

  // Suggested views fill the name and filters.
  await views.getByRole("button", { name: "Save view" }).click();
  await dialog.getByRole("button", { name: "+ Blocked" }).click();
  await dialog.getByRole("button", { name: "Save view" }).click();
  await expect(views.getByRole("link", { name: "Blocked" })).toBeVisible();
  await expect(page).toHaveURL(/blocked=true/);
  await expect(row(page, "WEB-1")).toBeVisible();
  await expect(row(page, "WEB-2")).toHaveCount(0);

  // Editing the open view's grouping marks it Edited until saved.
  await views.getByRole("link", { name: "UX epic triage" }).click();
  await page.getByRole("button", { name: "Group by Epic" }).click();
  await page.getByRole("menuitemradio", { name: "Priority" }).click();
  await expect(page.getByText("Edited", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText("Edited", { exact: true })).toHaveCount(0);
  expect(api.savedViews[0].query).toBe("view=active&group=priority");

  // Rename from the view header keeps the box open and focused.
  await page.getByRole("button", { name: "Saved view actions" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const header = page.getByRole("textbox", { name: "Rename UX epic triage" });
  await expect(header).toBeFocused();
  await page.waitForTimeout(100);
  await expect(header).toBeFocused();
  await header.fill("UX board");
  await header.press("Enter");
  await expect(page.getByRole("heading", { name: "UX board" })).toBeVisible();
  // F2 renames the open view too; Esc cancels.
  await page.keyboard.press("F2");
  await expect(page.getByRole("textbox", { name: "Rename UX board" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("textbox", { name: "Rename UX board" })).toHaveCount(0);

  // Inline rename in the sidebar; a duplicate name is a 409.
  await views.getByRole("button", { name: "UX board view actions" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  const input = views.getByRole("textbox", { name: "Rename UX board" });
  await input.fill("blocked");
  await input.press("Enter");
  await expect(views.getByRole("alert")).toHaveText("You already have a view with that name");
  await input.fill("UX triage");
  await input.press("Enter");
  await expect(views.getByRole("link", { name: "UX triage" })).toBeVisible();

  page.once("dialog", (d) => d.accept());
  await views.getByRole("button", { name: "UX triage view actions" }).click();
  await page.getByRole("menuitem", { name: "Delete view" }).click();
  await expect(views.getByRole("link", { name: "UX triage" })).toHaveCount(0);
  await expect(page).not.toHaveURL(/sv=/);
  expect(api.savedViews.map((v) => v.name)).toEqual(["Blocked"]);
  expect(api.unhandled).toEqual([]);
});


test("bulk results survive clearing selection during delayed mixed responses", async ({ page }) => {
  const api = await installBoardApi(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  const patched = await installBulkPatches(page, api, gate);
  await page.goto("/tickets?project=WEB");
  await row(page, "WEB-4").getByRole("checkbox").check();
  await row(page, "WEB-1").getByRole("checkbox").check();
  const bar = page.getByRole("toolbar", { name: "Bulk actions" });
  await bar.getByRole("button", { name: "Status", exact: true }).click();
  await page.getByRole("menuitem", { name: "Canceled" }).click();
  await expect.poll(() => patched.length).toBeGreaterThan(0);
  await bar.getByRole("button", { name: "Clear selection" }).click();
  await expect(bar).toHaveCount(0);
  release();
  const result = page.getByRole("alert", { name: "Bulk result" });
  await expect(result).toContainText("1 updated, 1 failed");
  await expect(result).toContainText("WEB-4");
  await expect(result).toContainText("409");
});

test("row checkbox focus permits list shortcuts while search keeps typing", async ({ page }) => {
  await installBoardApi(page);
  await page.goto("/tickets?project=WEB");
  const checkbox = row(page, "WEB-4").getByRole("checkbox");
  await checkbox.focus();
  await page.keyboard.press("Space");
  await expect(checkbox).toBeChecked();
  await expect(row(page, "WEB-4")).toHaveAttribute("data-focused", "true");
  await page.keyboard.press("j");
  await expect(row(page, "WEB-5")).toHaveAttribute("data-focused", "true");
  await page.keyboard.press("k");
  await expect(row(page, "WEB-4")).toHaveAttribute("data-focused", "true");
  await page.keyboard.press("x");
  await expect(checkbox).not.toBeChecked();
  await page.keyboard.press("x");
  await page.keyboard.press("Escape");
  await expect(checkbox).not.toBeChecked();
  const search = page.getByLabel("Filter tickets");
  await search.fill("jkx");
  await expect(search).toHaveValue("jkx");
  await expect(checkbox).not.toBeChecked();
});

test("tickets waiting on the person are grouped first with an Open action", async ({ page }) => {
  const api = await installBoardApi(page);
  api.inbox.push(
    { id: "approval:ap-1", kind: "approval", title: "Other epic work", summary: "Push to main", href: "/tickets/WEB-4", project: "WEB", ticket_key: "WEB-4", created_at: "2026-11-12T10:00:00Z", resolved: false, read: false, run_id: "run-4" },
    { id: "review:t-5", kind: "review", title: "Review board", summary: "Ready for your review", href: "/tickets/WEB-5", project: "WEB", ticket_key: "WEB-5", created_at: "2026-11-12T10:00:00Z", resolved: false, read: false },
  );
  await page.goto("/tickets?project=WEB");
  const sections = page.locator("main section[aria-label]");
  await expect(sections.first()).toHaveAttribute("aria-label", "Needs you, 1");
  const needs = page.getByRole("region", { name: "Needs you, 1" });
  await expect(needs.locator("[data-ticket-row]")).toHaveAttribute("data-ticket-row", "WEB-4");
  await expect(row(page, "WEB-4")).toContainText("Wants approval: Push to main");
  await expect(needs.getByRole("link", { name: "Open what WEB-4 needs" })).toHaveAttribute("href", "/tickets/WEB-4");
  // A review isn't a run waiting on the person; the rest follow in status buckets.
  await expect(page.getByRole("region", { name: "In progress, 1" })).toContainText("WEB-5");
  await expect(page.getByRole("region", { name: "Ready, 1" })).toBeVisible();
  // The board shows the same state on the card.
  await page.goto("/tickets?layout=board&project=WEB");
  const card = page.locator('[data-ticket-key="WEB-4"]');
  await expect(card).toHaveAttribute("data-needs-you", "true");
  await expect(card).toContainText("Wants approval: Push to main");
  await expect(card.getByRole("link", { name: "Open what WEB-4 needs" })).toBeVisible();
  expect(api.unhandled).toEqual([]);
});

test("filter tokens narrow the list and round-trip through the URL", async ({ page }) => {
  const api = await installBoardApi(page);
  await page.goto("/tickets?project=WEB");
  const box = page.getByLabel("Filter tickets");
  await box.fill('epic:"Other epic" agent:reviewer status:in_progress board');
  await expect(page).toHaveURL(/epic=epic-2/);
  await expect(page).toHaveURL(/assignee=reviewer/);
  await expect(page).toHaveURL(/status=in_progress/);
  await expect(page).toHaveURL(/q=board/);
  await box.fill("epic:nothing");
  await expect(page.getByRole("status").filter({ hasText: "No match for epic:nothing" })).toBeVisible();
  await expect(page).not.toHaveURL(/epic=/);
  // Opening a filtered link shows its filters as tokens.
  await page.goto("/tickets?project=WEB&epic=epic-1&status=todo");
  await expect(box).toHaveValue('epic:"Kanban board" status:todo');
  // project: moves the picker and leaves the box, so it can't reset the picker later.
  await box.fill("project:ops bug");
  await expect(page).toHaveURL(/project=OPS/);
  await expect(box).toHaveValue("bug");
  // A late debounce from before the navigation must not switch the project back.
  await page.waitForTimeout(600);
  await expect(page).toHaveURL(/project=OPS/);
  await expect(page).toHaveURL(/q=bug/);
  expect(api.unhandled).toEqual([]);
});

test("Back to a filter the box applied shows it again", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1"));
  const api = await installBoardApi(page);
  api.savedViews.push({ id: "sv-plain", name: "Plain", query: "view=active", created_at: "2026-11-01T00:00:00Z", updated_at: "2026-11-01T00:00:00Z" });
  await page.goto("/tickets?project=WEB");
  const box = page.getByLabel("Filter tickets");
  await box.fill("status:done");
  await expect(page).toHaveURL(/status=done/);
  await page.getByRole("group", { name: "Saved views" }).getByRole("link", { name: "Plain" }).click();
  await expect(page).toHaveURL(/sv=sv-plain/);
  await expect(box).toHaveValue("");
  await page.goBack();
  await expect(page).toHaveURL(/status=done/);
  await expect(box).toHaveValue("status:done");
});

test.describe("on a touch screen", () => {
  test.use({ hasTouch: true });

  test("row actions show without hovering", async ({ page }) => {
    await installBoardApi(page);
    await page.goto("/tickets?project=WEB");
    expect(await page.evaluate(() => matchMedia("(hover: none)").matches)).toBe(true);
    const select = page.getByRole("checkbox", { name: "Select WEB-2" });
    await expect(select).toHaveCSS("opacity", "1");
    await select.tap();
    await expect(select).toBeChecked();
  });
});
