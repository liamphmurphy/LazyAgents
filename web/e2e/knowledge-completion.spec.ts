import { expect, test, type Page as BrowserPage } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

const ID = "55555555-5555-4555-8555-555555555555";
const pages = [
  { id: "dawn", title: "Dawn", path: "Campaign/Sessions/Dawn" },
  { id: "harbour", title: "Harbour", path: "Campaign/Harbour" },
  { id: "other-dawn", title: "Dawn", path: "Other/Dawn" },
];
const mod = process.platform === "darwin" ? "Meta" : "Control";

async function install(page: BrowserPage, body = "Notes. ", delay = 0) {
  await installChatApi(page);
  const saved: string[] = [];
  const searches: string[] = [];
  const note = { id: ID, title: "Tracker", path: "Campaign/Tracker", audience: "private", body_md: body,
    version_id: "v1", seq: 1, properties: {}, direct: false, paused: false, chat_id: null,
    url: `/knowledge/${ID}`, embeds: [], reviews: [], versions: [] };
  await page.route(/\/api\/v1\/knowledge\//, async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/api/v1/knowledge", "");
    let data: unknown = {};
    if (path === "/suggest") {
      const q = url.searchParams.get("q") ?? "";
      searches.push(q);
      if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
      data = { targets: pages.filter((p) => `${p.title} ${p.path}`.toLowerCase().includes(q.toLowerCase()))
        .map((p) => ({ ...p, kind: "artifact", artifact_type: "note", href: `/knowledge/${p.id}` })) };
    }
    if (path === "/query") data = { rows: [note, ...pages], has_more: false };
    if (path === `/pages/${ID}`) {
      if (route.request().method() === "PUT") {
        const input = route.request().postDataJSON();
        saved.push(input.body_md);
        Object.assign(note, input, { seq: note.seq + 1, version_id: `v${note.seq + 1}` });
      }
      data = note;
    }
    if (path === "/backlinks") data = { linked: [], unlinked: [] };
    if (path === "/resolve") data = { links: [] };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(data) });
  });
  await page.goto(`/knowledge/${ID}`);
  const editor = page.getByTestId("doc-editor");
  await expect(editor).toBeVisible();
  await editor.focus();
  await editor.press(`${mod}+End`);
  return { editor, saved, searches };
}

test("[[ opens above the caret, filters by typed text and inserts the chosen path", async ({ page }) => {
  const { editor, saved } = await install(page);
  await page.keyboard.type("[[");
  const list = page.getByRole("listbox", { name: "Link to note" });
  await expect(list.getByRole("option")).toHaveCount(3);
  const caretTop = await editor.evaluate(() => {
    const selection = window.getSelection()!;
    const range = selection.getRangeAt(0).cloneRange();
    range.setStart(selection.anchorNode!, Math.max(0, selection.anchorOffset - 1));
    return range.getBoundingClientRect().top;
  });
  expect((await list.boundingBox())!.y + (await list.boundingBox())!.height).toBeLessThan(caretTop);
  await page.keyboard.type("Harb");
  await expect(list.getByRole("option")).toHaveCount(1);
  await expect(list.getByRole("option")).toContainText("Harbour");
  await editor.press("Enter");
  await expect(list).toHaveCount(0);
  await page.keyboard.type(" afterwards");
  await expect.poll(() => saved.at(-1)).toBe("Notes. [[Campaign/Harbour]] afterwards");
});

test("arrow keys and Tab choose between matching titles with distinct paths", async ({ page }) => {
  const { editor, saved } = await install(page);
  await page.keyboard.type("[[Dawn");
  const list = page.getByRole("listbox", { name: "Link to note" });
  await expect(list.getByRole("option")).toHaveCount(2);
  await editor.press("ArrowUp");
  await expect(list.getByRole("option").nth(1)).toHaveAttribute("aria-selected", "true");
  await editor.press("Tab");
  await expect.poll(() => saved.at(-1)).toBe("Notes. [[Other/Dawn]]");
  await expect(editor).toBeFocused();
});

test("Escape dismisses without changing the text and a new [[ can reopen", async ({ page }) => {
  const { editor } = await install(page);
  await page.keyboard.type("[[Dawn");
  const list = page.getByRole("listbox", { name: "Link to note" });
  await expect(list).toBeVisible();
  await editor.press("Escape");
  await page.keyboard.type(" more");
  await expect(list).toHaveCount(0);
  await expect(editor).toContainText("[[Dawn more");
  await page.keyboard.type("]] [[Harb");
  await expect(list.getByRole("option")).toHaveCount(1);
  await list.getByRole("option").click();
  await expect(list).toHaveCount(0);
  await expect(editor).toContainText("[[Campaign/Harbour]]");
});

test("code, closed links and a blurred editor do not keep the picker open", async ({ page }) => {
  const { editor, searches } = await install(page, "A `[[Dawn` example.");
  await editor.locator("code").click();
  await editor.press("ArrowLeft");
  await page.keyboard.type("x");
  await expect(page.getByRole("listbox", { name: "Link to note" })).toHaveCount(0);
  expect(searches).toEqual([]);
  await editor.press(`${mod}+End`);
  await page.keyboard.type(" [[Dawn]]");
  await expect(page.getByRole("listbox", { name: "Link to note" })).toHaveCount(0);
  await page.keyboard.type(" [[Harb");
  await expect(page.getByRole("listbox", { name: "Link to note" })).toBeVisible();
  await page.getByRole("textbox", { name: "Title", exact: true }).focus();
  await expect(page.getByRole("listbox", { name: "Link to note" })).toHaveCount(0);
});

test("loading results cannot insert a stale choice or create a paragraph on Enter", async ({ page }) => {
  const { editor, saved } = await install(page, "Notes. ", 350);
  await page.keyboard.type("[[Dawn");
  const list = page.getByRole("listbox", { name: "Link to note" });
  await expect(list.getByRole("option")).toHaveCount(2);
  await editor.press("Backspace");
  await editor.press("Backspace");
  await editor.press("Backspace");
  await editor.press("Backspace");
  await page.keyboard.type("Harb");
  await editor.press("Enter");
  await expect(editor.locator("p")).toHaveCount(1);
  await expect(list.getByRole("option")).toHaveCount(1);
  await editor.press("Enter");
  await expect.poll(() => saved.at(-1)).toBe("Notes. [[Campaign/Harbour]]");
});

test("completion replaces existing closing brackets and leaves the following text", async ({ page }) => {
  const { editor, saved } = await install(page, "Notes. [[]] tail");
  for (let i = 0; i < 7; i++) await editor.press("ArrowLeft");
  await page.keyboard.type("Harb");
  const list = page.getByRole("listbox", { name: "Link to note" });
  await expect(list.getByRole("option")).toHaveCount(1);
  await editor.press("Enter");
  await expect.poll(() => saved.at(-1)).toBe("Notes. [[Campaign/Harbour]] tail");
});

test("no matches leaves literal note text editable and Enter can continue writing", async ({ page }) => {
  const { editor } = await install(page);
  await page.keyboard.type("[[Missing");
  await expect(page.getByRole("listbox", { name: "Link to note" })).toContainText("No matching notes");
  await editor.press("Enter");
  await expect(editor.locator("p")).toHaveCount(2);
  await expect(page.getByRole("listbox", { name: "Link to note" })).toHaveCount(0);
});
