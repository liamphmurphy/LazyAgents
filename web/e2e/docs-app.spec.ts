import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The Docs app: documents (text canvases) you type on full page, beside their
// chat, with the persona's edits coming back as suggestions to review.

const DOC = "44444444-4444-4444-8444-444444444444";
const OTHER = "55555555-5555-4555-8555-555555555555";
const MOCKUP = "66666666-6666-4666-8666-666666666666";
const at = "2026-11-10T12:00:00Z";

type Version = { seq: number; author_type: "user" | "agent"; note: string; body: string };

async function install(page: Page, first = "# Trip plan\n\nFly to **Lisbon** on Friday.\n") {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  // A real chat in the mock, so the doc's conversation can load and send.
  await page.goto("/apps/docs");
  const chat = await page.evaluate(() => fetch("/api/v1/chats", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).then((r) => r.json()));
  const chatId = chat.id as string;
  const s = {
    versions: [{ seq: 1, author_type: "user", note: "", body: first }] as Version[],
    reviewed: 0,
    docsPosted: [] as unknown[],
    saves: [] as { base_seq: number; body: string; note?: string }[],
    reviewedPosts: [] as number[],
    library: 0,
    comments: [] as Record<string, unknown>[],
    sent: [] as Record<string, unknown>[],
  };
  const head = () => s.versions[s.versions.length - 1];
  const mine = (id: string, kind: string, title: string, chatTitle: string) => ({ id, kind, title, chat_id: chatId, chat_title: chatTitle, updated_at: at, reviewed_seq: s.reviewed });
  await page.route(/\/api\/v1\/(canvases|docs)(\/|$)/, (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname;
    const json = (data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    if (p.endsWith("/reviewed")) {
      const seq = req.postDataJSON().seq as number;
      s.reviewedPosts.push(seq);
      s.reviewed = Math.max(s.reviewed, seq);
      return json({ reviewed_seq: s.reviewed });
    }
    if (p.endsWith("/library")) {
      s.library++;
      return json({ id: "77777777-7777-4777-8777-777777777777", kind: "document", title: "Trip plan", filename: "Trip plan.md", content_type: "text/markdown", size_bytes: 10, created_at: at }, 201);
    }
    if (p.endsWith("/docs")) {
      s.docsPosted.push(req.postDataJSON());
      return json(mine(DOC, "text", "Trip plan", "Trip plan"), 201);
    }
    if (p.endsWith(`/canvases/${DOC}`)) return json(mine(DOC, "text", "Trip plan", "Trip plan"));
    return json({ canvases: [mine(DOC, "text", "Trip plan", "Trip plan"), mine(MOCKUP, "svg", "Login screen", "Design chat"), mine(OTHER, "text", "Retention policy", "Policy chat")] });
  });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases\//, (route) => {
    const req = route.request();
    const u = new URL(req.url());
    const p = u.pathname;
    const json = (data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    if (p.endsWith("/comments/send")) {
      s.sent.push(req.postDataJSON());
      return route.fulfill({ status: 204 });
    }
    if (p.endsWith("/comments")) {
      if (req.method() === "POST") {
        const c = { id: `c${s.comments.length + 1}`, author_type: "user", anchor: {}, target_lost: false, status: "draft", created_at: at, ...req.postDataJSON() };
        s.comments.push(c);
        return json(c, 201);
      }
      return json({ comments: [] });
    }
    if (p.endsWith("/versions")) {
      const body = req.postDataJSON();
      s.saves.push(body);
      if (body.base_seq !== head().seq) return json({ code: "conflict", message: `this canvas is at version ${head().seq}, not ${body.base_seq}` }, 409);
      const v = { seq: head().seq + 1, author_type: "user" as const, note: body.note ?? "", body: body.body };
      s.versions.push(v);
      return json({ seq: v.seq, note: v.note, author_type: "user", created_at: at }, 201);
    }
    const want = Number(u.searchParams.get("seq")) || head().seq;
    const v = s.versions.find((x) => x.seq === want)!;
    return json({ id: DOC, kind: "text", title: "Trip plan", seq: head().seq, viewing_seq: v.seq, created_at: at, updated_at: at, body: v.body, versions: [...s.versions].reverse().map((x) => ({ seq: x.seq, note: x.note, author_type: x.author_type, created_at: at })) });
  });
  /** The persona edits the document, as canvas_patch would. */
  const agentEdit = (body: string, note = "Edited") => s.versions.push({ seq: head().seq + 1, author_type: "agent", note, body });
  return { mock, chatId, s, head, agentEdit };
}

const editor = (page: Page) => page.getByTestId("doc-editor");

