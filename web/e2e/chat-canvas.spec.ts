import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// History lives in the expanded menu.
test.beforeEach(({ page }) => page.addInitScript(() => localStorage.setItem("lazyagents.sidebar-expanded", "1")));

// A ```canvas fence opens a canvas beside the chat, without running its content.

const DOC = "11111111-1111-4111-8111-111111111111";
const SVG = "22222222-2222-4222-8222-222222222222";
const at = "2026-11-08T12:00:00Z";
const summary = (id: string, kind: string, title: string, seq: number) => ({ id, kind, title, seq, created_at: at, updated_at: at });
const versions = (n: number) => Array.from({ length: n }, (_, i) => ({ seq: n - i, note: `Note ${n - i}`, author_type: "agent", created_at: at }));
const svgBody = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20" onload="document.title='ran'"><script>document.title="ran"</script><rect data-vid="abc" width="40" height="20" fill="#36c"/></svg>`;

type Comment = Record<string, unknown> & { id: string; status: string };
type Calls = { comments: Comment[]; sent: Record<string, unknown>[]; added: Record<string, unknown>[]; saved: Record<string, unknown>[]; restored: unknown[]; promoted: number; applied: string[] };

async function open(page: Page, initial: Comment[] = [], card = DOC) {
  const calls: Calls = { comments: [...initial], sent: [], added: [], saved: [], restored: [], promoted: 0, applied: [] };
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases(\/|\?|$)/, (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const seq = Number(url.searchParams.get("seq") ?? 2);
    const body = () => (req.postDataJSON() ?? {}) as Record<string, unknown>;
    const json = (data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    const version = { seq: 3, note: "x", author_type: "user", created_at: at };
    const p = url.pathname;
    if (p.endsWith("/comments/send")) {
      calls.sent.push(body());
      for (const c of calls.comments) if ((body().comment_ids as string[]).includes(c.id) && c.status === "draft") c.status = "open";
      return route.fulfill({ status: 204 });
    }
    if (p.endsWith("/apply")) {
      calls.applied.push(p);
      return json(version, 201);
    }
    const one = /\/comments\/([^/]+)$/.exec(p);
    if (one) {
      const c = calls.comments.find((x) => x.id === one[1])!;
      if (req.method() === "DELETE") {
        calls.comments = calls.comments.filter((x) => x !== c);
        return route.fulfill({ status: 204 });
      }
      c.status = body().status as string;
      return json(c);
    }
    if (p.endsWith("/comments")) {
      if (req.method() === "GET") return json({ comments: calls.comments });
      const b = body();
      calls.added.push(b);
      const id = `c${calls.added.length}`;
      const c: Comment = { id, seq: 2, author_type: "user", body: b.body, anchor: b.vid ? { vid: b.vid, tag: "rect" } : { start: b.start, end: b.end, quote: "after" }, target_lost: false, status: "draft", created_at: at };
      calls.comments.push(c);
      return json(c, 201);
    }
    if (p.endsWith("/versions")) {
      calls.saved.push(body());
      return json(version, 201);
    }
    if (p.endsWith("/restore")) {
      calls.restored.push(body().seq);
      return json(version, 201);
    }
    if (p.endsWith("/promote")) {
      calls.promoted++;
      return json({ artifact_id: "33333333-3333-4333-8333-333333333333" });
    }
    if (p.endsWith("/canvases")) return json({ canvases: [summary(DOC, "text", "Retention policy", 2), summary(SVG, "svg", "Run screen", 1)] });
    if (p.endsWith(DOC)) return json({ ...summary(DOC, "text", "Retention policy", 2), viewing_seq: seq, body: `# Policy\n\nArchive **after** ${seq * 30} days.`, versions: versions(2) });
    return json({ ...summary(SVG, "svg", "Run screen", 1), viewing_seq: 1, body: svgBody, versions: versions(1) });
  });
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill(`canvas: ${card}`);
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  return calls;
}

