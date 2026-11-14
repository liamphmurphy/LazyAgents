import { expect, test, type Locator, type Page } from "@playwright/test";
import { installChatApi, openEmptyChat, persona } from "./mock-chat-api";

// The main chat's layout contract: at any width the header is one row and the
// composer's controls fit in one row under the box, and nothing in either is cut
// off. A new control that would crowd the chat fails here, not in review.

const REPO = "https://github.com/acme/a-repository-with-a-long-name";
const dev = persona(1, { name: "Hackerman", is_default: true, capabilities: { code: true, images: false }, repos: [REPO] });

/** Children of the box that are cut off or pushed outside it. */
async function overflowing(box: Locator): Promise<string[]> {
  return box.evaluate((root) => {
    const r = root.getBoundingClientRect();
    const out: string[] = [];
    for (const el of root.querySelectorAll<HTMLElement>("button, a, [role=combobox], [data-testid]")) {
      if (!el.offsetParent) continue;
      const b = el.getBoundingClientRect();
      const label = el.getAttribute("aria-label") || el.textContent?.trim().slice(0, 30) || el.tagName;
      if (b.right > r.right + 1 || b.left < r.left - 1) out.push(`${label}: outside the box`);
      else if (el.scrollWidth > el.clientWidth + 1 && getComputedStyle(el).overflow !== "visible" && !el.matches("[role=combobox]")) out.push(`${label}: clipped`);
    }
    return out;
  });
}

const header = (page: Page) => page.getByTestId("chat-header");
const controls = (page: Page) => page.getByTestId("composer-controls");

for (const width of [1440, 1024, 860]) {
  test(`at ${width}px the header is one row and the composer's controls one row, nothing cut off`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await installChatApi(page, { personas: [dev] });
    await openEmptyChat(page, { title: "Knowledge chat dock fixes and a title long enough to need truncating", repo_url: REPO });

    await expect(header(page)).toBeVisible();
    const h = await header(page).boundingBox();
    expect(h!.height).toBeLessThanOrEqual(60);
    expect(await overflowing(header(page))).toEqual([]);

    const row = await controls(page).boundingBox();
    // One row of 28–36px controls; a second row would be over 60.
    expect(row!.height).toBeLessThanOrEqual(44);
    expect(await overflowing(controls(page))).toEqual([]);
  });
}

test("while a reply runs, with Research on, the controls still fit one row; options live in +", async ({ page }) => {
  await page.setViewportSize({ width: 860, height: 900 });
  await installChatApi(page, { personas: [dev], replyDelayMs: 10_000 });
  await openEmptyChat(page, { repo_url: REPO });

  // Every option is in +, not in the row.
  await expect(controls(page).getByRole("button", { name: "Research" })).toHaveCount(0);
  await controls(page).getByRole("button", { name: "Composer options" }).click();
  const menu = page.getByTestId("composer-menu");
  for (const option of ["context", "attach", "code", "research"]) await expect(menu.locator(`[data-option="${option}"]`)).toBeVisible();
  // The repository is chat-level, so it sits in the header, not in +.
  await expect(menu.locator('[data-option="repository"]')).toHaveCount(0);
  await expect(header(page).getByTestId("repository-picker")).toContainText("a-repository-with-a-long-name");
  await menu.getByRole("button", { name: "Research" }).click();
  await page.keyboard.press("Escape");
  // A mode that is on shows as its own pill, and pressing it turns it off.
  const research = page.getByTestId("composer-modes").getByRole("button", { name: "Research" });
  await expect(research).toHaveAttribute("aria-pressed", "true");

  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(controls(page).getByRole("radiogroup", { name: "Send as" })).toBeVisible();
  await expect(controls(page).getByRole("button", { name: /Stop/ })).toBeVisible();
  expect((await controls(page).boundingBox())!.height).toBeLessThanOrEqual(44);
  expect(await overflowing(controls(page))).toEqual([]);
});

test("while a reply runs, the header says what the persona is doing and opens Activity", async ({ page }) => {
  await installChatApi(page, { personas: [dev], replyDelayMs: 10_000 });
  await openEmptyChat(page, { repo_url: REPO });
  await expect(page.getByTestId("chat-presence")).toHaveCount(0);
  await page.getByRole("combobox", { name: "Message" }).fill("hello");
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  const presence = header(page).getByTestId("chat-presence");
  await expect(presence).toHaveText(/^Hackerman · .+…$/);
  // The status card is on screen and already says it, so the header keeps only the pulsing dot.
  await expect(page.getByTestId("chat-status-card")).toBeInViewport();
  await expect(presence).toHaveAttribute("data-dot-only", "true");
  expect((await header(page).boundingBox())!.height).toBeLessThanOrEqual(60);
  expect(await overflowing(header(page))).toEqual([]);
  await presence.click();
  await expect(page).toHaveURL(/[?&]panel=activity/);
  // Activity is the dock's only tab, so there is no tab strip: its panel shows under the dock's header.
  await expect(page.getByRole("complementary", { name: "Panel" }).getByRole("tabpanel", { name: "Activity" })).toBeVisible();
});
