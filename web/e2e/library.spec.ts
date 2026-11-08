import { expect, test, type Page } from "@playwright/test";
import type { components } from "../lib/api/schema";
import { installBoardApi } from "./mock-board-api";

type Item = components["schemas"]["LibraryItem"];

// The Library: generated images saved from Assistant chats, private to the person.
const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=", "base64");

async function installLibrary(page: Page, items: Item[]) {
  await installBoardApi(page);
  const deleted: string[] = [];
  // Registered after the board mock, so these win for /library.
  await page.route("**/api/v1/library**", async (route) => {
    const url = new URL(route.request().url());
    const m = url.pathname.match(/\/library\/([^/]+)(\/content)?$/);
    if (route.request().method() === "DELETE" && m) {
      deleted.push(m[1]);
      items.splice(items.findIndex((i) => i.id === m[1]), 1);
      return route.fulfill({ status: 204 });
    }
    if (m?.[2]) return route.fulfill({ status: 200, contentType: "image/png", body: png });
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items, has_more: false }) });
  });
  return { deleted };
}

const item = (id: string, over: Partial<Item> = {}): Item => ({
  id, kind: "image", title: "Dragon lair", filename: `${id}.png`, content_type: "image/png", size_bytes: 2048, created_at: new Date().toISOString(), ...over,
});

test("the Library lists generated images with their source and can remove one", async ({ page }) => {
  const api = await installLibrary(page, [
    item("img-1", { chat_id: "chat-1", chat_title: "Dragon lair", project_key: "WEB", persona_id: "p-1", persona_name: "Gwen" }),
    item("img-2", { title: "Old map" }),
  ]);
  await page.goto("/library");
  // The menu starts as an icon rail: links are named, not labelled with text.
  await expect(page.getByRole("navigation", { name: "Primary" }).locator('[aria-current="page"]')).toHaveAccessibleName("Library");
  const tiles = page.getByTestId("library-item");
  await expect(tiles).toHaveCount(2);
  await expect(tiles.nth(0)).toContainText("Gwen");
  await expect(tiles.nth(0).getByRole("link", { name: "Dragon lair" })).toHaveAttribute("href", "/chat/chat-1?project=WEB");
  await expect(tiles.nth(1)).toContainText("chat deleted");

  await tiles.nth(1).getByRole("button", { name: "Open Old map" }).click();
  const dialog = page.getByRole("dialog", { name: "Old map" });
  await expect(dialog.getByRole("link", { name: "Download" })).toHaveAttribute("href", "/api/v1/library/img-2/content?download=true");
  page.once("dialog", (d) => d.accept());
  await dialog.getByRole("button", { name: "Remove" }).click();
  await expect(tiles).toHaveCount(1);
  expect(api.deleted).toEqual(["img-2"]);
});

test("saved diagrams have their own tab and can't be references", async ({ page }) => {
  await installLibrary(page, [item("dia-1", { kind: "diagram", title: "Auth flow", filename: "diagram.svg", content_type: "image/svg+xml", chat_id: "chat-1", chat_title: "Auth flow", project_key: "WEB" })]);
  await page.goto("/library");
  await page.getByRole("tab", { name: "Diagrams" }).click();
  await page.getByRole("button", { name: "Open Auth flow" }).click();
  const dialog = page.getByRole("dialog", { name: "Auth flow" });
  await expect(dialog.getByRole("link", { name: "Download" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Use as reference" })).toHaveCount(0);
});

test("an empty Library explains how images get there", async ({ page }) => {
  await installLibrary(page, []);
  await page.goto("/library");
  await expect(page.getByRole("heading", { name: "Nothing here yet" })).toBeVisible();
});
