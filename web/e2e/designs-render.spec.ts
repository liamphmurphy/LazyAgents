import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";
import { handleRender } from "../lib/designs/worker";
import { normalize } from "../lib/designs/schema";

// Render parity (LAZYAGENTS-148, ADR-designs §5.1): the worker's PNG (resvg)
// and the editor's <DesignSvg> in Chromium, both at 2x, from the acceptance
// design. pixelmatch threshold 0.1 with anti-aliased pixels excluded.
// Needs DESIGN_WORKER_TOKEN in the environment of both this process and the
// app under test (the preview page is a 404 without it).

const TOKEN = process.env.DESIGN_WORKER_TOKEN;
const dir = join(__dirname, "../../docs/design/fixtures");
const body = readFileSync(join(dir, "acceptance-design.json"), "utf8");
const design = normalize(body).design;
const dataUri = `data:image/png;base64,${readFileSync(join(dir, "landscape.png")).toString("base64")}`;
const assets = { a_landscape: dataUri };
const MAX_DIFF = 0.001;

test.describe("render parity", () => {
  test.skip(!TOKEN, "set DESIGN_WORKER_TOKEN (also for the app under test)");

  for (const node of ["n_pay", "n_state"]) {
    test(`${node}: worker PNG matches <DesignSvg> at 2x`, async ({ browser, baseURL }) => {
      const res = await handleRender(
        new Request("http://worker/render", { method: "POST", headers: { authorization: `Bearer ${TOKEN}` }, body: JSON.stringify({ body, node, scale: 2, assets }) }),
      );
      expect(res.status).toBe(200);
      const server = PNG.sync.read(Buffer.from(await res.arrayBuffer()));

      const page = design.pages.find((p) => p.children.some((c) => c.id === node))!;
      const context = await browser.newContext({ deviceScaleFactor: 2, viewport: { width: 1200, height: 1000 } });
      const tab = await context.newPage();
      await tab.addInitScript((p) => ((window as unknown as { __preview: unknown }).__preview = p), { design, page: page.id, node, w: server.width / 2, h: server.height / 2, assets });
      await tab.goto(`${baseURL}/internal/designs/preview?token=${encodeURIComponent(TOKEN!)}`);
      await tab.waitForFunction(() => (window as unknown as { __ready?: boolean }).__ready === true);
      const editor = PNG.sync.read(await tab.locator("#shot").screenshot());
      await context.close();

      expect([editor.width, editor.height]).toEqual([server.width, server.height]);
      const differing = pixelmatch(server.data, editor.data, undefined, server.width, server.height, { threshold: 0.1, includeAA: false });
      const pct = (differing / (server.width * server.height)) * 100;
      console.log(`parity ${node}: ${differing} of ${server.width * server.height} px differ (${pct.toFixed(4)} %)`);
      expect(pct).toBeLessThanOrEqual(MAX_DIFF * 100);
    });
  }
});
