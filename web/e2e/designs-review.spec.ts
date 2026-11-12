import { expect, test } from "@playwright/test";

// Reviewing a persona's design suggestions (LAZYAGENTS-148 slice C, ADR-designs §3.7)
// on the real stack with the fake harness in design-review mode (see the header of
// server/cmd/fake-harness/main.go). It needs its own runner script, so e2e-stack/run.sh
// runs it in a second pass (`E2E_FAKE_HARNESS_MODE=design-review npm run e2e:stack -- designs-review`
// runs just this), and the mocked suite (playwright.config.ts) leaves it out.

const API = process.env.E2E_STACK_API_URL;
const ON = process.env.E2E_FAKE_HARNESS_MODE === "design-review" && !!API;

type Node = { id: string; type: string; name?: string; x: number; opacity?: number; fill?: string; children?: Node[] };
type Body = { pages: { children: Node[] }[] };
const walk = (n: Node, f: (n: Node) => void) => (f(n), n.children?.forEach((c) => walk(c, f)));
const nodes = (b: Body) => {
  const all: Node[] = [];
  b.pages.forEach((p) => p.children.forEach((n) => walk(n, (x) => all.push(x))));
  return all;
};

test.describe("@design-review", () => {
  test.skip(!ON, "needs the stack with E2E_FAKE_HARNESS_MODE=design-review");

  test("suggestions are reviewed per change, direct edit applies and undoes, a selection reaches the persona", async ({ page, request }) => {
    test.setTimeout(240_000);
    const get = async (path: string) => {
      const res = await request.get(`${API}/api/v1${path}`);
      expect(res.ok(), `${path}: ${res.status()}`).toBeTruthy();
      return res.json();
    };
    const head = async (id: string): Promise<{ seq: number; body: Body }> => {
      const { seq } = await get(`/designs/${id}`);
      return { seq, body: JSON.parse((await get(`/designs/${id}/versions/${seq}`)).body) };
    };
    const made = await request.post(`${API}/api/v1/designs`, { data: { title: "Review e2e", starter: "mobile-flow" } });
    expect(made.ok()).toBeTruthy();
    const { id } = await made.json();
    const first = await head(id);
    const all = nodes(first.body);
    const frame = first.body.pages[0].children.find((n) => n.type === "frame")!;
    const texts = frame.children!.filter((n) => n.type === "text");
    const button = frame.children!.find((n) => n.type === "shape" && n.name === "Button")!;
    const kept = texts[0], dropped = texts[1];
    expect(all.length).toBeGreaterThan(3);

    await page.goto(`/apps/designs/${id}`);
    await expect(page.getByTestId("design-canvas")).toBeVisible();
    const chatBox = page.locator("[data-design-chat] textarea");
    const say = async (text: string) => {
      await chatBox.fill(text);
      await chatBox.press("Enter");
    };
    const chat = page.locator("[data-design-chat]");
    const layer = (n: Node) => page.getByRole("treeitem", { name: n.name ?? "", exact: true }).first();
    const pick = async (n: Node) => {
      await layer(n).click({ position: { x: 70, y: 8 } });
      await expect(layer(n)).toHaveAttribute("aria-selected", "true");
    };

    // 1. The persona's patch waits as a suggestion; the design is untouched.
    await say("Tidy the Start screen");
    const open = page.getByRole("button", { name: "Review 1 suggestion" }).first();
    await expect(open).toBeVisible({ timeout: 120_000 });
    await expect(chat.getByText("I suggested 3 changes.")).toBeVisible();
    expect((await get(`/designs/${id}`)).seq).toBe(first.seq);
    expect((await get(`/designs/${id}`)).pending_proposals).toBe(1);

    // 2. An unsaved edit of mine (nudge a layer) is in flight while I review.
    await pick(kept);
    await page.keyboard.press("Shift+ArrowRight");
    await open.click();
    const panel = page.getByTestId("review-panel");
    await expect(panel.getByTestId("review-summary")).toHaveText("Tidy the Start screen");
    for (const [n, why, kind] of [[1, "Add a promo code field", "added"], [2, "Make the button stand out", "changed"], [3, "Remove the placeholder copy", "removed"]] as const) {
      const card = panel.getByTestId(`review-change-${n}`);
      await expect(card.getByTestId("change-why")).toHaveText(why);
      await expect(page.locator(`[data-testid="review-overlay"] [data-pin="${n}"]`)).toHaveAttribute("data-kind", kind);
    }
    await expect(panel.getByTestId("review-change-1")).toContainText("Promo code");

    // Before / After / Diff toggles the preview; After shows the new field on the canvas.
    const canvasText = page.getByTestId("design-canvas");
    await page.getByRole("button", { name: "After", exact: true }).click();
    await expect(canvasText.getByText("Promo code")).toBeVisible();
    await page.getByRole("button", { name: "Before", exact: true }).click();
    await expect(canvasText.getByText("Promo code")).toHaveCount(0);
    await page.getByRole("button", { name: "Diff", exact: true }).click();

    // 3. Accept one, reject one with a reason, accept the rest.
    await panel.getByTestId("review-change-1").getByRole("button", { name: "Accept", exact: true }).click();
    await panel.getByTestId("review-change-3").getByRole("button", { name: "Reject", exact: true }).click();
    await page.getByLabel(/Why reject change 3/).fill("Keep the placeholder copy");
    await page.getByLabel(/Why reject change 3/).press("Enter");
    await expect(panel.getByTestId("review-change-3")).toContainText("Rejected: Keep the placeholder copy");
    await expect(page.getByRole("button", { name: "Done" })).toBeDisabled();
    await page.getByRole("button", { name: "Accept rest" }).click();
    await expect(panel.getByTestId("review-change-2")).toContainText("Accepted");
    await expect(page.getByTestId("review-status")).toContainText("0 of 3 left");

    // Nothing changed until Done (my own nudge may have saved by now, but not the suggestion).
    const before = nodes((await head(id)).body);
    expect(before.some((n) => n.name === "Promo code")).toBe(false);
    expect(before.find((n) => n.id === button.id)?.fill).toBe(button.fill);
    expect(before.some((n) => n.id === dropped.id)).toBe(true);

    // 4. Done: one version with the accepted changes on top of my unsaved edit.
    await page.getByRole("button", { name: "Done" }).click();
    await expect.poll(async () => nodes((await head(id)).body).some((n) => n.name === "Promo code"), { timeout: 30_000 }).toBe(true);
    const after = nodes((await head(id)).body);
    expect(after.find((n) => n.id === button.id)?.fill).toBe("#2F6BFF");
    expect(after.some((n) => n.id === dropped.id)).toBe(true);
    expect(after.find((n) => n.id === kept.id)?.x).toBe(kept.x + 10);
    await expect(page.getByTestId("review-panel")).toHaveCount(0);
    await expect(page.getByTestId("design-save-state")).toContainText("Saved");

    // 5. The persona hears the reason; the notice isn't a canvas card.
    await expect(chat.getByTestId("system-message").filter({ hasText: "Review of your design suggestion" })).toBeVisible({ timeout: 30_000 });
    await expect(chat.getByText(/Noted:.*Keep the placeholder copy/)).toBeVisible({ timeout: 120_000 });

    // 6. Direct edit on: a patch applies at once as one undoable step, and the selection rides along.
    const direct = page.getByRole("switch", { name: "Let personas edit directly" });
    await direct.check();
    await expect.poll(async () => (await get(`/designs/${id}`)).settings.direct_edit).toBe(true);
    const seqBefore = (await head(id)).seq;
    await pick(kept);
    await say("Fade this");
    await expect(chat.getByText(`You selected ${kept.id}`)).toBeVisible({ timeout: 120_000 });
    await expect.poll(async () => nodes((await head(id)).body).find((n) => n.id === kept.id)?.opacity, { timeout: 30_000 }).toBe(0.8);
    const arrival = page.getByTestId("design-arrival");
    await expect(arrival).toContainText("edited the design");
    await arrival.getByRole("button", { name: "Undo" }).click();
    await expect.poll(async () => nodes((await head(id)).body).find((n) => n.id === kept.id)?.opacity ?? 1, { timeout: 30_000 }).toBe(1);
    expect((await head(id)).seq).toBeGreaterThan(seqBefore + 1);
  });
});
