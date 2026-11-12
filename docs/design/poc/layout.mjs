// The shared display list (PoC of web/lib/designs/layout.ts). One function
// turns a node of a design into flat drawing ops in page coordinates. The SVG
// renderer (editor) and the server PNG renderer both draw these ops, so they
// agree on every position, line break and colour.
//
// Covered here: frames/components/instances with auto layout (hug, fill,
// fixed; gap, padding, align, justify), groups, sections, shapes (rect,
// roundRect, ellipse, diamond) with text, paths, text (Inter, Slides text
// model, greedy wrap), images (crop + cover, rounded clip), stickies and
// orthogonal connectors with arrowheads and labels. Rotation and constraints
// are left to the editor ticket.

export const SHAPE_INSET = 8;
export const STICKY_INSET = 12;
export const LINE = 1.2;
export const STICKY_COLORS = { yellow: "#FDE68A", pink: "#FBCFE8", green: "#BBF7D0", blue: "#BFDBFE", violet: "#DDD6FE", grey: "#E5E7EB" };
export const STICKY_TEXT = "#1A1206";
const STUB = 20; // connector stub length out of an anchor
const ARROW = 10; // arrowhead length

const r2 = (n) => Math.round(n * 100) / 100;

export function makeLayout(design, measureKit) {
  const { measure, face } = measureKit;
  const index = new Map();
  const visit = (n, parent, page) => {
    index.set(n.id, { n, parent, page });
    for (const c of n.children ?? []) visit(c, n, page);
  };
  for (const p of design.pages) for (const n of p.children) visit(n, null, p);
  const set = design.token_sets[design.tokens_ref?.set];

  const color = (c, mode) => {
    if (c === undefined) return undefined;
    if (!c.startsWith("$")) return c;
    const name = c.slice(1);
    return (mode === "light" && set.modes.light?.[name]?.value) || set.modes.dark[name].value;
  };
  const paint = (p, mode) => (p === undefined ? undefined : typeof p === "string" ? color(p, mode) : { angle: p.angle, stops: p.stops.map((s) => color(s, mode)) });

  // ---------- text ----------
  const weightOf = (r) => r.weight ?? (r.bold ? 700 : 400);
  const lineHeight = (p) => Math.max(...p.runs.map((r) => r.size)) * LINE * (p.lineSpacing || 1);

  /** Greedy wrap (as lib/slides/layout.ts). innerW = Infinity gives the unwrapped width. */
  function wrap(paragraphs, innerW) {
    const lines = [];
    let y = 0;
    for (const p of paragraphs) {
      y += p.spaceBefore ?? 0;
      const lh = lineHeight(p);
      const maxSize = Math.max(...p.runs.map((r) => r.size));
      let cur = [];
      let w = 0;
      const flush = () => {
        lines.push({ y, lh, maxSize, pieces: cur, align: p.align ?? "left" });
        cur = [];
        w = 0;
        y += lh;
      };
      for (const r of p.runs)
        for (const t of r.text.split(/(?<= )/)) {
          if (!t) continue;
          const wt = weightOf(r);
          const tw = measure(t, wt, r.italic, r.size);
          const trimmed = measure(t.trimEnd(), wt, r.italic, r.size);
          if (cur.length && w + trimmed > innerW + 0.001) flush();
          cur.push({ run: r, text: t, x: w, w: tw });
          w += tw;
        }
      flush();
    }
    const width = Math.max(0, ...lines.map((l) => usedWidth(l)));
    return { lines, height: y, width };
  }
  const usedWidth = (l) => {
    const last = l.pieces.at(-1);
    return last ? last.x + measure(last.text.trimEnd(), weightOf(last.run), last.run.italic, last.run.size) : 0;
  };

  function textOps(paragraphs, box, inset, valign, mode, ops, el, defaultColor) {
    const innerW = box.w - 2 * inset;
    const { lines, height } = wrap(paragraphs, innerW);
    const avail = box.h - 2 * inset;
    const shift = valign === "middle" ? (avail - height) / 2 : valign === "bottom" ? avail - height : 0;
    for (const l of lines) {
      const f = face(400, false);
      const baseline = box.y + inset + shift + l.y + (l.lh - (f.asc + f.desc) * l.maxSize) / 2 + f.asc * l.maxSize;
      const room = innerW - usedWidth(l);
      const dx = l.align === "center" ? room / 2 : l.align === "right" ? room : 0;
      for (const pc of l.pieces) {
        const text = pc.text;
        if (!text.trim()) continue;
        const r = pc.run;
        const x = box.x + inset + dx + pc.x;
        const w = measure(text.trimEnd(), weightOf(r), r.italic, r.size);
        const fill = color(r.color, mode) ?? defaultColor;
        ops.push({ op: "text", x: r2(x), y: r2(baseline), text: text.trimEnd(), weight: weightOf(r), italic: !!r.italic, size: r.size, fill, el });
        const fm = face(weightOf(r), r.italic);
        if (r.underline) ops.push({ op: "rect", x: r2(x), y: r2(baseline + fm.underline.pos * r.size), w: r2(w), h: r2(fm.underline.thick * r.size), fill, el });
        if (r.strike) ops.push({ op: "rect", x: r2(x), y: r2(baseline - fm.strike.pos * r.size), w: r2(w), h: r2(fm.strike.thick * r.size), fill, el });
      }
    }
    return height;
  }

  // ---------- sizing and auto layout ----------
  const sizing = (n) => ({ w: n.sizing?.w ?? "fixed", h: n.sizing?.h ?? "fixed" });
  const AL = (n) => n.auto_layout && { gap: 0, padding: [0, 0, 0, 0], align: "start", justify: "start", ...n.auto_layout };
  const flowKids = (n) => (n.children ?? []).filter((c) => !c.absolute && c.visible !== false && c.type !== "connector");

  /** Expands an instance into its component's subtree with the overrides applied. */
  function expand(n) {
    if (n.type !== "instance") return n;
    const main = index.get(n.component).n;
    const clone = (m) => {
      const o = n.overrides?.[m.id] ?? {};
      const c = { ...m, id: `${n.id}/${m.id}`, children: m.children?.map(clone) };
      if (o.fill !== undefined) c.fill = o.fill;
      if (o.visible !== undefined) c.visible = o.visible;
      if (o.text !== undefined && c.paragraphs) c.paragraphs = [{ ...c.paragraphs[0], runs: [{ ...c.paragraphs[0].runs[0], text: o.text }] }];
      return c;
    };
    const root = clone(main);
    return { ...root, id: n.id, type: "frame", x: n.x, y: n.y, w: n.w, h: n.h, sizing: n.sizing ?? main.sizing, name: n.name };
  }

  /** Intrinsic (hug) width; for text, its unwrapped width. */
  function hugW(n) {
    n = expand(n);
    if (n.type === "text") return wrap(n.paragraphs, Infinity).width;
    const al = AL(n);
    if (!al) return n.w;
    const ws = flowKids(n).map((c) => (sizing(c).w === "fill" ? (al.direction === "horizontal" ? hugW(c) : 0) : widthOf(c)));
    const [, pr, , pl] = al.padding;
    return pl + pr + (al.direction === "horizontal" ? ws.reduce((a, b) => a + b, 0) + al.gap * Math.max(0, ws.length - 1) : Math.max(0, ...ws));
  }
  const widthOf = (n) => (sizing(n).w === "hug" ? hugW(n) : n.w);

  /** Height at a given width (hug), after laying the children out. */
  function hugH(n, w) {
    n = expand(n);
    if (n.type === "text") return wrap(n.paragraphs, w).height;
    const al = AL(n);
    if (!al) return n.h;
    const boxes = childBoxes(n, al, w, undefined);
    const [pt, , pb] = al.padding;
    const hs = boxes.map((b) => b.h);
    return pt + pb + (al.direction === "vertical" ? hs.reduce((a, b) => a + b, 0) + al.gap * Math.max(0, hs.length - 1) : Math.max(0, ...hs));
  }

  /** Child sizes (and positions when h is known) inside an auto-layout container of width w. */
  function childBoxes(n, al, w, h) {
    const kids = flowKids(n);
    const [pt, pr, pb, pl] = al.padding;
    const innerW = w - pl - pr;
    const horiz = al.direction === "horizontal";
    // Widths.
    let widths;
    if (horiz) {
      const fixed = kids.map((c) => (sizing(c).w === "fill" ? null : widthOf(c)));
      const fills = fixed.filter((x) => x === null).length;
      const used = fixed.reduce((a, b) => a + (b ?? 0), 0) + al.gap * Math.max(0, kids.length - 1);
      const share = fills ? Math.max(0, innerW - used) / fills : 0;
      widths = fixed.map((x) => x ?? share);
    } else widths = kids.map((c) => (sizing(c).w === "fill" ? innerW : widthOf(c)));
    // Heights.
    const innerH = h === undefined ? undefined : h - pt - pb;
    let heights = kids.map((c, i) => (sizing(c).h === "hug" ? hugH(c, widths[i]) : sizing(c).h === "fill" && !horiz ? null : sizing(c).h === "fill" ? (innerH ?? c.h) : c.h));
    if (!horiz) {
      const fills = heights.filter((x) => x === null).length;
      const used = heights.reduce((a, b) => a + (b ?? 0), 0) + al.gap * Math.max(0, kids.length - 1);
      const share = fills && innerH !== undefined ? Math.max(0, innerH - used) / fills : 0;
      heights = heights.map((x) => x ?? share);
    }
    // Positions.
    const mainSizes = horiz ? widths : heights;
    const total = mainSizes.reduce((a, b) => a + b, 0) + al.gap * Math.max(0, kids.length - 1);
    const mainInner = horiz ? innerW : (innerH ?? total);
    let gap = al.gap;
    let start = 0;
    if (al.justify === "center") start = (mainInner - total) / 2;
    else if (al.justify === "end") start = mainInner - total;
    else if (al.justify === "space-between" && kids.length > 1) gap = (mainInner - (total - al.gap * (kids.length - 1))) / (kids.length - 1);
    const crossInner = horiz ? (innerH ?? Math.max(0, ...heights)) : innerW;
    let at = start;
    return kids.map((c, i) => {
      const cw = widths[i];
      const ch = heights[i];
      const cross = horiz ? ch : cw;
      const off = al.align === "center" ? (crossInner - cross) / 2 : al.align === "end" ? crossInner - cross : 0;
      const box = horiz ? { x: pl + at, y: pt + off, w: cw, h: ch } : { x: pl + off, y: pt + at, w: cw, h: ch };
      at += (horiz ? cw : ch) + gap;
      return { c, ...box };
    });
  }

  // ---------- drawing ----------
  const anchorsOf = new Map(); // node id -> absolute box, for connectors

  function draw(n, ax, ay, w, h, mode, ops) {
    if (n.visible === false) return;
    n = expand(n);
    mode = n.mode ?? mode;
    const box = { x: ax, y: ay, w, h };
    anchorsOf.set(n.id, box);
    const op0 = ops.length;
    switch (n.type) {
      case "frame":
      case "component":
      case "section": {
        const fill = paint(n.fill, mode);
        if (fill || n.stroke) ops.push({ op: "rect", ...box, r: n.radius ?? 0, fill, stroke: color(n.stroke, mode), sw: n.stroke_width ?? 1, el: n.id });
        if (n.type === "section") ops.push({ op: "text", x: ax + 16, y: ay + 28, text: n.name ?? "Section", weight: 600, italic: false, size: 14, fill: color("$text-secondary", mode), el: n.id });
        const clip = n.clip && n.type !== "section";
        if (clip) ops.push({ op: "clip", ...box, r: n.radius ?? 0 });
        drawChildren(n, box, mode, ops);
        if (clip) ops.push({ op: "unclip" });
        break;
      }
      case "group":
        drawChildren(n, box, mode, ops);
        break;
      case "shape": {
        const fill = paint(n.fill, mode);
        const stroke = color(n.stroke, mode);
        const sw = n.stroke_width ?? 1;
        if (n.shape === "ellipse") ops.push({ op: "ellipse", ...box, fill, stroke, sw, el: n.id });
        else if (n.shape === "diamond")
          ops.push({ op: "path", d: `M${r2(ax + w / 2)} ${r2(ay)} L${r2(ax + w)} ${r2(ay + h / 2)} L${r2(ax + w / 2)} ${r2(ay + h)} L${r2(ax)} ${r2(ay + h / 2)} Z`, fill, stroke, sw, el: n.id });
        else ops.push({ op: "rect", ...box, r: n.shape === "roundRect" ? Math.min(n.radius ?? 8, w / 2, h / 2) : 0, fill, stroke, sw, el: n.id });
        if (n.text) textOps(n.text.paragraphs, box, SHAPE_INSET, n.text.valign ?? "middle", mode, ops, n.id, color("$text", mode));
        break;
      }
      case "path": {
        const d = n.d.replace(/(-?[\d.]+)[ ,](-?[\d.]+)/g, (_, x, y) => `${r2(ax + +x)} ${r2(ay + +y)}`);
        ops.push({ op: "path", d, fill: paint(n.fill, mode), stroke: color(n.stroke, mode), sw: n.stroke_width ?? 1, cap: n.cap, join: n.join, el: n.id });
        break;
      }
      case "text":
        textOps(n.paragraphs, box, 0, n.valign ?? "top", mode, ops, n.id, color("$text", mode));
        break;
      case "image": {
        const a = design.assets[n.asset];
        const c = n.crop ?? { l: 0, t: 0, r: 0, b: 0 };
        // Source rectangle after the crop, then cover inside the frame.
        let sx = c.l * a.w, sy = c.t * a.h, sw = (1 - c.l - c.r) * a.w, sh = (1 - c.t - c.b) * a.h;
        if ((n.fit ?? "cover") === "cover") {
          const scale = Math.max(w / sw, h / sh);
          const vw = w / scale, vh = h / scale;
          sx += (sw - vw) / 2; sy += (sh - vh) / 2; sw = vw; sh = vh;
        }
        ops.push({ op: "image", asset: n.asset, ...box, r: n.radius ?? 0, src: { x: r2(sx), y: r2(sy), w: r2(sw), h: r2(sh) }, iw: a.w, ih: a.h, alt: n.alt ?? a.alt ?? "", el: n.id });
        break;
      }
      case "sticky": {
        ops.push({ op: "rect", ...box, r: 4, fill: STICKY_COLORS[n.color ?? "yellow"], el: n.id });
        const paras = [{ runs: [{ text: n.text, size: 14, weight: 500, color: STICKY_TEXT }] }];
        if (n.author) paras.push({ spaceBefore: 6, runs: [{ text: n.author, size: 11, color: STICKY_TEXT }] });
        textOps(paras, box, STICKY_INSET, "top", mode, ops, n.id, STICKY_TEXT);
        break;
      }
    }
    if (n.opacity !== undefined && n.opacity < 1) for (let i = op0; i < ops.length; i++) ops[i].opacity = (ops[i].opacity ?? 1) * n.opacity;
  }

  function drawChildren(n, box, mode, ops) {
    const al = AL(n);
    const placed = al ? childBoxes(n, al, box.w, box.h) : [];
    const byId = new Map(placed.map((b) => [b.c.id, b]));
    const connectors = [];
    for (const c of n.children ?? []) {
      if (c.type === "connector") { connectors.push(c); continue; }
      const b = byId.get(c.id);
      if (b) draw(c, box.x + b.x, box.y + b.y, b.w, b.h, mode, ops);
      else {
        const w = widthOf(c);
        draw(c, box.x + c.x, box.y + c.y, w, sizing(c).h === "hug" ? hugH(c, w) : c.h, mode, ops);
      }
    }
    for (const c of connectors) drawConnector(c, mode, ops, typeof n.fill === "string" ? color(n.fill, mode) : undefined, box);
  }

  // ---------- connectors ----------
  const DIRS = { top: [0, -1], bottom: [0, 1], left: [-1, 0], right: [1, 0] };
  function anchorPoint(b, a) {
    return { top: [b.x + b.w / 2, b.y], bottom: [b.x + b.w / 2, b.y + b.h], left: [b.x, b.y + b.h / 2], right: [b.x + b.w, b.y + b.h / 2] }[a];
  }
  function autoAnchor(from, to) {
    const dx = to.x + to.w / 2 - (from.x + from.w / 2);
    const dy = to.y + to.h / 2 - (from.y + from.h / 2);
    return Math.abs(dx) > Math.abs(dy) ? (dx > 0 ? "right" : "left") : dy > 0 ? "bottom" : "top";
  }
  /** Simple orthogonal route: stubs out of both anchors joined by one or two bends. Obstacle avoidance is ticket 7. */
  function route(fb, fa, tb, ta) {
    const p0 = anchorPoint(fb, fa), p3 = anchorPoint(tb, ta);
    const [fdx, fdy] = DIRS[fa], [tdx, tdy] = DIRS[ta];
    const p1 = [p0[0] + fdx * STUB, p0[1] + fdy * STUB];
    const p2 = [p3[0] + tdx * STUB, p3[1] + tdy * STUB];
    const pts = [p0, p1];
    const fh = fdx !== 0, th = tdx !== 0;
    if (fh && th) pts.push([(p1[0] + p2[0]) / 2, p1[1]], [(p1[0] + p2[0]) / 2, p2[1]]);
    else if (!fh && !th) {
      const ymid = fdy === tdy ? (fdy > 0 ? Math.max(p1[1], p2[1]) : Math.min(p1[1], p2[1])) : (p1[1] + p2[1]) / 2;
      pts.push([p1[0], ymid], [p2[0], ymid]);
    } else if (fh) pts.push([p2[0], p1[1]]);
    else pts.push([p1[0], p2[1]]);
    pts.push(p2, p3);
    // Drop repeated and collinear points.
    const out = [];
    for (const p of pts) {
      if (out.length && out.at(-1)[0] === p[0] && out.at(-1)[1] === p[1]) continue;
      if (out.length >= 2) {
        const [a, b] = [out.at(-2), out.at(-1)];
        if ((a[0] === b[0] && b[0] === p[0]) || (a[1] === b[1] && b[1] === p[1])) out.pop();
      }
      out.push(p);
    }
    return out;
  }
  /** origin: the parent's absolute box; cached points are in the parent's coordinates. */
  function drawConnector(c, mode, ops, bg, origin = { x: 0, y: 0 }) {
    const fb = anchorsOf.get(c.from.node), tb = anchorsOf.get(c.to.node);
    if (!fb || !tb) return;
    const fa = !c.from.anchor || c.from.anchor === "auto" ? autoAnchor(fb, tb) : c.from.anchor;
    const ta = !c.to.anchor || c.to.anchor === "auto" ? autoAnchor(tb, fb) : c.to.anchor;
    const pts = c.points ? c.points.map(([x, y]) => [origin.x + x, origin.y + y]) : route(fb, fa, tb, ta);
    connectorOps(pts, c, color(c.stroke ?? "$text-secondary", mode), bg ?? color("$bg", mode), ops, mode);
  }
  function connectorOps(pts, c, stroke, bg, ops, mode) {
    const sw = c.stroke_width ?? 1.5;
    const end = c.end_arrow ?? "arrow";
    const line = pts.map((p) => [...p]);
    if (end === "arrow") {
      const [a, b] = [line.at(-2), line.at(-1)];
      const len = Math.hypot(b[0] - a[0], b[1] - a[1]) || 1;
      const ux = (b[0] - a[0]) / len, uy = (b[1] - a[1]) / len;
      const tip = [...b];
      line[line.length - 1] = [b[0] - ux * ARROW * 0.8, b[1] - uy * ARROW * 0.8];
      const base = [tip[0] - ux * ARROW, tip[1] - uy * ARROW];
      const half = ARROW * 0.45;
      ops.push({ op: "path", d: `M${r2(tip[0])} ${r2(tip[1])} L${r2(base[0] - uy * half)} ${r2(base[1] + ux * half)} L${r2(base[0] + uy * half)} ${r2(base[1] - ux * half)} Z`, fill: stroke, el: c.id });
    }
    ops.splice(ops.length - (end === "arrow" ? 1 : 0), 0, { op: "polyline", points: line.map(([x, y]) => [r2(x), r2(y)]), stroke, sw, dash: c.dash, el: c.id });
    if (c.label) {
      // Label on the middle of the longest segment, on a chip of the background colour.
      let best = 0, bi = 0;
      for (let i = 1; i < pts.length; i++) {
        const l = Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]);
        if (l > best) (best = l), (bi = i);
      }
      const mx = (pts[bi][0] + pts[bi - 1][0]) / 2, my = (pts[bi][1] + pts[bi - 1][1]) / 2;
      const size = 12;
      const tw = measure(c.label, 400, false, size);
      ops.push({ op: "rect", x: r2(mx - tw / 2 - 6), y: r2(my - 10), w: r2(tw + 12), h: 20, r: 4, fill: bg, el: c.id });
      const f = face(400, false);
      ops.push({ op: "text", x: r2(mx - tw / 2), y: r2(my - 10 + (20 - (f.asc + f.desc) * size) / 2 + f.asc * size), text: c.label, weight: 400, italic: false, size, fill: color("$text-secondary", mode), el: c.id });
    }
  }

  /** The display list of one top-level node, and its box in page coordinates. */
  function layoutNode(id) {
    const { n } = index.get(id);
    const w = widthOf(n);
    const h = sizing(n).h === "hug" ? hugH(n, w) : n.h;
    const ops = [];
    anchorsOf.clear();
    draw(n, n.x, n.y, w, h, design.mode, ops);
    return { box: { x: n.x, y: n.y, w, h }, ops };
  }

  /** A whole page: every top-level node, for the pan/zoom canvas. */
  function layoutPage(pageId) {
    const page = design.pages.find((p) => p.id === pageId);
    const ops = [];
    anchorsOf.clear();
    const connectors = [];
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const n of page.children) {
      if (n.type === "connector") { connectors.push(n); continue; }
      const w = widthOf(n);
      const h = sizing(n).h === "hug" ? hugH(n, w) : n.h;
      draw(n, n.x, n.y, w, h, design.mode, ops);
      x0 = Math.min(x0, n.x); y0 = Math.min(y0, n.y); x1 = Math.max(x1, n.x + w); y1 = Math.max(y1, n.y + h);
    }
    for (const c of connectors) drawConnector(c, design.mode, ops);
    return { box: { x: x0, y: y0, w: x1 - x0, h: y1 - y0 }, ops, background: color(page.background, design.mode) };
  }

  return { layoutNode, layoutPage, wrap, color };
}
