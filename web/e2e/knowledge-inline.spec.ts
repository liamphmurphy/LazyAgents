import { expect, test, type Page as BrowserPage } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";
import type { Embed, Page } from "../lib/api/knowledge-hooks";

const ID = "55555555-5555-4555-8555-555555555555";
const image = "![[attachment:88888888-8888-4888-8888-888888888888/map.png]]";
const mod = process.platform === "darwin" ? "Meta" : "Control";

async function install(page: BrowserPage, body: string, embeds: Embed[] = []) {
  await installChatApi(page);
  const saved: string[] = [];
  const note: Page = { id: ID, title: "Open Threads", path: "Campaign/Open Threads", audience: "private", body_md: body,
    version_id: "v1", seq: 1, properties: {}, direct: false, paused: false, chat_id: null,
    url: `/knowledge/${ID}`, embeds: [], reviews: [], versions: [] };
  const refresh = () => {
    note.embeds = [...embeds, ...(note.body_md?.includes(image) ? [{ ref: image, kind: "file" as const, title: "map.png", available: true,
      content_type: "image/png", url: "/inline-map.png" }] : [])];
  };
  refresh();
  await page.route("**/inline-map.png", (route) => route.fulfill({ contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=", "base64") }));
  await page.route(/\/api\/v1\/knowledge\//, (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1/knowledge", "");
    let data: unknown = {};
    if (path === "/query") data = { rows: [note, { ...note, id: "other", title: "Session 27", path: "Campaign/Sessions/Session 27" }], has_more: false };
    if (path === `/pages/${ID}`) {
      if (route.request().method() === "PUT") {
        const input = route.request().postDataJSON();
        saved.push(input.body_md);
        Object.assign(note, input, { seq: note.seq + 1, version_id: `v${note.seq + 1}` });
        refresh();
      }
      data = note;
    }
    if (path === "/backlinks") data = { linked: [], unlinked: [] };
    if (path === "/resolve") data = { links: [] };
    return route.fulfill({ contentType: "application/json", body: JSON.stringify(data) });
  });
  await page.goto(`/knowledge/${ID}`);
  await expect(page.getByTestId("doc-editor")).toBeVisible();
  return { saved };
}

test("links show note titles and aliases while preserving their source when edited", async ({ page }) => {
  const body = "Before [[Campaign/Sessions/Session 27]] and [[Campaign/Sessions/Session 27#Summary|Recap]]. After.";
  const { saved } = await install(page, body);
  const editor = page.getByTestId("doc-editor");
  await expect(editor).toHaveText("Before Session 27 and Recap. After.", { useInnerText: true });
  await editor.getByText("Session 27", { exact: true }).click();
  await expect(editor).toContainText("[[Campaign/Sessions/Session 27]]", { useInnerText: true });
  await editor.press(`${mod}+End`);
  await editor.press("End");
  await page.keyboard.type(" Updated.");
  await page.getByRole("textbox", { name: "Title", exact: true }).focus();
  await expect.poll(() => saved.at(-1)).toBe(`${body} Updated.`);
  await expect(editor).toHaveText("Before Session 27 and Recap. After. Updated.", { useInnerText: true });
  await editor.getByText("Session 27", { exact: true }).click({ modifiers: [mod] });
  await expect(page).toHaveURL(/\/knowledge\/other\/session-27$/);
});

test("image embeds render between their surrounding paragraphs and disappear when removed", async ({ page }) => {
  const { saved } = await install(page, `Before the map.\n\n${image}\n\nAfter the map.`);
  const editor = page.getByTestId("doc-editor");
  await expect(editor.getByRole("img", { name: "map.png" })).toBeVisible();
  await expect(editor.locator("p")).toHaveCount(3);
  await expect(editor.locator("p").nth(1).getByRole("img")).toBeVisible();
  await expect(page.getByRole("region", { name: "Embeds", exact: true })).toHaveCount(0);
  await editor.fill("The map was removed.");
  await expect(editor.getByRole("img")).toHaveCount(0);
  await expect.poll(() => saved.at(-1)).toBe("The map was removed.");
});

test("note previews retain Markdown rendering and app context in their inline position", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await install(page, "Before.\n\n![[Excerpt]]\n\nAfter.", [{ ref: "![[Excerpt]]", kind: "page", title: "Excerpt", available: true,
    markdown: "A **bold** note with [[Campaign/Sessions/Session 27]]." }]);
  const editor = page.getByTestId("doc-editor");
  await expect(editor.locator("figure strong")).toHaveText("bold");
  await expect(editor.locator("figure")).toContainText("Session 27");
  expect(errors).toEqual([]);
});

test("an image added while editing appears inline after autosave resolves it", async ({ page }) => {
  const { saved } = await install(page, "Before.");
  const editor = page.getByTestId("doc-editor");
  await editor.focus();
  await editor.press(`${mod}+End`);
  await page.keyboard.type(` ${image} After.`);
  await expect.poll(() => saved.at(-1)).toBe(`Before. ${image} After.`);
  await expect(editor.getByRole("img", { name: "map.png" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Embeds", exact: true })).toHaveCount(0);
});
