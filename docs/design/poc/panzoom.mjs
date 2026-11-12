// Pan/zoom timing for the ~2k-node design in Chromium. The whole page is one
// SVG (the editor renderer's output). A scripted camera runs for a fixed
// number of animation frames (pan at overview zoom, zoom 0.1 <-> 2, pan at
// 100%), and every requestAnimationFrame timestamp is recorded. Frame time =
// delta between consecutive rAF callbacks, which includes style, layout,
// paint and raster of the frame before (Chromium blocks rAF on the previous
// frame's commit).
//
// Two camera strategies:
//   attr: set transform="translate() scale()" on the root <g> every frame (what
//         React state -> SVG attribute does). Text re-rasterises crisp each frame.
//   css:  CSS transform on a wrapper with will-change: transform, re-rendered
//         crisp (attribute) when the gesture ends. Compositor-only during the gesture.
import { readFileSync, rmSync } from "node:fs";

const stats = (xs) => {
  const d = [...xs].sort((a, b) => a - b);
  const pct = (p) => d[Math.min(d.length - 1, Math.floor(p * d.length))];
  const r = (n) => Math.round(n * 10) / 10;
  return { n: d.length, p50: r(pct(0.5)), p95: r(pct(0.95)), max: r(d.at(-1) ?? 0), over25: d.filter((x) => x > 25).length, over33: d.filter((x) => x > 33.4).length };
};

/**
 * The trace adds what rAF alone can't show: per frame, the main thread's
 * BeginMainFrame (style, layout, prepaint, paint), the raster time on worker
 * threads, and the interval between frames actually drawn (Display::DrawAndSwap).
 */
export async function panZoom(browser, svgInner, box, fonts, opts) {
  const tracePath = `/tmp/panzoom-${opts.strategy}.json`;
  await browser.startTracing(undefined, { path: tracePath, categories: ["devtools.timeline", "disabled-by-default-devtools.timeline", "cc", "viz"] });
  const raf = await run(browser, svgInner, box, fonts, opts);
  await browser.stopTracing();
  const ev = JSON.parse(readFileSync(tracePath, "utf8")).traceEvents.filter((e) => e.ph === "X" && e.dur !== undefined);
  rmSync(tracePath);
  const named = (n) => ev.filter((e) => e.name === n).sort((a, b) => a.ts - b.ts);
  const main = named("ProxyMain::BeginMainFrame");
  const draws = named("Display::DrawAndSwap");
  const t0 = main[0]?.ts ?? 0;
  const t1 = main.at(-1)?.ts ?? 0;
  const raster = named("RasterTask").filter((e) => e.ts >= t0 && e.ts <= t1);
  // Raster work per drawn frame: total worker raster time between consecutive draws.
  const perFrameRaster = [];
  for (let i = 1; i < draws.length; i++) perFrameRaster.push(raster.filter((e) => e.ts > draws[i - 1].ts && e.ts <= draws[i].ts).reduce((a, e) => a + e.dur / 1000, 0));
  return {
    raf,
    mainThreadPerFrameMs: stats(main.map((e) => e.dur / 1000)),
    rasterPerFrameMs: stats(perFrameRaster),
    drawnFrameIntervalMs: stats(draws.slice(1).map((e, i) => (e.ts - draws[i].ts) / 1000)),
  };
}

async function run(browser, svgInner, box, fonts, { strategy, frames = 360, background }) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 });
  const html = `<!doctype html><html><head><style>${fonts}html,body{margin:0;overflow:hidden;background:${background}}
    #wrap{position:absolute;inset:0;transform-origin:0 0;${strategy === "css" ? "will-change:transform;" : ""}}
    svg{position:absolute;left:0;top:0;overflow:visible}
    text{font-kerning:none;text-rendering:geometricPrecision}</style></head>
    <body><div id="wrap"><svg width="1440" height="900"><g id="cam">${svgInner}</g></svg></div></body></html>`;
  await page.setContent(html);
  await page.evaluate(async () => {
    await Promise.all(["400", "500", "600", "700"].map((w) => document.fonts.load(`${w} 16px Inter`)));
    await document.fonts.ready;
  });
  const result = await page.evaluate(
    ({ strategy, frames, box }) =>
      new Promise((resolve) => {
        const cam = document.getElementById("cam");
        const wrap = document.getElementById("wrap");
        const fit = Math.min(1440 / box.w, 900 / box.h);
        // Camera at frame i: [scale, centre x, centre y] in design coordinates.
        const third = Math.floor(frames / 3);
        const at = (i) => {
          if (i < third) {
            const t = i / third;
            return [fit * 1.4, box.x + box.w * (0.25 + 0.5 * t), box.y + box.h / 2];
          }
          if (i < 2 * third) {
            const t = (i - third) / third;
            const s = Math.exp(Math.log(0.1) + (Math.log(2) - Math.log(0.1)) * (0.5 - 0.5 * Math.cos(t * 2 * Math.PI)));
            return [s, box.x + box.w * 0.4, box.y + box.h * 0.4];
          }
          const t = (i - 2 * third) / third;
          return [1, box.x + 200 + (box.w - 400) * t, box.y + 450 + (box.h - 900) * 0.3 * t];
        };
        const apply = ([s, cx, cy]) => {
          const tx = 720 - cx * s, ty = 450 - cy * s;
          if (strategy === "attr") cam.setAttribute("transform", `translate(${tx} ${ty}) scale(${s})`);
          else wrap.style.transform = `translate(${tx}px, ${ty}px) scale(${s})`;
        };
        if (strategy === "css") cam.setAttribute("transform", "");
        apply(at(0));
        const times = [];
        let i = 0;
        const longTasks = [];
        try {
          new PerformanceObserver((l) => l.getEntries().forEach((e) => longTasks.push(e.duration))).observe({ type: "longtask", buffered: false });
        } catch {}
        const tick = (now) => {
          times.push(now);
          if (++i >= frames) {
            const d = times.slice(1).map((t, k) => t - times[k]).sort((a, b) => a - b);
            const pct = (p) => Math.round(d[Math.min(d.length - 1, Math.floor(p * d.length))] * 10) / 10;
            resolve({
              frames: d.length,
              p50: pct(0.5), p95: pct(0.95), p99: pct(0.99), max: pct(1),
              over16_7: d.filter((x) => x > 16.7 + 1).length,
              over33: d.filter((x) => x > 33.4).length,
              fps: Math.round((10000 * d.length) / (times.at(-1) - times[0])) / 10,
              longTasks: longTasks.length,
              elements: document.querySelectorAll("#cam *").length,
            });
            return;
          }
          apply(at(i));
          requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      }),
    { strategy, frames, box },
  );
  await page.close();
  return result;
}
