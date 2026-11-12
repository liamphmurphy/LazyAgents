import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Page, type Route } from "@playwright/test";
import { installChatApi } from "./mock-chat-api";

// The Designs app (LAZYAGENTS-147): Home, the editor's rendering, autosave
// with a debounce, undo, reopening, 409 recovery, and pan/zoom on ~2k nodes.
// The designs API is mocked here; the chat rail uses the chat mock.

const at = "2026-11-12T12:00:00Z";
const acceptance = JSON.parse(readFileSync(join(__dirname, "../../docs/design/fixtures/acceptance-design.json"), "utf8"));
const PNG = readFileSync(join(__dirname, "../../docs/design/fixtures/landscape.png"));

type Version = { seq: number; body: string; author: "user" | "agent" };
type Stored = { id: string; title: string; chat_id: string; versions: Version[] };

const simple = () => ({
  schema: 1, mode: "light", token_sets: {}, assets: {}, links: [],
  pages: [
    {
      id: "p_one", name: "Flows",
      children: [
        { id: "n_frame", type: "frame", name: "Home", preset: "iphone-15", x: 0, y: 0, w: 393, h: 852, fill: "#FFFFFF", clip: true, children: [
          { id: "n_title", type: "text", name: "Title", x: 24, y: 64, w: 200, h: 34, sizing: { w: "hug", h: "hug" }, paragraphs: [{ runs: [{ text: "Welcome", size: 28, weight: 700 }] }] },
          { id: "n_box", type: "shape", shape: "rect", name: "Box", x: 24, y: 140, w: 120, h: 80, fill: "#7C6AF0" },
        ] },
      ],
    },
    { id: "p_two", name: "Diagrams", children: [] },
  ],
});