/** Selects `text` in the preview the way a mouse would. */
async function select(page: Page, from: string, to: string) {
  await page.getByTestId("canvas-text").evaluate((root, [a, b]) => {
    const spans = [...root.querySelectorAll<HTMLElement>("[data-src-start]")];
    const find = (s: string) => spans.find((x) => x.textContent!.includes(s))!;
    const range = document.createRange();
    range.setStart(find(a).firstChild!, find(a).textContent!.indexOf(a));
    range.setEnd(find(b).firstChild!, find(b).textContent!.indexOf(b) + b.length);
    getSelection()!.removeAllRanges();
    getSelection()!.addRange(range);
    root.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  }, [from, to]);
}

const openDoc = async (page: Page) => {
  await expect(page.getByTestId("canvas-panel")).toContainText("days.");
  return page.getByTestId("canvas-panel");
};

test("the newest card opens in the chat, collapses, and steps through versions", async ({ page }) => {
  await open(page);
  const pane = page.getByTestId("canvas-panel");
  await expect(pane).toContainText("after 60 days.");
  await expect(pane.getByTestId("canvas-version")).toHaveText("v2 of 2");
  await expect(pane).toContainText("Note 2");
  await expect(pane.getByRole("button", { name: "Next version" })).toBeDisabled();

  await pane.getByRole("button", { name: "Previous version" }).click();
  await expect(pane).toContainText("after 30 days.");
  await expect(pane.getByTestId("canvas-version")).toHaveText("v1 of 2");

  await pane.getByRole("tab", { name: "Source" }).click();
  await expect(pane).toContainText("# Policy");

  await pane.getByRole("button", { name: "Collapse canvas" }).click();
  await expect(pane).toHaveCount(0);
  const card = page.getByTestId("canvas-card");
  await expect(card).toContainText("Document · v2");
  await card.click();
  await expect(page.getByTestId("canvas-panel")).toBeVisible();
});

test("an SVG canvas draws sanitized elements that keep their ids", async ({ page }) => {
  await open(page, [], SVG);
  const pane = page.getByTestId("canvas-panel");
  await expect(pane.getByTestId("canvas-svg").locator('rect[data-vid="abc"]')).toBeVisible();
  await expect(pane.getByTestId("canvas-svg").locator("script")).toHaveCount(0);
  await expect(pane).toContainText("can't be shown here");
  expect(await page.title()).not.toBe("ran");
});

test("only the latest card for a canvas is live; earlier ones lead to it", async ({ page }) => {
  await open(page);
  await expect(page.getByTestId("canvas-panel")).toBeVisible();
  const composer = page.getByRole("combobox", { name: "Message" });
  await expect(composer).toBeEnabled();
  await composer.fill(`canvas: ${DOC}`);
  await composer.press("Enter");
  await expect(page.getByTestId("assistant-message")).toHaveCount(2);
  await expect(page.getByTestId("assistant-message").last().getByTestId("canvas-panel")).toBeVisible();
  await expect(page.getByTestId("canvas-panel")).toHaveCount(1);
  await expect(page.getByTestId("assistant-message").first().getByTestId("canvas-card")).toBeVisible();
});

