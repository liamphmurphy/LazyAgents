import { expect, test } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

test.use({ viewport: { width: 1440, height: 900 } });

const P = "55555555-5555-4555-8555-555555555555";

for (const path of ["/knowledge", `/knowledge/${P}`]) {
  test(`the chat on ${path} stays available in the app and is created outside normal history`, async ({ page }) => {
    const api = await installChatApi(page, { replyDelayMs: 10 });
    const document = { id: P, title: "Reading list", path: "Reading list", audience: "private", body_md: "Notes", version_id: "v1", seq: 1, properties: {}, direct: false, paused: false, chat_id: null as string | null, url: `/knowledge/${P}`, embeds: [], reviews: [], versions: [] };
    await page.route(/\/api\/v1\/knowledge\//, (route) => {
      const request = route.request();
      const endpoint = new URL(request.url()).pathname.replace("/api/v1/knowledge", "");
      if (endpoint === `/pages/${P}/chat`) document.chat_id = request.postDataJSON().chat_id;
      const body = endpoint === "/query" ? { columns: [], rows: [], has_more: false }
        : endpoint === "/backlinks" ? { linked: [], unlinked: [] }
        : endpoint.startsWith(`/pages/${P}`) ? document : { nodes: [], edges: [] };
      return route.fulfill({ contentType: "application/json", body: JSON.stringify(body) });
    });
    await page.goto(path);
    await expect(page.getByRole("tablist", { name: "Open pages" }).getByRole("tab", { name: /Reading list|New tab/ })).toBeVisible();
    await page.getByRole("navigation", { name: "Panel rail" }).getByRole("button", { name: "Open Chat", exact: true }).click();
    const dock = page.getByRole("complementary", { name: "Panel", exact: true });
    await dock.getByRole("button", { name: /^Start a chat/ }).click();
    await expect(dock.getByRole("combobox", { name: "Message" })).toBeVisible();
    expect(api.created).toHaveLength(1);
    expect(api.created[0].hidden).toBe(true);

    // Opening the app again resumes the same chat by id.
    await page.reload();
    await expect(dock.getByRole("combobox", { name: "Message" })).toBeVisible();
    expect(api.created).toHaveLength(1);
  });
}
