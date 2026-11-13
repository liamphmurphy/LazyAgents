import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The persona chat dock on every Knowledge screen: folded to a rail by default, on the shared dock.
test.use({ viewport: { width: 1440, height: 900 } });

const P = "55555555-5555-4555-8555-555555555555";
const json = (route: Route, data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });

async function install(page: Page, pageCount = 0) {
  await installChatApi(page, { replyDelayMs: 10 });
  const rows = Array.from({ length: pageCount }, (_, i) => ({ id: `55555555-5555-4555-8555-${String(i).padStart(12, "0")}`, title: `Note ${i}`, path: `Note ${i}`, audience: "private", properties: {}, updated_at: "2026-11-13T09:00:00Z" }));
  const s = { attached: [] as string[], page: { id: P, title: "Reading list", path: "Reading list", audience: "private", body_md: "Notes", version_id: "v1", seq: 1, properties: {}, direct: false, paused: false, chat_id: null as string | null, url: `/knowledge/${P}`, embeds: [], reviews: [], versions: [] } };
  await page.route(/\/api\/v1\/knowledge\//, (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace("/api/v1/knowledge", "");
    if (path === "/query") {
      const params = new URL(req.url()).searchParams;
      const found = params.has("folder") ? [] : rows.slice(0, Number(params.get("limit") ?? 200));
      return json(route, { columns: [], rows: found, total: found.length, has_more: false });
    }
    if (path === `/pages/${P}/chat` && req.method() === "PUT") {
      s.page.chat_id = (req.postDataJSON() as { chat_id: string }).chat_id;
      s.attached.push(s.page.chat_id);
      return json(route, s.page);
    }
    if (path === `/pages/${P}`) return json(route, s.page);
    if (path === "/network" || path === "/graph") return json(route, { nodes: [], edges: [] });
    if (path === "/backlinks") return json(route, { linked: [], unlinked: [] });
    return json(route, {});
  });
  return s;
}

const dock = (page: Page) => page.getByRole("complementary", { name: "Panel", exact: true });
const rail = (page: Page) => page.getByRole("navigation", { name: "Panel rail" });
const mod = process.platform === "darwin" ? "Meta" : "Control";

test("the chat starts folded to a rail on Home, opens from it, and a general chat starts there", async ({ page }) => {
  await install(page);
  await page.goto("/knowledge");
  await expect(rail(page)).toBeVisible();
  await expect(dock(page)).toHaveCount(0);

  await rail(page).getByRole("button", { name: /^Open Chat/ }).click();
  await expect(dock(page).getByRole("tab", { name: /Chat/ })).toHaveAttribute("aria-selected", "true");
  await dock(page).getByRole("button", { name: "Start a chat", exact: true }).click();
  await expect(dock(page).getByRole("combobox", { name: "Message" })).toBeVisible();

  // The choice is remembered: open after a reload, with the same chat.
  await page.reload();
  await expect(dock(page)).toBeVisible();
  await expect(dock(page).getByRole("combobox", { name: "Message" })).toBeVisible();

  // ⌘\ folds it again.
  await page.locator("body").click({ position: { x: 700, y: 400 } });
  await page.keyboard.press(`${mod}+Backslash`);
  await expect(rail(page)).toBeVisible();
});

test("on a page the dock holds that page's chat, and ⌘J opens it, with the page's own sidebar free of a Chat tab", async ({ page }) => {
  const s = await install(page);
  await page.goto(`/knowledge/${P}`);
  await expect(dock(page)).toHaveCount(0);
  await expect(rail(page)).toBeVisible();
  // The page's own sidebar no longer carries a Chat tab.
  await expect(page.getByRole("complementary", { name: "Page sidebar" }).getByRole("tab", { name: "Chat" })).toHaveCount(0);

  await page.keyboard.press(`${mod}+j`);
  await expect(dock(page)).toBeVisible();
  await dock(page).getByRole("button", { name: "Start a chat about this page" }).click();
  await expect.poll(() => s.attached.length).toBe(1);
  await expect(dock(page).getByRole("combobox", { name: "Message" })).toBeVisible();
});

for (const route of ["/knowledge", `/knowledge/${P}`]) {
  test(`the chat stays inside the viewport with a long explorer and conversation on ${route}`, async ({ page }) => {
    await install(page, 80);
    await page.goto(route);
    await rail(page).getByRole("button", { name: /^Open Chat/ }).click();
    await dock(page).getByRole("button", { name: /^Start a chat/ }).click();
    const composer = dock(page).getByRole("combobox", { name: "Message" });
    await expect(composer).toBeVisible();
    await composer.fill("A long Knowledge conversation. ".repeat(100));
    await composer.press("Enter");
    await expect(dock(page).getByRole("button", { name: "Send", exact: true })).toBeVisible();

    for (const viewport of [{ width: 1440, height: 900 }, { width: 1100, height: 650 }]) {
      await page.setViewportSize(viewport);
      // Visibility alone allows off-screen elements; check the full composer and dock bounds.
      for (const element of [dock(page), composer]) {
        await expect.poll(async () => {
          const box = await element.boundingBox();
          return !!box && box.y >= 0 && box.y + box.height <= viewport.height + 1;
        }).toBe(true);
      }
      expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBeLessThanOrEqual(viewport.height + 1);
      const explorer = page.getByRole("navigation", { name: "Pages", exact: true });
      await explorer.evaluate((el) => { el.scrollTop = el.scrollHeight; });
      expect(await explorer.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
      const history = dock(page).getByRole("region").getByRole("tabpanel", { name: "Chat", exact: true });
      expect(await history.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
    }
  });
}