test("fullscreen covers the whole window, keeps a message box, and Escape returns", async ({ page }) => {
  await page.setViewportSize({ width: 1500, height: 900 });
  const calls = await open(page);
  const inline = page.getByTestId("canvas-panel");
  await expect(inline).toContainText("days.");
  const history = page.getByRole("complementary", { name: "Chat history" });
  await expect(history).toBeVisible();
  const chat = (await page.getByRole("log", { name: "Conversation" }).boundingBox())!;
  const box = (await inline.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(chat.x);
  expect(box.x + box.width).toBeLessThanOrEqual(chat.x + chat.width);

  await inline.getByRole("button", { name: "Fullscreen" }).click();
  const full = page.getByTestId("canvas-fullscreen");
  await expect(full).toContainText("days.");
  await expect(inline).toHaveCount(0);
  expect(await full.boundingBox()).toEqual({ x: 0, y: 0, width: 1500, height: 900 });
  // On top of the app's sidebar, top bar and corner widgets, not just beside them.
  for (const [x, y] of [[10, 10], [10, 890], [700, 20], [1490, 890]]) {
    expect(await page.evaluate(([px, py]) => !!document.elementFromPoint(px, py)?.closest('[data-testid="canvas-fullscreen"]'), [x, y])).toBe(true);
  }
  await expect(full.getByTestId("canvas-latest-reply")).toContainText("Drafted it.");
  await expect(full.getByRole("textbox", { name: "Message from fullscreen" })).toBeVisible();

  await page.keyboard.press("Escape");
  await expect(full).toHaveCount(0);
  await expect(inline).toContainText("days.");
  expect(calls.sent).toHaveLength(0);
});

test("inline, a comment sits under its passage; fullscreen puts it alongside", async ({ page }) => {
  await open(page);
  const pane = await openDoc(page);
  await select(page, "Archive", "Archive");
  await pane.getByTestId("canvas-toolbar").getByRole("button", { name: "Comment" }).click();
  await expect(page.getByTestId("canvas-text").getByTestId("canvas-composer")).toBeVisible();
  await pane.getByRole("textbox", { name: "Comment" }).fill("Too short");
  await pane.getByRole("button", { name: "Add to batch" }).click();
  await expect(page.getByTestId("canvas-text").getByTestId("canvas-comment")).toContainText("Too short");
  await expect(pane.getByRole("complementary", { name: "Comments" })).toHaveCount(0);

  await pane.getByRole("button", { name: "Fullscreen" }).click();
  const full = page.getByTestId("canvas-fullscreen");
  await expect(full.getByRole("complementary", { name: "Comments" }).getByTestId("canvas-comment")).toContainText("Too short");
  await expect(full.getByTestId("canvas-text").getByTestId("canvas-comment")).toHaveCount(0);
});

test("a selection across formatting becomes a draft on the right source range, sent as a batch", async ({ page }) => {
  const calls = await open(page);
  const pane = await openDoc(page);
  await select(page, "Archive", "aft");
  await pane.getByTestId("canvas-toolbar").getByRole("button", { name: "Comment" }).click();
  const composer = pane.getByTestId("canvas-composer");
  await composer.getByRole("textbox", { name: "Comment" }).fill("Too short");
  await composer.getByRole("button", { name: "Add to batch" }).click();
  await expect(pane.getByTestId("canvas-comment")).toContainText("Too short");
  // "# Policy\n\n" is 10 long; the range covers the ** markers.
  expect(calls.added[0]).toMatchObject({ seq: 2, start: 10, end: 23, body: "Too short" });
  expect(calls.sent).toHaveLength(0);

  // The draft is painted without changing the rendered text.
  await expect.poll(() => page.evaluate(() => CSS.highlights.get("canvas-comment")?.size)).toBeGreaterThan(0);

  await pane.getByRole("button", { name: "Send 1 comment" }).click();
  await expect(pane.getByRole("button", { name: "Send 1 comment" })).toHaveCount(0);
  expect(calls.sent[0]).toEqual({ comment_ids: ["c1"] });
});

test("a quick action sends straight away", async ({ page }) => {
  const calls = await open(page);
  const pane = await openDoc(page);
  await select(page, "Archive", "Archive");
  await pane.getByTestId("canvas-toolbar").getByRole("button", { name: "Shorter" }).click();
  await expect.poll(() => calls.sent.length).toBe(1);
  expect(calls.added[0]).toMatchObject({ body: "Make this shorter.", start: 10, end: 17 });
  expect(calls.sent[0]).toEqual({ comment_ids: ["c1"], message: "Make this shorter." });
});

test("clicking an SVG element comments on it and pins the thread", async ({ page }) => {
  const calls = await open(page, [], SVG);
  const pane = page.getByTestId("canvas-panel");
  await pane.getByTestId("canvas-svg").locator('rect[data-vid="abc"]').click();
  const composer = pane.getByTestId("canvas-composer");
  await composer.getByRole("textbox", { name: "Comment" }).fill("Confirm first");
  await composer.getByRole("button", { name: "Send now" }).click();
  await expect(pane.getByTestId("canvas-pin")).toHaveCount(1);
  expect(calls.added[0]).toMatchObject({ seq: 1, vid: "abc", body: "Confirm first" });
  expect(calls.added[0].point).toEqual({ x: expect.any(Number), y: expect.any(Number) });
  expect(calls.sent[0]).toEqual({ comment_ids: ["c1"] });
});

test("threads can be applied, resolved, reopened, and show a lost target", async ({ page }) => {
  const base = { seq: 2, author_type: "agent", target_lost: false, created_at: at };
  const calls = await open(page, [
    { ...base, id: "s1", body: "Longer?", anchor: { start: 10, end: 17, quote: "Archive" }, status: "open", suggestion: "Store" },
    { ...base, id: "s2", body: "Gone one", anchor: { start: 0, end: 1, quote: "x" }, status: "open", target_lost: true },
    { ...base, id: "s3", body: "Handled and rewritten", anchor: { start: 0, end: 1, quote: "x" }, status: "addressed", target_lost: true },
    { ...base, id: "r1", parent_id: "s2", body: "Handled it", anchor: {}, status: "open" },
  ]);
  const pane = await openDoc(page);
  const first = pane.getByTestId("canvas-comment").filter({ hasText: "Longer?" });
  const second = pane.getByTestId("canvas-comment").filter({ hasText: "Gone one" });
  await expect(second).toContainText("Original target removed");
  await expect(pane.getByTestId("canvas-comment").filter({ hasText: "Handled and rewritten" })).not.toContainText("Original target removed");
  await expect(second).toContainText("Handled it");
  await first.getByRole("button", { name: "Apply" }).click();
  await expect.poll(() => calls.applied.length).toBe(1);

  await second.getByRole("button", { name: "Resolve" }).click();
  await expect(pane.getByText("1 resolved")).toBeVisible();
  await pane.getByText("1 resolved").click();
  await pane.getByTestId("canvas-comment").filter({ hasText: "Gone one" }).getByRole("button", { name: "Reopen" }).click();
  await expect(pane.getByText("1 resolved")).toHaveCount(0);
});

test("edit saves a version, changes show a diff, older versions restore, and it can become an artifact", async ({ page }) => {
  const calls = await open(page);
  const pane = await openDoc(page);

  await pane.getByRole("tab", { name: "Changes" }).click();
  const diff = pane.getByTestId("canvas-diff");
  await expect(diff).toContainText("Removed: ");
  await expect(diff).toContainText("30 days");
  await expect(diff).toContainText("60 days");

  await pane.getByRole("tab", { name: "Edit" }).click();
  await pane.getByRole("textbox", { name: "Canvas text" }).fill("Mine");
  await pane.getByRole("button", { name: "Save version" }).click();
  await expect.poll(() => calls.saved.length).toBe(1);
  expect(calls.saved[0]).toMatchObject({ base_seq: 2, body: "Mine" });

  await pane.getByRole("tab", { name: "Preview" }).click();
  await pane.getByRole("button", { name: "Previous version" }).click();
  await expect(pane.getByRole("tab", { name: "Edit" })).toBeDisabled();
  await pane.getByRole("button", { name: "Restore" }).click();
  await expect.poll(() => calls.restored).toEqual([1]);

  await pane.getByRole("button", { name: "Save as artifact" }).click();
  await expect.poll(() => calls.promoted).toBe(1);
});

test("fullscreen opens a board: zoomed out, and it pans and zooms", async ({ page }) => {
  await page.setViewportSize({ width: 1500, height: 900 });
  const calls = await open(page, [], SVG);
  await page.getByTestId("canvas-panel").getByRole("button", { name: "Fullscreen" }).click();
  const board = page.getByTestId("canvas-board");
  const svg = board.getByTestId("canvas-svg");
  await expect(svg).toBeVisible();
  await expect(board.getByTestId("canvas-zoom")).toHaveText("85%");
  const boxOf = async () => (await svg.boundingBox())!;
  const area = (await board.boundingBox())!;
  const start = await boxOf();
  expect(start.x).toBeGreaterThan(area.x);
  expect(start.y).toBeGreaterThan(area.y);
  expect(start.x + start.width).toBeLessThan(area.x + area.width);
  expect(start.y + start.height).toBeLessThan(area.y + area.height);

  // Dragging on the drawing pans it, and isn't taken for a click.
  const cx = start.x + start.width / 2;
  const cy = start.y + start.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx + 120, cy + 60, { steps: 4 });
  await page.mouse.up();
  const moved = await boxOf();
  expect(moved.x).toBeCloseTo(start.x + 120, 0);
  expect(moved.y).toBeCloseTo(start.y + 60, 0);
  await expect(page.getByTestId("canvas-composer")).toHaveCount(0);

  await board.getByRole("button", { name: "Zoom in" }).click();
  await expect(board.getByTestId("canvas-zoom")).toHaveText("106%");
  await page.mouse.move(area.x + 300, area.y + 300);
  await page.mouse.wheel(0, 300);
  await expect.poll(async () => parseInt((await board.getByTestId("canvas-zoom").textContent())!)).toBeLessThan(106);
  await board.getByRole("button", { name: "Fit" }).click();
  await expect.poll(async () => (await boxOf()).height).toBeGreaterThan(start.height);

  // A plain click still comments on the element, and pins land on it at this scale.
  await svg.locator('rect[data-vid="abc"]').click();
  const composer = page.getByTestId("canvas-composer");
  await composer.getByRole("textbox", { name: "Comment" }).fill("Confirm first");
  await composer.getByRole("button", { name: "Add to batch" }).click();
  await expect(board.getByTestId("canvas-pin")).toHaveCount(1);
  expect(calls.added[0]).toMatchObject({ vid: "abc" });
});