/**
 * Puts the caret at the end of the document's first paragraph. ProseMirror
 * reads a click's caret from the DOM a moment later; keys pressed sooner land
 * where its caret was, so wait until it has caught up.
 */
async function caretAtParagraphEnd(page: Page) {
  const p = editor(page).locator("p").first();
  await p.click();
  await page.keyboard.press("End");
  const end = await p.evaluate((el) => {
    const view = (el.closest(".ProseMirror") as unknown as { editor: { view: { posAtDOM: (n: Node, o: number) => number } } }).editor.view;
    return view.posAtDOM(el, el.childNodes.length);
  });
  await expect
    .poll(() => editor(page).evaluate((el) => (el as unknown as { editor: { state: { selection: { from: number } } } }).editor.state.selection.from))
    .toBe(end);
}

test("lists documents from the app and from chats, but not mockups", async ({ page }) => {
  await install(page);
  await page.reload();
  const list = page.getByRole("list", { name: "Documents" });
  await expect(list.getByRole("link")).toHaveCount(2);
  await expect(list).toContainText("Trip plan");
  await expect(list).toContainText("Retention policy");
  await expect(list).toContainText("From Policy chat");
  await expect(list).not.toContainText("Login screen");
});

test("a new document opens as a page you type on, beside its chat", async ({ page }) => {
  const { mock, chatId, s } = await install(page);
  await page.reload();
  await page.getByRole("textbox", { name: "New document title" }).fill("Trip plan");
  await page.getByRole("button", { name: "New document" }).click();
  await expect(page).toHaveURL(new RegExp(`/apps/docs/${DOC}$`));
  expect(s.docsPosted).toEqual([{ title: "Trip plan" }]);

  // Formatted, not Markdown source.
  await expect(editor(page).locator("strong")).toHaveText("Lisbon");

  // Typing saves a version once it pauses, from the version it started on.
  await caretAtParagraphEnd(page);
  await page.keyboard.type(" Back on Tuesday.");
  await expect(page.getByTestId("doc-save-state")).toHaveText("Saved · v2", { timeout: 5000 });
  expect(s.saves).toEqual([{ base_seq: 1, body: "# Trip plan\n\nFly to **Lisbon** on Friday. Back on Tuesday.", note: "Edited" }]);

  // The chat beside it is the document's own.
  const chat = page.getByTestId("doc-chat");
  const composer = chat.getByRole("combobox", { name: "Message" });
  await composer.fill("Add a day in Porto");
  await composer.press("Enter");
  await expect.poll(() => mock.posted.filter((x) => x.chat === chatId).map((x) => x.body.body)).toContain("Add a day in Porto");
  await page.getByRole("button", { name: "✦ Hide chat" }).click();
  await expect(chat).toHaveCount(0);
});

test("opening a document saves nothing, even when the editor tidies it", async ({ page }) => {
  // A table at the end gets a paragraph after it in the editor; that isn't an edit.
  const { s } = await install(page, "# Trip plan\n\nFour days in late March, flying in **Friday** morning.\n\n## Days\n\n- Fri: Alfama\n- Sat: Belém\n\n## Bookings\n\n- [x] Flights\n- [ ] Stay\n\n| Day | Plan |\n|---|---|\n| Fri | Alfama |\n");
  await page.goto(`/apps/docs/${DOC}`);
  await expect(editor(page).locator("table")).toContainText("Alfama");
  await editor(page).locator("p").first().click();
  // Past the save delay (1.5s): a negative, so it has to wait.
  await page.waitForTimeout(2000);
  await expect(page.getByTestId("doc-save-state")).toHaveText("Saved · v1");
  expect(s.saves).toEqual([]);
});

test("the persona's edits come back as suggestions to accept or reject", async ({ page }) => {
  const { s, agentEdit } = await install(page, "# Trip plan\n\nFly on Friday.\n\nStay in Alfama.\n");
  agentEdit("# Trip plan\n\nFly on Friday morning.\n\nStay in Alfama.\n\nDay trip to Sintra.\n");
  await page.goto(`/apps/docs/${DOC}`);

  const review = page.getByTestId("doc-review");
  await expect(review).toContainText("Assistant suggested 2 changes");
  const changes = review.getByTestId("doc-change");
  await expect(changes).toHaveCount(2);
  await expect(changes.first()).toContainText("Fly on Friday.");
  await expect(changes.first()).toContainText("Fly on Friday morning.");

  // Keep the first, drop the second: the page goes back to editing on a version that says so.
  await changes.first().getByRole("button", { name: "Accept" }).click();
  await changes.nth(1).getByRole("button", { name: "Reject" }).click();
  await expect(editor(page)).toContainText("Fly on Friday morning.");
  await expect(editor(page)).not.toContainText("Sintra");
  expect(s.saves).toEqual([{ base_seq: 2, body: "# Trip plan\n\nFly on Friday morning.\n\nStay in Alfama.\n", note: "Kept 1 of Assistant's 2 changes, rejected 1" }]);
  expect(s.reviewedPosts).toEqual([]);

  // Accepting everything writes nothing; it records the review instead.
  agentEdit("# Trip plan\n\nFly on Friday morning.\n\nStay in Alfama, near the castle.\n");
  await page.reload();
  await page.getByRole("button", { name: "Accept all" }).click();
  await expect(editor(page)).toContainText("near the castle");
  expect(s.reviewedPosts).toEqual([4]);
  expect(s.saves).toHaveLength(1);
});