async function install(page: Page, designs: Record<string, object>) {
  await installChatApi(page, { replyDelayMs: 10 });
  await page.goto("/apps/designs");
  const chatId = (await page.evaluate(() => fetch("/api/v1/chats", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).then((r) => r.json()))).id as string;
  const s = {
    designs: Object.entries(designs).map(([id, body], i) => ({ id, title: i ? `Design ${i}` : "Checkout flow", chat_id: chatId, versions: [{ seq: 1, body: JSON.stringify(body), author: "user" as const }] })) as Stored[],
    saves: [] as { id: string; base_seq: number; body: string }[],
    calls: [] as string[],
  };
  const head = (d: Stored) => d.versions[d.versions.length - 1];
  const json = (route: Route, data: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
  const mine = (d: Stored) => ({ id: d.id, kind: "design", title: d.title, chat_id: d.chat_id, chat_title: d.title, updated_at: at, reviewed_seq: 0 });
  const version = (v: Version) => ({ seq: v.seq, note: "Edited", author_type: v.author, created_at: at, body: v.body });
  await page.route(/\/api\/v1\/designs(\/|\?|$)/, async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const parts = url.pathname.replace(/^\/api\/v1\/designs\/?/, "").split("/").filter(Boolean);
    const method = req.method();
    s.calls.push(`${method} /${parts.join("/")}`);
    if (!parts.length) {
      if (method === "POST") {
        const body = req.postDataJSON() ?? {};
        const d: Stored = { id: `00000000-0000-4000-8000-${String(s.designs.length + 1).padStart(12, "0")}`, title: body.title || "Untitled design", chat_id: chatId, versions: [{ seq: 1, body: JSON.stringify(simple()), author: "user" }] };
        s.designs.unshift(d);
        return json(route, mine(d), 201);
      }
      return json(route, {
        designs: s.designs.map((d) => ({ id: d.id, title: d.title, chat_id: d.chat_id, updated_at: at, seq: head(d).seq, pages: JSON.parse(head(d).body).pages.length, frames: 1, pending_proposals: d.title === "Checkout flow" ? 3 : 0, persona_edited: d.versions.some((v) => v.author === "agent") })),
      });
    }
    const d = s.designs.find((x) => x.id === parts[0]);
    if (!d) return json(route, { code: "not_found", message: "not found" }, 404);
    if (parts[1] === "versions" && method === "POST") {
      const body = req.postDataJSON();
      s.saves.push({ id: d.id, ...body });
      if (body.base_seq !== head(d).seq) return json(route, { code: "conflict", message: `this canvas is at version ${head(d).seq}, not ${body.base_seq}` }, 409);
      d.versions.push({ seq: head(d).seq + 1, body: body.body, author: "user" });
      return json(route, version(head(d)), 201);
    }
    if (parts[1] === "versions") return json(route, version(d.versions.find((v) => v.seq === Number(parts[2]))!));
    if (parts[1] === "assets" && method === "POST") return json(route, { id: "a_upload", mime: "image/png", w: 640, h: 400 }, 201);
    if (parts[1] === "assets") return route.fulfill({ status: 200, contentType: "image/png", body: PNG });
    if (parts[1] === "duplicate") {
      const copy: Stored = { ...d, id: `00000000-0000-4000-8000-${String(s.designs.length + 1).padStart(12, "0")}`, title: `${d.title} (copy)`, versions: [{ ...head(d), seq: 1 }] };
      s.designs.unshift(copy);
      return json(route, mine(copy), 201);
    }
    if (method === "PATCH") {
      d.title = req.postDataJSON().title;
      return json(route, mine(d));
    }
    if (method === "DELETE") {
      s.designs = s.designs.filter((x) => x !== d);
      return route.fulfill({ status: 204 });
    }
    return json(route, { ...mine(d), seq: head(d).seq, settings: { direct_edit: false, persona_paused: false, project_id: null }, pending_proposals: 0 });
  });
  /** A version written elsewhere (a persona with direct edit on), on top of the current one. */
  const external = (id: string, edit: (body: ReturnType<typeof simple>) => void) => {
    const d = s.designs.find((x) => x.id === id)!;
    const body = JSON.parse(head(d).body);
    edit(body);
    d.versions.push({ seq: head(d).seq + 1, body: JSON.stringify(body), author: "agent" });
  };
  const latest = (id: string) => JSON.parse(head(s.designs.find((x) => x.id === id)!).body);
  return { s, external, latest };
}

const D1 = "11111111-1111-4111-8111-111111111111";
const D2 = "22222222-2222-4222-8222-222222222222";
const node = (body: { pages: { children: unknown[] }[] }, id: string): Record<string, unknown> | undefined => {
  let found: Record<string, unknown> | undefined;
  const walk = (list: Record<string, unknown>[]) => list.forEach((n) => (n.id === id ? (found = n) : Array.isArray(n.children) && walk(n.children as Record<string, unknown>[])));
  body.pages.forEach((p) => walk(p.children as Record<string, unknown>[]));
  return found;
};
const savedState = (page: Page) => page.getByTestId("design-save-state");
const canvas = (page: Page) => page.getByTestId("design-canvas");

