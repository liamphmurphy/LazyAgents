import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The Docs app: documents (text canvases) listed full page, opened beside their chat.

const DOC = "44444444-4444-4444-8444-444444444444";
const OTHER = "55555555-5555-4555-8555-555555555555";
const MOCKUP = "66666666-6666-4666-8666-666666666666";
const at = "2026-11-10T12:00:00Z";

async function install(page: Page) {
  const mock = await installChatApi(page, { replyDelayMs: 10 });
  // A real chat in the mock, so the drawer's conversation can load and send.
  await page.goto("/apps/docs");
  const chat = await page.evaluate(() => fetch("/api/v1/chats", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).then((r) => r.json()));
  const chatId = chat.id as string;
  const docsPosted: Record<string, unknown>[] = [];
  const mine = (id: string, kind: string, title: string, chatTitle: string) => ({ id, kind, title, chat_id: chatId, chat_title: chatTitle, updated_at: at });
  await page.route(/\/api\/v1\/(canvases|docs)(\/|$)/, (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname;
    const json = (data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    if (p.endsWith("/docs")) {
      docsPosted.push(req.postDataJSON());
      return json(mine(DOC, "text", "Trip plan", "Trip plan"), 201);
    }
    if (p.endsWith(`/canvases/${DOC}`)) return json(mine(DOC, "text", "Trip plan", "Trip plan"));
    return json({ canvases: [mine(DOC, "text", "Trip plan", "Trip plan"), mine(MOCKUP, "svg", "Login screen", "Design chat"), mine(OTHER, "text", "Retention policy", "Policy chat")] });
  });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases\//, (route) => {
    const p = new URL(route.request().url()).pathname;
    const json = (data: unknown) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(data) });
    if (p.endsWith("/comments")) return json({ comments: [] });
    return json({ id: DOC, kind: "text", title: "Trip plan", seq: 2, viewing_seq: 2, created_at: at, updated_at: at, body: "# Trip plan\n\nFly to **Lisbon** on Friday.", versions: [{ seq: 2, note: "Added flights", author_type: "agent", created_at: at }, { seq: 1, note: "", author_type: "user", created_at: at }] });
  });
  return { mock, chatId, docsPosted };
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

test("a new document opens full page beside its chat", async ({ page }) => {
  const { mock, chatId, docsPosted } = await install(page);
  await page.reload();
  await page.getByRole("textbox", { name: "New document title" }).fill("Trip plan");
  await page.getByRole("button", { name: "New document" }).click();
  await expect(page).toHaveURL(new RegExp(`/apps/docs/${DOC}$`));
  expect(docsPosted).toEqual([{ title: "Trip plan" }]);

  const doc = page.getByTestId("doc-page");
  await expect(doc).toContainText("Fly to Lisbon on Friday.");
  await expect(doc.getByTestId("canvas-version")).toHaveText("v2 of 2");

  // The drawer is the document's own chat: what's typed there goes to it.
  const drawer = page.getByRole("dialog", { name: "Ask about this document" });
  await expect(drawer).toBeVisible();
  const composer = drawer.getByRole("combobox", { name: "Message" });
  await composer.fill("Add a day in Porto");
  await composer.press("Enter");
  await expect.poll(() => mock.posted.filter((x) => x.chat === chatId).map((x) => x.body.body)).toContain("Add a day in Porto");

  // Closing it gives the page its width back; Ask brings it back.
  await drawer.getByRole("button", { name: "Close" }).click();
  await expect(drawer).toHaveCount(0);
  await page.getByRole("button", { name: "✦ Ask" }).click();
  await expect(page.getByRole("dialog", { name: "Ask about this document" })).toBeVisible();
});

test("exports Markdown as a file and PDF through the print view", async ({ page }) => {
  await install(page);
  await page.goto(`/apps/docs/${DOC}`);
  await expect(page.getByTestId("doc-page")).toContainText("Lisbon");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Markdown" }).click();
  expect((await download).suggestedFilename()).toBe("Trip plan.md");
  await expect(page.getByRole("link", { name: "PDF" })).toHaveAttribute("href", `/print/docs/${DOC}`);

  await page.addInitScript(() => (window.print = () => {}));
  await page.goto(`/print/docs/${DOC}`);
  await expect(page.getByTestId("print-doc")).toContainText("Fly to Lisbon on Friday.");
});
