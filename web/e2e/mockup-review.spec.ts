import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The mockup review workspace (LAZYAGENTS-51) against a mocked artifact with
// three versions: v3 is current and pending, v2 had changes requested with a pin.

const A = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const V = ["11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"];
const svg = (label: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="625" viewBox="0 0 1000 625"><rect width="1000" height="625" fill="#fff"/><text x="40" y="80" font-size="48">${label}</text></svg>`;
const liam = { id: "user-1", name: "Liam", initials: "L" };
const agent = { type: "agent", id: "agent-1", name: "Mastermind", color_index: 0 };

type Pin = { x: number; y: number; text: string };
type Review = { state: "pending" | "approved" | "changes_requested"; note: string; pins: Pin[]; reviewed_by?: typeof liam; reviewed_at?: string };

function version(i: number, review: Review) {
  return {
    id: V[i], seq: i + 1, label: `v${i + 1}`, note: "", review, created_by: agent, created_at: `2026-11-0${i + 1}T10:00:00Z`,
    content: { kind: "svg", content_type: "image/svg+xml", size_bytes: 200, sections: 0 },
  };
}

async function install(page: Page) {
  await installChatApi(page);
  let current: Review = { state: "pending", note: "", pins: [] };
  const posted: Array<Record<string, unknown>> = [];
  let ticketGets = 0;
  const artifact = () => {
    const versions = [
      version(2, current),
      version(1, { state: "changes_requested", note: "Move the filters", pins: [{ x: 0.2, y: 0.3, text: "Filters above columns" }], reviewed_by: liam, reviewed_at: "2026-11-02T12:00:00Z" }),
      version(0, { state: "changes_requested", note: "Start over", pins: [], reviewed_by: liam, reviewed_at: "2026-11-01T12:00:00Z" }),
    ];
    return {
      id: A, type: "mockup", title: "Login screen", summary: "", suggestions: 0, author: agent, body_md: svg("v3"),
      current_version: versions[0], versions, outline: [], read_by: [],
      tickets: [{ key: "WEB-7", title: "Login", status: "in_progress" }],
      created_at: "2026-11-01T10:00:00Z", updated_at: "2026-11-03T10:00:00Z",
    };
  };
  await page.route(`**/api/v1/artifacts/${A}**`, async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    const content = /\/versions\/([^/]+)\/content$/.exec(path);
    if (content) return route.fulfill({ contentType: "image/svg+xml", body: svg(`v${V.indexOf(content[1]) + 1}`) });
    if (path.endsWith("/review") && req.method() === "POST") {
      const body = req.postDataJSON();
      posted.push(body);
      const pins = (body.pins ?? []) as Pin[];
      current = {
        state: body.decision, pins, reviewed_by: liam, reviewed_at: "2026-11-06T12:00:00Z",
        note: [body.note, pins.length ? `Pinned comments:\n${pins.map((p, i) => `${i + 1}. ${p.text}`).join("\n")}` : ""].filter(Boolean).join("\n\n"),
      };
      return route.fulfill({ json: artifact() });
    }
    return route.fulfill({ json: artifact() });
  });
  await page.route("**/api/v1/tickets/WEB-7", (route) => {
    ticketGets++;
    return route.fulfill({ json: {
      id: "t-7", key: "WEB-7", title: "Login", status: "in_progress",
      criteria: [
        { id: "c1", position: 1, text: "Shows a forgot-password link", done: false },
        { id: "c2", position: 2, text: "Errors appear under the field", done: false },
      ],
    } });
  });
  return { posted, ticketGets: () => ticketGets };
}

async function box(page: Page, alt: string) {
  const b = await page.getByRole("img", { name: alt }).boundingBox();
  if (!b) throw new Error(`no box for ${alt}`);
  return b;
}

test("review workspace zooms, pans, compares versions and pins comments", async ({ page }) => {
  const api = await install(page);
  await page.goto(`/artifacts/${A}/review?ticket=WEB-7`);
  const dialog = page.getByRole("dialog", { name: "Login screen" });
  await expect(dialog).toBeVisible();
  await expect(page.getByRole("img", { name: "Login screen v3" })).toBeVisible();

  // Fit, 100% and zoom by button, keyboard and ⌘/Ctrl+wheel.
  const zoom = page.getByLabel("Zoom level");
  const pinTool = page.getByRole("toolbar", { name: "Canvas tools" }).getByRole("button", { name: /^Pin/ });
  const fitText = await zoom.textContent();
  await page.getByRole("button", { name: "100%" }).click();
  await expect(zoom).toHaveText("100%");
  expect((await box(page, "Login screen v3")).width).toBeCloseTo(1000, 0);
  await page.keyboard.press("0");
  await expect(zoom).toHaveText(fitText!);
  await page.keyboard.press("1");
  await page.keyboard.press("+");
  await expect(zoom).toHaveText("125%");
  await page.getByRole("button", { name: "Zoom out" }).click();
  await expect(zoom).toHaveText("100%");
  const canvas = page.getByRole("group", { name: "v3 canvas" });
  const cb = (await canvas.boundingBox())!;
  await page.mouse.move(cb.x + cb.width / 2, cb.y + cb.height / 2);
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -100);
  await page.keyboard.up("Control");
  await expect(zoom).not.toHaveText("100%");
  await page.keyboard.press("0");

  // Pan by keyboard and by dragging.
  let before = await box(page, "Login screen v3");
  await page.keyboard.press("ArrowRight");
  await expect.poll(async () => (await box(page, "Login screen v3")).x).toBeCloseTo(before.x - 48, 0);
  before = await box(page, "Login screen v3");
  await page.mouse.move(cb.x + 200, cb.y + 200);
  await page.mouse.down();
  await page.mouse.move(cb.x + 260, cb.y + 230, { steps: 5 });
  await page.mouse.up();
  const after = await box(page, "Login screen v3");
  expect(after.x - before.x).toBeCloseTo(60, 0);
  expect(after.y - before.y).toBeCloseTo(30, 0);
  await page.keyboard.press("0");

  // Side by side: v2 (with its saved pin) beside v3.
  await page.getByRole("radio", { name: "Side by side" }).click();
  await expect(page.getByRole("img", { name: "Login screen v2" })).toBeVisible();
  await expect(page.getByRole("img", { name: "Login screen v3" })).toBeVisible();
  await expect(page.getByRole("group", { name: "v2 canvas" }).getByRole("button", { name: "Pin 1: Filters above columns" })).toBeVisible();
  const left = await box(page, "Login screen v2");
  const right = await box(page, "Login screen v3");
  expect(left.x).toBeLessThan(right.x);
  expect(left.width).toBeCloseTo(right.width, 0);

  // Swipe: a slider moves by keyboard.
  await page.getByRole("radio", { name: "Swipe" }).click();
  const slider = page.getByRole("slider", { name: "Swipe between v2 and v3" });
  await expect(slider).toHaveAttribute("aria-valuenow", "50");
  await slider.focus();
  await page.keyboard.press("ArrowRight");
  await expect(slider).toHaveAttribute("aria-valuenow", "52");
  await page.keyboard.press("Shift+ArrowLeft");
  await expect(slider).toHaveAttribute("aria-valuenow", "42");
  await page.keyboard.press("Home");
  await expect(slider).toHaveAttribute("aria-valuenow", "0");
  await page.getByRole("radio", { name: "Single" }).click();

  // Criteria from the ticket sit beside the canvas.
  await expect(page.getByRole("checkbox", { name: "Shows a forgot-password link" })).toBeVisible();

  // Pin a comment: C, click the drawing, type, ⌘/Ctrl+Enter.
  await dialog.focus();
  await page.keyboard.press("c");
  await expect(pinTool).toHaveAttribute("aria-pressed", "true");
  const d = await box(page, "Login screen v3");
  await page.mouse.click(d.x + d.width / 2, d.y + d.height / 4);
  await page.getByRole("textbox", { name: "Comment 1" }).fill("Make the button bigger");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(canvas.getByRole("button", { name: "Pin 1: Make the button bigger" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Pinned comments" })).toContainText("Make the button bigger");

  // Request changes needs a note; pins are sent with it.
  const request = page.getByRole("button", { name: "Request changes" });
  await expect(request).toBeDisabled();
  await page.getByRole("textbox", { name: "Note" }).fill("Close, but see the pin");
  const gets = api.ticketGets();
  await request.click();
  await expect(page.getByRole("status").filter({ hasText: "Changes requested on v3." })).toBeVisible();
  expect(api.posted).toHaveLength(1);
  const body = api.posted[0] as { decision: string; note: string; pins: Pin[] };
  expect(body.decision).toBe("changes_requested");
  expect(body.note).toBe("Close, but see the pin");
  expect(body.pins).toHaveLength(1);
  expect(body.pins[0].text).toBe("Make the button bigger");
  expect(body.pins[0].x).toBeCloseTo(0.5, 1);
  expect(body.pins[0].y).toBeCloseTo(0.25, 1);
  // The ticket (and its mockup gate) is fetched again.
  await expect.poll(api.ticketGets).toBeGreaterThan(gets);
  // The pin is now part of the saved review; deciding again doesn't resend it or the old note.
  await expect(canvas.getByRole("button", { name: "Pin 1: Make the button bigger" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Pinned comments" })).toContainText("Saved with the last decision");
  await expect(page.getByRole("button", { name: "Remove pin 1" })).toHaveCount(0);
  await page.getByRole("button", { name: "Approve v3" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Approved v3." })).toBeVisible();
  expect(api.posted[1]).toEqual({ decision: "approved", pins: [] });

  // Older versions show their saved pins and can't be decided on.
  await dialog.focus();
  await page.keyboard.press("[");
  await expect(page.getByRole("img", { name: "Login screen v2" })).toBeVisible();
  await expect(page.getByRole("group", { name: "v2 canvas" }).getByRole("button", { name: "Pin 1: Filters above columns" })).toBeVisible();
  await expect(page.getByText("Only the current version can be reviewed.")).toBeVisible();
  await expect(pinTool).toBeDisabled();
});

test("Review on the mockup page opens the workspace; approving sends the decision and Esc returns", async ({ page }) => {
  const api = await install(page);
  await page.goto(`/artifacts/${A}`);
  await page.getByRole("link", { name: "Review", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/artifacts/${A}/review$`));
  await expect(page.getByRole("img", { name: "Login screen v3" })).toBeVisible();
  await page.getByRole("button", { name: "Approve v3" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Approved v3." })).toBeVisible();
  expect(api.posted).toEqual([{ decision: "approved", pins: [] }]);
  await expect(page.getByRole("button", { name: "Approve v3" })).toBeDisabled();
  await page.getByRole("dialog").focus();
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(new RegExp(`/artifacts/${A}$`));
});

test("opened directly, Tab stays in the workspace and Close falls back to the ticket", async ({ page }) => {
  await install(page);
  await page.goto(`/artifacts/${A}/review`);
  await expect(page.getByRole("img", { name: "Login screen v3" })).toBeVisible();
  const inDialog = () => page.evaluate(() => !!document.querySelector('[role="dialog"]')?.contains(document.activeElement));
  await page.getByRole("dialog").focus();
  await page.keyboard.press("Shift+Tab");
  expect(await inDialog()).toBe(true);
  await page.keyboard.press("Tab");
  // The workspace's own Back, not the rail's Go back behind the dialog.
  await expect(page.getByRole("dialog").getByRole("button", { name: "Back", exact: true })).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  expect(await inDialog()).toBe(true);
  await page.getByRole("button", { name: "Close" }).click();
  await expect(page).toHaveURL(/\/tickets\/WEB-7$/);
});