test("Designs is in Apps; Home has starters and recent designs to rename, duplicate and delete", async ({ page }) => {
  const { s } = await install(page, { [D1]: acceptance, [D2]: simple() });
  await page.goto("/apps/designs");
  await expect(page.getByRole("heading", { name: "Designs", level: 1 })).toBeVisible();
  await page.getByRole("button", { name: "Apps" }).click();
  await expect(page.getByRole("dialog", { name: "Apps" }).getByRole("link", { name: "Designs" })).toHaveAttribute("href", "/apps/designs");
  await page.keyboard.press("Escape");

  for (const name of ["Blank", "Mobile flow", "Web flow", "Diagram", "Wireframe", /From a ticket/]) await expect(page.getByRole("button", { name })).toBeVisible();
  const cards = page.getByTestId("design-card");
  await expect(cards).toHaveCount(2);
  // The thumbnail is the first page drawn by the shared renderer.
  await expect(cards.first().getByRole("img", { name: "First page of Checkout flow" }).locator("text").first()).toBeVisible();
  await expect(cards.first()).toContainText("LAZYAGENTS-150");
  await expect(cards.first()).toContainText("3 to review");
  await page.getByRole("tab", { name: /Needs review/ }).click();
  await expect(cards).toHaveCount(1);
  await page.getByRole("tab", { name: "All" }).click();
  await page.getByRole("searchbox", { name: "Search designs" }).fill("design 1");
  await expect(cards).toHaveCount(1);
  await page.getByRole("searchbox", { name: "Search designs" }).fill("");

  await page.getByRole("button", { name: "More actions for Design 1" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  await page.getByRole("textbox", { name: "Design title" }).fill("Runner architecture");
  await page.keyboard.press("Enter");
  await expect(cards.filter({ hasText: "Runner architecture" })).toHaveCount(1);
  await page.getByRole("button", { name: "More actions for Runner architecture" }).click();
  await page.getByRole("menuitem", { name: "Duplicate" }).click();
  await expect(cards).toHaveCount(3);
  await page.getByRole("button", { name: "More actions for Runner architecture (copy)" }).click();
  await page.getByRole("menuitem", { name: "Delete…" }).click();
  await page.getByRole("button", { name: "Delete design" }).click();
  await expect(cards).toHaveCount(2);
  expect(s.calls).toEqual(expect.arrayContaining([`PATCH /${D2}`, `POST /${D2}/duplicate`, expect.stringMatching(/^DELETE \//)]));

  // A starter creates a design and opens it.
  await page.getByRole("button", { name: "Mobile flow" }).click();
  await expect(page).toHaveURL(/\/apps\/designs\/0000/);
  await expect(canvas(page)).toBeVisible();
});

test("every node type in the acceptance design renders in the editor", async ({ page }) => {
  await install(page, { [D1]: acceptance });
  await page.goto(`/apps/designs/${D1}`);
  const svg = canvas(page).locator("svg").first();
  // Text is one <text> per measured word.
  await expect(svg.locator("text", { hasText: "$84.00" })).toHaveCount(2);
  await expect(svg.locator("image")).toHaveCount(1);
  await expect(svg.locator("ellipse").first()).toBeAttached();
  await expect(svg.locator("path").first()).toBeAttached();
  await expect(svg.locator("clipPath").first()).toBeAttached();
  // Layers list the page's frames; another page shows its own (connectors, sticky, section).
  await expect(page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Payment" })).toBeVisible();
  await page.getByRole("list", { name: "Pages" }).getByRole("button", { name: "Diagrams" }).click();
  await expect(svg.locator("polyline").first()).toBeAttached();
  await expect(svg.locator("text", { hasText: "Order state" })).toHaveCount(1);
});

test("drawing, autosave, undo and reopening: rapid edits save as one version", async ({ page }) => {
  const { s, latest } = await install(page, { [D1]: simple() });
  await page.goto(`/apps/designs/${D1}`);
  await expect(savedState(page)).toContainText("Saved · v1");

  // Draw a rectangle on the page outside the frame.
  await page.getByRole("button", { name: "Rectangle (R)" }).click();
  const box = (await canvas(page).boundingBox())!;
  await page.mouse.move(box.x + box.width - 160, box.y + 120);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 80, box.y + 180, { steps: 4 });
  await page.mouse.up();
  await expect(page.getByTestId("design-size-badge")).toBeVisible();
  await expect(savedState(page)).toContainText("Saved · v2");
  expect(s.saves).toHaveLength(1);
  // Drawn over the frame, it went into the frame, on top.
  const added = (latest(D1).pages[0].children[0].children as { id: string; x: number }[]).at(-1)!;
  expect(added).toMatchObject({ type: "shape", shape: "rect" });

  // Ten nudges in quick succession are debounced into one version.
  for (let i = 0; i < 10; i++) await page.keyboard.press("ArrowRight");
  await expect(savedState(page)).toContainText("Saved · v3");
  await page.waitForTimeout(1200);
  expect(s.saves).toHaveLength(2);
  expect(node(latest(D1), added.id)!.x).toBe(added.x + 10);

  // Undo: the nudges were one step.
  await page.keyboard.press("ControlOrMeta+z");
  await expect(savedState(page)).toContainText("Saved · v4");
  expect(node(latest(D1), added.id)!.x).toBe(added.x);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect(savedState(page)).toContainText("Saved · v5");
  expect(node(latest(D1), added.id)!.x).toBe(added.x + 10);

  // Auto layout from the inspector: the frame's children reflow.
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Home" }).locator("div").first().click();
  await page.getByRole("button", { name: "Add auto layout" }).click();
  await expect(savedState(page)).toContainText("Saved · v6");
  const title = node(latest(D1), "n_title") as { y: number; h: number };
  expect(title.y).toBe(16);
  expect(node(latest(D1), "n_box")!.y).toBe(title.y + title.h + 16);
  await page.getByRole("textbox", { name: "Gap" }).fill("40");
  await expect(savedState(page)).toContainText("Saved · v7");
  expect(node(latest(D1), "n_box")!.y).toBe(title.y + title.h + 40);

  // Delete, then reopen: what's saved is what opens.
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Home" }).getByRole("button", { name: "Expand" }).click();
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Rectangle" }).locator("div").first().click();
  await page.keyboard.press("Delete");
  await expect(savedState(page)).toContainText("Saved · v8");
  await page.reload();
  await expect(savedState(page)).toContainText("Saved · v8");
  await expect(page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Rectangle" })).toHaveCount(0);
  await expect(page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Home" })).toBeVisible();
});

test("text edits in place; a page can be added and renamed", async ({ page }) => {
  const { latest } = await install(page, { [D1]: simple() });
  await page.goto(`/apps/designs/${D1}`);
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Home" }).getByRole("button", { name: "Expand" }).click();
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Title" }).locator("div").first().click();
  await page.keyboard.press("Enter");
  const editor = page.getByTestId("design-text-editor");
  await expect(editor).toBeFocused();
  await editor.fill("Welcome back");
  await page.keyboard.press("Escape");
  await expect(savedState(page)).toContainText("Saved · v2");
  expect((node(latest(D1), "n_title") as { paragraphs: { runs: { text: string; weight: number }[] }[] }).paragraphs[0].runs[0]).toMatchObject({ text: "Welcome back", weight: 700 });
  await page.getByRole("button", { name: "Add a page" }).click();
  await page.getByRole("textbox", { name: "Page name" }).fill("Explorations");
  await page.keyboard.press("Enter");
  await expect(savedState(page)).toContainText("Saved · v3");
  expect(latest(D1).pages.map((p: { name: string }) => p.name)).toEqual(["Flows", "Diagrams", "Explorations"]);
});

test("a 409 merges another layer's change, and asks about the same layer without dropping either side", async ({ page }) => {
  const { s, external, latest } = await install(page, { [D1]: simple() });
  await page.goto(`/apps/designs/${D1}`);
  await expect(savedState(page)).toContainText("Saved · v1");

  // A persona changes the title meanwhile; my nudge of the box merges with it.
  external(D1, (b) => ((b.pages[0].children[0] as { children: { name: string }[] }).children[0].name = "Heading"));
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Home" }).getByRole("button", { name: "Expand" }).click();
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Box" }).locator("div").first().click();
  await page.keyboard.press("ArrowDown");
  await expect(savedState(page)).toContainText("Saved · v3");
  expect(s.saves.map((x) => x.base_seq)).toEqual([1, 2]);
  expect(node(latest(D1), "n_title")!.name).toBe("Heading");
  expect(node(latest(D1), "n_box")!.y).toBe(141);

  // Now both change the box: nothing saves until the person chooses.
  external(D1, (b) => ((b.pages[0].children[0] as { children: { fill?: string }[] }).children[1].fill = "#FF0000"));
  await page.keyboard.press("ArrowDown");
  const banner = page.getByText("This design changed while you were editing");
  await expect(banner).toBeVisible();
  await expect(page.getByText(/both changed Box/)).toBeVisible();
  await expect(savedState(page)).toContainText("Not saved");
  expect(s.designs[0].versions.at(-1)!.author).toBe("agent");
  // Keep mine saves over theirs; theirs stays as a version.
  await page.getByRole("button", { name: "Keep mine" }).click();
  await expect(savedState(page)).toContainText("Saved · v5");
  expect(node(latest(D1), "n_box")).toMatchObject({ y: 142, fill: "#7C6AF0" });
  expect(node(JSON.parse(s.designs[0].versions[3].body), "n_box")).toMatchObject({ fill: "#FF0000" });

  // And Use the latest version takes theirs, dropping only the unsaved edit.
  external(D1, (b) => ((b.pages[0].children[0] as { children: { fill?: string }[] }).children[1].fill = "#00FF00"));
  await page.keyboard.press("ArrowDown");
  await expect(banner).toBeVisible();
  await page.getByRole("button", { name: "Use the latest version" }).click();
  await expect(banner).toHaveCount(0);
  await expect(savedState(page)).toContainText("Saved · v6");
  // History restarted with their version: undo has nothing to take back.
  await page.keyboard.press("ControlOrMeta+z");
  await page.waitForTimeout(1200);
  expect(s.designs[0].versions).toHaveLength(6);
});

/** The PoC's ~2k-node design (docs/design/poc/large.mjs): 40 phone frames of 12 auto-layout rows, wired by connectors. */
function largeDesign(frames = 40, rows = 12) {
  const children: unknown[] = [];
  const cols = 8;
  for (let f = 0; f < frames; f++) {
    const kids: unknown[] = [{ id: `n_f${f}_title`, type: "text", name: "Title", x: 24, y: 64, w: 345, h: 31, sizing: { w: "fill", h: "hug" }, paragraphs: [{ runs: [{ text: `Screen ${f + 1}`, size: 26, weight: 600, color: "$text" }] }] }];
    for (let r = 0; r < rows; r++) {
      const id = `n_f${f}_r${r}`;
      kids.push({
        id, type: "frame", name: `Row ${r + 1}`, x: 24, y: 0, w: 345, h: 52, sizing: { w: "fill", h: "hug" }, fill: "$surface", radius: 10,
        auto_layout: { direction: "horizontal", gap: 12, padding: [8, 12, 8, 12], align: "center" },
        children: [
          { id: `${id}_av`, type: "shape", shape: "ellipse", name: "Avatar", x: 0, y: 0, w: 36, h: 36, fill: r % 2 ? "$agent-quill" : "$agent-atlas" },
          { id: `${id}_name`, type: "text", name: "Name", x: 0, y: 0, w: 150, h: 18, sizing: { w: "fill", h: "hug" }, paragraphs: [{ runs: [{ text: `Item ${r + 1} on screen ${f + 1}`, size: 15, weight: 500, color: "$text" }] }] },
          { id: `${id}_meta`, type: "text", name: "Meta", x: 0, y: 0, w: 60, h: 16, sizing: { w: "hug", h: "hug" }, paragraphs: [{ runs: [{ text: `${(r * 7 + f) % 59}m ago`, size: 13, color: "$text-tertiary" }] }] },
        ],
      });
    }
    children.push({ id: `n_f${f}`, type: "frame", name: `Screen ${f + 1}`, preset: "iphone-15", x: (f % cols) * 473, y: Math.floor(f / cols) * 952, w: 393, h: 852, fill: "$bg-panel", clip: true, auto_layout: { direction: "vertical", gap: 8, padding: [64, 24, 32, 24] }, children: kids });
    if (f > 0 && f % cols) children.push({ id: `n_w${f}`, type: "connector", from: { node: `n_f${f - 1}`, anchor: "right" }, to: { node: `n_f${f}`, anchor: "left" }, route: "orthogonal", stroke: "$accent", stroke_width: 2, end_arrow: "arrow" });
  }
  return { ...acceptance, pages: [{ id: "p_large", name: "Large", background: "$bg", children }], links: [] };
}

test("pan and zoom stay smooth on the ~2k-node PoC design", async ({ page }, info) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  await install(page, { [D1]: largeDesign() });
  await page.goto(`/apps/designs/${D1}`);
  const svg = canvas(page).locator("svg").first();
  await expect(svg.locator("text", { hasText: /^40$/ }).first()).toBeAttached();
  // Close the chat rail so the canvas is the PoC's size.
  await page.getByRole("button", { name: "Hide chat" }).click();
  // The PoC's camera path through the editor's own wheel handling: pan across the grid at
  // overview zoom (every node visible), zoom 0.1× ↔ 2×, pan at 100 %. One wheel event per frame.
  const result = await canvas(page).evaluate(async (el) => {
    const r = el.getBoundingClientRect();
    const fire = (dx: number, dy: number, zoom: boolean, x = r.left + r.width / 2, y = r.top + r.height / 2) =>
      el.dispatchEvent(new WheelEvent("wheel", { deltaX: dx, deltaY: dy, ctrlKey: zoom, clientX: x, clientY: y, bubbles: true, cancelable: true }));
    const frame = () => new Promise<number>((res) => requestAnimationFrame(res));
    // Overview first: zoom out until the whole grid fits.
    for (let i = 0; i < 4; i++) {
      fire(0, 60, true);
      await frame();
    }
    const steps: (() => void)[] = [];
    for (let i = 0; i < 120; i++) steps.push(() => fire(i < 60 ? 12 : -12, i % 40 < 20 ? 4 : -4, false));
    for (let i = 0; i < 120; i++) steps.push(() => fire(0, i < 60 ? -17 : 17, true));
    for (let i = 0; i < 120; i++) steps.push(() => fire(i < 60 ? 20 : -20, i < 60 ? 10 : -10, false));
    const times: number[] = [];
    let last = await frame();
    for (const step of steps) {
      step();
      const t = await frame();
      times.push(t - last);
      last = t;
    }
    const sorted = [...times].sort((a, b) => a - b);
    const pct = (p: number) => sorted[Math.min(sorted.length - 1, Math.floor(p * sorted.length))];
    return { frames: times.length, fps: Math.round((1000 * times.length) / times.reduce((a, b) => a + b, 0)), p50: pct(0.5), p95: pct(0.95), max: sorted.at(-1)!, over33: times.filter((t) => t > 33.4).length, nodes: el.querySelectorAll("svg:first-of-type g[data-testid=design-camera] *").length };
  });
  await info.attach("panzoom.json", { body: JSON.stringify(result, null, 2), contentType: "application/json" });
  console.log("pan/zoom on the 2k-node design:", JSON.stringify(result));
  expect(result.nodes).toBeGreaterThan(4000);
  // 60 fps target; the bound leaves room for a loaded CI machine, the attachment has the numbers.
  expect(result.p95).toBeLessThan(34);
  expect(result.over33 / result.frames).toBeLessThan(0.05);
});

// ---- Prototype links and Present mode (LAZYAGENTS-149).

/** simple() with a second frame, and optionally links. */
const twoFrames = (links: unknown[] = []) => {
  const b = simple();
  (b.pages[0].children as unknown[]).push({ id: "n_next", type: "frame", name: "Details", x: 500, y: 0, w: 393, h: 852, fill: "#F4F4F8", clip: true, children: [] });
  return { ...b, links };
};

test("the Prototype tab adds, edits and removes a layer's links, and they're saved in the body", async ({ page }) => {
  const { latest } = await install(page, { [D1]: twoFrames() });
  await page.goto(`/apps/designs/${D1}`);
  await expect(savedState(page)).toContainText("Saved · v1");
  const layers = page.getByRole("tree", { name: "Layers" });
  await layers.getByRole("treeitem", { name: "Home" }).getByRole("button", { name: "Expand" }).click();
  await layers.getByRole("treeitem", { name: "Box" }).locator("div").first().click();
  await page.getByRole("tab", { name: "Prototype" }).click();

  await page.getByRole("button", { name: "Add interaction" }).click();
  await expect(savedState(page)).toContainText("Saved · v2");
  expect(latest(D1).links).toEqual([expect.objectContaining({ from: "n_box", to: "n_next", trigger: "click", transition: "dissolve" })]);
  await expect(page.getByTestId("design-wires").locator("[data-wire]")).toHaveCount(1);

  await page.getByRole("combobox", { name: "Transition" }).selectOption({ label: "Slide left" });
  await page.getByRole("combobox", { name: "Trigger" }).selectOption({ label: "After delay" });
  await expect(savedState(page)).toContainText("Saved · v3");
  expect(latest(D1).links[0]).toMatchObject({ transition: "slide-left", trigger: "after-delay", delay_ms: 800 });

  await page.getByRole("button", { name: "Remove", exact: true }).click();
  await expect(savedState(page)).toContainText("Saved · v4");
  expect(latest(D1).links).toEqual([]);
});

test("a link to a deleted frame shows as broken in the editor and is skipped in Present", async ({ page }) => {
  const broken = { id: "k_gone", from: "n_box", to: "n_gone", trigger: "click", transition: "dissolve", duration_ms: 300 };
  const live = { id: "k_live", from: "n_title", to: "n_next", trigger: "click", transition: "instant" };
  await install(page, { [D1]: twoFrames([broken, live]) });
  await page.goto(`/apps/designs/${D1}`);
  await page.getByRole("tab", { name: "Prototype" }).click();
  await expect(page.getByRole("region", { name: "Flow" })).toContainText("1 broken");
  await expect(page.locator('[data-link="k_gone"][data-broken]')).toContainText("deleted frame (broken)");
  await expect(page.locator('[data-wire="k_gone"][data-broken]')).toBeAttached();
  await page.locator('[data-link="k_gone"] button').first().click();
  await expect(page.getByRole("alert").filter({ hasText: "Broken link" })).toBeVisible();

  await page.goto(`/present/designs/${D1}?start=n_frame`);
  await expect(page.getByTestId("present-frame")).toHaveAttribute("data-at", "n_frame");
  await expect(page.locator("[data-hotspot]")).toHaveCount(1);
  await page.locator('[data-hotspot="k_live"]').click();
  await expect(page.getByTestId("present-frame")).toHaveAttribute("data-at", "n_next");
});

test("Present starts from a chosen frame, follows hotspots, and goes back, restarts and exits by keyboard", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await install(page, { [D1]: acceptance });
  await page.goto(`/apps/designs/${D1}`);
  await page.getByRole("tree", { name: "Layers" }).getByRole("treeitem", { name: "Shipping" }).locator("div").first().click();
  await page.getByRole("tab", { name: "Prototype" }).click();
  await page.getByRole("button", { name: "Present from “Shipping”" }).click();
  await expect(page).toHaveURL(new RegExp(`/present/designs/${D1}\\?start=n_ship`));
  const frame = page.getByTestId("present-frame");
  await expect(frame).toHaveAttribute("data-at", "n_ship");

  await page.getByRole("button", { name: "Go to Payment" }).click();
  await expect(frame).toHaveAttribute("data-at", "n_pay");
  await page.getByRole("button", { name: "Go to Confirmation" }).click();
  await expect(frame).toHaveAttribute("data-at", "n_done");
  await page.keyboard.press("ArrowLeft");
  await expect(frame).toHaveAttribute("data-at", "n_pay");
  await page.keyboard.press("r");
  await expect(frame).toHaveAttribute("data-at", "n_ship");
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(new RegExp(`/apps/designs/${D1}$`));
  expect(errors).toEqual([]);
});

test("diagram tools: connect two nodes, add a sticky, and draw a section that takes them in", async ({ page }) => {
  const shapeAt = (id: string, name: string, x: number) => ({ id, type: "shape", shape: "roundRect", name, x, y: 0, w: 140, h: 60, fill: "#E5E7EB" });
  const body = { schema: 1, mode: "light", token_sets: {}, assets: {}, links: [], pages: [{ id: "p_diag", name: "Diagram", children: [shapeAt("n_a", "Start", 0), shapeAt("n_b", "End", 480)] }] };
  const { latest } = await install(page, { [D1]: body });
  await page.goto(`/apps/designs/${D1}`);
  await expect(savedState(page)).toContainText("Saved · v1");
  const layers = page.getByRole("tree", { name: "Layers" });
  const centre = async (name: string, id: string) => {
    await layers.getByRole("treeitem", { name }).locator("div").first().click();
    const b = (await page.locator(`[data-selected="${id}"]`).boundingBox())!;
    return { x: b.x + b.width / 2, y: b.y + b.height / 2, b };
  };
  const a = await centre("Start", "n_a");
  const b = await centre("End", "n_b");

  // Connector: drag from Start to End.
  await page.getByRole("button", { name: "Connector (C)" }).click();
  await page.mouse.move(a.x, a.y);
  await page.mouse.down();
  await page.mouse.move(b.x, b.y, { steps: 6 });
  await expect(page.getByTestId("design-connect-preview")).toBeVisible();
  await page.mouse.up();
  await expect.poll(() => latest(D1).pages[0].children.find((n: { type: string }) => n.type === "connector")).toMatchObject({ from: { node: "n_a" }, to: { node: "n_b" } });

  // Sticky: click below Start and type.
  await page.getByRole("button", { name: "Sticky (S)" }).click();
  await page.mouse.click(a.x, a.y + a.b.height * 4);
  const sticky = page.getByTestId("design-sticky-editor");
  await expect(sticky).toBeFocused();
  await sticky.fill("Check the copy");
  await page.keyboard.press("Escape");
  await expect.poll(() => latest(D1).pages[0].children.find((n: { type: string }) => n.type === "sticky")?.text).toBe("Check the copy");

  // Section: drawn around both shapes, it takes them (and their connector) in.
  await page.getByRole("button", { name: "Section (⇧S)" }).click();
  await page.mouse.move(a.b.x - 30, a.b.y - 60);
  await page.mouse.down();
  await page.mouse.move(b.b.x + b.b.width + 30, b.b.y + b.b.height + 30, { steps: 6 });
  await page.mouse.up();
  await expect.poll(() => (latest(D1).pages[0].children[0] as { type: string; children?: { id: string }[] }).children?.map((c) => c.id).slice(0, 2)).toEqual(["n_a", "n_b"]);
  const section = latest(D1).pages[0].children[0] as { id: string; type: string; x: number; children: { id: string; type: string; x: number }[] };
  expect(section.type).toBe("section");
  expect(section.children.some((c) => c.type === "connector")).toBe(true);

  // Moving the section moves its contents: children keep their place relative to it.
  const startX = section.children.find((c) => c.id === "n_a")!.x;
  await layers.getByRole("treeitem", { name: "Section" }).locator("div").first().click();
  await page.keyboard.press("Shift+ArrowRight");
  await expect.poll(() => (latest(D1).pages[0].children[0] as { x: number }).x).toBe(section.x + 10);
  expect(node(latest(D1), "n_a")!.x).toBe(startX);
});