test("@persona on a line of its own asks for writing right there", async ({ page }) => {
  const { mock, chatId } = await install(page);
  await page.goto(`/apps/docs/${DOC}`);
  await expect(editor(page)).toContainText("Lisbon");
  await caretAtParagraphEnd(page);
  await page.keyboard.press("Enter");
  await page.keyboard.type("@Assistant add a packing list");
  await page.keyboard.press("Enter");

  await expect.poll(() => mock.posted.filter((x) => x.chat === chatId).map((x) => x.body.body)).toContainEqual(expect.stringContaining("add a packing list"));
  const asked = mock.posted.filter((x) => x.chat === chatId).at(-1)!.body.body as string;
  expect(asked).toContain("right after the passage “Fly to Lisbon on Friday.”");
  // The request line itself doesn't stay in the document.
  await expect(editor(page)).not.toContainText("@Assistant");
});

test("a selection can go to the persona as a comment on that passage", async ({ page }) => {
  const { s } = await install(page);
  await page.goto(`/apps/docs/${DOC}`);
  await editor(page).locator("p", { hasText: "Lisbon" }).evaluate((p) => {
    // Select "Lisbon on Friday" across the bold mark.
    const r = document.createRange();
    r.setStart(p.querySelector("strong")!.firstChild!, 0);
    r.setEnd(p.lastChild!, " on Friday".length);
    getSelection()!.removeAllRanges();
    getSelection()!.addRange(r);
  });
  const menu = page.getByTestId("doc-selection-menu");
  await expect(menu).toBeVisible();
  await menu.getByRole("button", { name: "Shorter" }).click();

  await expect.poll(() => s.sent.length).toBe(1);
  const body = "# Trip plan\n\nFly to **Lisbon** on Friday.\n";
  const c = s.comments[0] as { seq: number; start: number; end: number; body: string };
  expect({ seq: c.seq, quoted: body.slice(c.start, c.end), body: c.body }).toEqual({ seq: 1, quoted: "Lisbon** on Friday", body: "Make this shorter." });
  expect(s.sent[0]).toEqual({ comment_ids: ["c1"], message: "Make this shorter." });
});

test("exports to PDF, Word and Markdown, copies rich text, and saves to the Library", async ({ page, context }) => {
  const { s } = await install(page);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(`/apps/docs/${DOC}`);
  await expect(editor(page)).toContainText("Lisbon");
  const menu = async (name: string) => {
    await page.getByRole("button", { name: "Export" }).click();
    await page.getByRole("menuitem", { name: new RegExp(`^${name}`) }).click();
  };

  for (const [name, file] of [["Word", "Trip plan.docx"], ["Markdown", "Trip plan.md"]] as const) {
    const download = page.waitForEvent("download");
    await menu(name);
    expect((await download).suggestedFilename()).toBe(file);
  }

  const popup = page.waitForEvent("popup");
  await page.addInitScript(() => (window.print = () => {}));
  await menu("PDF");
  const print = await popup;
  await expect(print).toHaveURL(new RegExp(`/print/docs/${DOC}$`));

  await menu("Copy as rich text");
  await expect(page.getByRole("status").filter({ hasText: "Copied with formatting" })).toBeVisible();
  const html = await page.evaluate(async () => {
    const [item] = await navigator.clipboard.read();
    return (await item.getType("text/html")).text();
  });
  expect(html).toContain("<strong>Lisbon</strong>");

  await menu("Save to Library");
  await expect(page.getByRole("status").filter({ hasText: "Saved to your Library" })).toBeVisible();
  expect(s.library).toBe(1);
});

test("Markdown the editor can't keep opens as source instead", async ({ page }) => {
  await install(page, "# Trip plan\n\nA claim.[^1]\n\n[^1]: The source.\n");
  await page.goto(`/apps/docs/${DOC}`);
  await expect(page.getByText("This document has HTML or footnotes the editor can't keep")).toBeVisible();
  await expect(editor(page)).toHaveCount(0);
  await expect(page.getByTestId("doc-page")).toContainText("A claim.");
});
