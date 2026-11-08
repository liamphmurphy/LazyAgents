import { expect, test, type Page } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// A ```canvas fence opens a canvas beside the chat, without running its content.

const DOC = "11111111-1111-4111-8111-111111111111";
const SVG = "22222222-2222-4222-8222-222222222222";
const at = "2026-11-08T12:00:00Z";
const summary = (id: string, kind: string, title: string, seq: number) => ({ id, kind, title, seq, created_at: at, updated_at: at });
const versions = (n: number) => Array.from({ length: n }, (_, i) => ({ seq: n - i, note: `Note ${n - i}`, author_type: "agent", created_at: at }));
const svgBody = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20" onload="document.title='ran'"><script>document.title="ran"</script><rect data-vid="abc" width="40" height="20" fill="#36c"/></svg>`;

async function open(page: Page) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases(\/|\?|$)/, (route) => {
    const url = new URL(route.request().url());
    const seq = Number(url.searchParams.get("seq") ?? 2);
    const json = (body: unknown) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
    if (url.pathname.endsWith("/canvases")) return json({ canvases: [summary(DOC, "text", "Retention policy", 2), summary(SVG, "svg", "Run screen", 1)] });
    if (url.pathname.endsWith(DOC)) return json({ ...summary(DOC, "text", "Retention policy", 2), viewing_seq: seq, body: `# Policy\n\nArchive after ${seq * 30} days.`, versions: versions(2) });
    return json({ ...summary(SVG, "svg", "Run screen", 1), viewing_seq: 1, body: svgBody, versions: versions(1) });
  });
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill(`canvas: ${DOC}`);
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
}

test("a card opens the canvas and versions can be stepped through", async ({ page }) => {
  await open(page);
  const card = page.getByTestId("canvas-card");
  await expect(card).toContainText("Retention policy");
  await expect(card).toContainText("Document · v2");
  await card.click();

  const pane = page.getByTestId("canvas-panel");
  await expect(pane).toContainText("Archive after 60 days.");
  await expect(pane.getByTestId("canvas-version")).toHaveText("v2 of 2");
  await expect(pane).toContainText("Note 2");
  await expect(pane.getByRole("button", { name: "Next version" })).toBeDisabled();

  await pane.getByRole("button", { name: "Previous version" }).click();
  await expect(pane).toContainText("Archive after 30 days.");
  await expect(pane.getByTestId("canvas-version")).toHaveText("v1 of 2");

  await pane.getByRole("tab", { name: "Source" }).click();
  await expect(pane).toContainText("# Policy");

  await pane.getByRole("button", { name: "Close canvas" }).click();
  await expect(pane).toHaveCount(0);
});

test("an SVG canvas draws sanitized elements that keep their ids", async ({ page }) => {
  await open(page);
  await page.getByRole("button", { name: "Canvas, 2 open" }).click();
  const pane = page.getByTestId("canvas-panel");
  // Opened from the header: the newest.
  await expect(pane.getByTestId("canvas-svg").locator('rect[data-vid="abc"]')).toBeVisible();
  await expect(pane.getByTestId("canvas-svg").locator("script")).toHaveCount(0);
  await expect(pane).toContainText("can't be shown here");
  expect(await page.title()).not.toBe("ran");

  await pane.getByRole("combobox", { name: "Canvas" }).selectOption(DOC);
  await expect(pane).toContainText("Archive after 60 days.");
});

test("one right-hand panel at a time", async ({ page }) => {
  await open(page);
  await page.getByTestId("canvas-card").click();
  await expect(page.getByTestId("canvas-panel")).toBeVisible();
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Memory", exact: true }).click();
  await expect(page.getByTestId("canvas-panel")).toHaveCount(0);
  await page.getByTestId("canvas-card").click();
  await expect(page.getByTestId("canvas-panel")).toBeVisible();
  await expect(page).not.toHaveURL(/memory=/);
});

test("a failing canvas request doesn't hold up the composer", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases$/, (route) => route.fulfill({ status: 500, contentType: "application/json", body: '{"code":"internal","message":"boom"}' }));
  await page.goto("/chat?project=WEB");
  const composer = page.getByRole("combobox", { name: "Message" }).or(page.getByRole("textbox", { name: "Message" })).first();
  await composer.fill("hello");
  await composer.press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText("Echo: hello");
  await expect(composer).toBeEnabled({ timeout: 1500 });
});

test("a fence naming an unknown canvas stays code", async ({ page }) => {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.route(/\/api\/v1\/chats\/[^/]+\/canvases$/, (route) => route.fulfill({ status: 200, contentType: "application/json", body: '{"canvases":[]}' }));
  await page.goto("/chat?project=WEB");
  await page.getByRole("textbox", { name: "Message" }).fill(`canvas: ${DOC}`);
  await page.getByRole("textbox", { name: "Message" }).press("Enter");
  await expect(page.getByTestId("assistant-message")).toContainText(DOC);
  await expect(page.getByTestId("canvas-card")).toHaveCount(0);
});