test("on the board a document keeps text selection and pans from its margins", async ({ page }) => {
  await page.setViewportSize({ width: 1500, height: 900 });
  await open(page);
  await page.getByTestId("canvas-panel").getByRole("button", { name: "Fullscreen" }).click();
  const board = page.getByTestId("canvas-board");
  const doc = board.getByTestId("canvas-board-content");
  await expect(board).toContainText("days.");
  const area = (await board.boundingBox())!;
  const before = (await doc.boundingBox())!;

  await page.mouse.move(area.x + 20, area.y + 300);
  await page.mouse.down();
  await page.mouse.move(area.x + 80, area.y + 340, { steps: 3 });
  await page.mouse.up();
  const after = (await doc.boundingBox())!;
  expect(after.x).toBeCloseTo(before.x + 60, 0);

  await page.mouse.move(area.x + 300, area.y + 300);
  await page.mouse.wheel(0, 40);
  await expect.poll(async () => (await doc.boundingBox())!.y).toBeCloseTo(after.y - 40, 0);
  await expect(board.getByTestId("canvas-zoom")).toHaveText("85%");

  await select(page, "Archive", "Archive");
  await expect(page.getByTestId("canvas-toolbar")).toBeVisible();
});

test("a failing canvas request doesn't hold up the composer", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases$/, (route) => route.fulfill({ status: 500, contentType: "application/json", body: '{"code":"internal","message":"boom"}' }));
  await page.goto("/chat?project=WEB");
  const composer = page.getByRole("combobox", { name: "Message" }).or(page.getByRole("combobox", { name: "Message" })).first();
  await composer.fill("hello");
  await composer.press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello");
  await expect(composer).toBeEnabled({ timeout: 1500 });
});

test("a fence naming an unknown canvas stays code", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases$/, (route) => route.fulfill({ status: 200, contentType: "application/json", body: '{"canvases":[]}' }));
  await page.goto("/chat?project=WEB");
  await page.getByRole("combobox", { name: "Message" }).fill(`canvas: ${DOC}`);
  await page.getByRole("combobox", { name: "Message" }).press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText(DOC);
  await expect(page.getByTestId("canvas-card")).toHaveCount(0);
});
