"use client";

import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { cx } from "@/lib/cx";
import { connectNodes, find, newId, removeNodes, insertNodes, mapNodes, newFrame, newSection, newShape, newSticky, newText, pathFrom, reparent, resizeIn, withPlainText, plainText, inAutoLayout, wrapInSection, PRESETS, STICKY_SIZE } from "@/lib/designs/edit";
import { connectTarget, dropTarget, hitPath, inMarquee, pick } from "@/lib/designs/hit";
import { STICKY_COLORS, STICKY_INSET, STICKY_TEXT, type Box, type LaidPage } from "@/lib/designs/layout";
import { FAMILY } from "@/lib/designs/measure";
import { LaidPageSvg } from "@/lib/designs/render";
import { screens } from "@/lib/designs/prototype";
import type { Design, DesignNode, Page, StickyNode, TextNode } from "@/lib/designs/schema";

export type Tool = "select" | "hand" | "frame" | "rect" | "ellipse" | "line" | "pen" | "text" | "sticky" | "section" | "connector";
/** screen = world × z + (x, y). */
export type Camera = { x: number; y: number; z: number };
export const MIN_ZOOM = 0.02;
export const MAX_ZOOM = 8;
export const clampZoom = (z: number) => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z));

/** Zooms about a screen point, keeping the world point under it still. */
export const zoomAt = (c: Camera, z: number, sx: number, sy: number): Camera => {
  const nz = clampZoom(z);
  return { z: nz, x: sx - ((sx - c.x) / c.z) * nz, y: sy - ((sy - c.y) / c.z) * nz };
};
/** The camera that fits a box into a viewport with a margin. */
export function fitCamera(b: Box, vw: number, vh: number, margin = 64): Camera {
  if (!b.w || !b.h) return { x: vw / 2 - b.x, y: vh / 2 - b.y, z: 1 };
  const z = clampZoom(Math.min((vw - 2 * margin) / b.w, (vh - 2 * margin) / b.h, 1));
  return { z, x: (vw - b.w * z) / 2 - b.x * z, y: (vh - b.h * z) / 2 - b.y * z };
}

type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";
const HANDLES: Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
const handlePoint = (b: Box, h: Handle): [number, number] => [b.x + (h.includes("w") ? 0 : h.includes("e") ? b.w : b.w / 2), b.y + (h.includes("n") ? 0 : h.includes("s") ? b.h : b.h / 2)];
const CURSOR: Record<Handle, string> = { nw: "nwse-resize", se: "nwse-resize", ne: "nesw-resize", sw: "nesw-resize", n: "ns-resize", s: "ns-resize", e: "ew-resize", w: "ew-resize" };

type Drag =
  | { kind: "pan"; sx: number; sy: number; cam: Camera }
  | { kind: "move"; wx: number; wy: number; start: Map<string, { x: number; y: number }>; key: string; moved: boolean }
  | { kind: "resize"; id: string; handle: Handle; wx: number; wy: number; box: Box; origin: { x: number; y: number }; key: string }
  | { kind: "marquee"; wx: number; wy: number; x: number; y: number; additive: boolean; base: string[] }
  | { kind: "create"; tool: "frame" | "rect" | "ellipse" | "line" | "section"; wx: number; wy: number; x: number; y: number }
  | { kind: "connect"; from: string; wx: number; wy: number; x: number; y: number; over?: string };

export type CanvasProps = {
  design: Design;
  page: Page;
  laid: LaidPage;
  selected: string[];
  onSelect: (ids: string[]) => void;
  tool: Tool;
  onTool: (t: Tool) => void;
  readOnly: boolean;
  assetUrl: (a: string) => string;
  camera: Camera;
  onCamera: (c: Camera | ((c: Camera) => Camera)) => void;
  /** Changes the working copy; key coalesces repeated edits into one undo step. */
  onEdit: (fn: (d: Design) => Design, key?: string) => void;
  editing?: string;
  onEditing: (id?: string) => void;
  onViewport?: (w: number, h: number) => void;
  remoteCursor?: never;
  /** Draws prototype links as wires (the Prototype tab). */
  wires?: boolean;
};

/** The infinite canvas: the page drawn once in world space under a camera transform, with overlays in screen space. */
export function DesignCanvas(p: CanvasProps) {
  const { design, page, laid, selected, camera, tool, readOnly } = p;
  const wrap = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ w: 1200, h: 800 });
  const [drag, setDrag] = useState<Drag | null>(null);
  const [hover, setHover] = useState<string>();
  const [pen, setPen] = useState<[number, number][]>([]);
  const [penAt, setPenAt] = useState<[number, number]>();
  const [space, setSpace] = useState(false);
  // Numbers each drag, so its edits coalesce into one undo step.
  const gesture = useRef(0);
  // Latest props for handlers registered once (wheel) and for drags across renders.
  const live = useRef(p);
  useEffect(() => {
    live.current = p;
  });

  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => {
      const r = e.contentRect;
      setSize({ w: r.width, h: r.height });
      live.current.onViewport?.(r.width, r.height);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Wheel: pan, or zoom about the pointer with ⌘/Ctrl (and trackpad pinch, which arrives as ctrl+wheel).
  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      const sx = e.clientX - r.left, sy = e.clientY - r.top;
      const scale = e.deltaMode === 1 ? 16 : 1;
      if (e.ctrlKey || e.metaKey) live.current.onCamera((c) => zoomAt(c, c.z * Math.exp(-e.deltaY * scale * 0.0025), sx, sy));
      else live.current.onCamera((c) => ({ ...c, x: c.x - (e.shiftKey ? e.deltaY : e.deltaX) * scale, y: c.y - (e.shiftKey ? 0 : e.deltaY) * scale }));
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  // Space held: pan with the pointer, as in design tools.
  useEffect(() => {
    const typing = (t: EventTarget | null) => t instanceof HTMLElement && (t.isContentEditable || t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || t instanceof HTMLSelectElement);
    const down = (e: KeyboardEvent) => {
      if (e.code === "Space" && !typing(e.target)) {
        e.preventDefault();
        setSpace(true);
      }
    };
    const up = (e: KeyboardEvent) => e.code === "Space" && setSpace(false);
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, []);

  // Viewport culling (ADR §3.8): top-level nodes more than a screen off-screen aren't drawn.
  const cullKey = useMemo(() => {
    const x0 = (-camera.x - size.w) / camera.z, y0 = (-camera.y - size.h) / camera.z;
    const x1 = (2 * size.w - camera.x) / camera.z, y1 = (2 * size.h - camera.y) / camera.z;
    return laid.nodes.filter((n) => n.box.x <= x1 && n.box.x + n.box.w >= x0 && n.box.y <= y1 && n.box.y + n.box.h >= y0).map((n) => n.id).join(" ");
  }, [laid, camera, size]);
  const visible = useMemo(() => new Set(cullKey.split(" ")), [cullKey]);

  const toWorld = (clientX: number, clientY: number): [number, number] => {
    const r = wrap.current!.getBoundingClientRect();
    return [(clientX - r.left - camera.x) / camera.z, (clientY - r.top - camera.y) / camera.z];
  };
  const toScreen = (b: Box): Box => ({ x: b.x * camera.z + camera.x, y: b.y * camera.z + camera.y, w: b.w * camera.z, h: b.h * camera.z });
  const parentOrigin = (d: Design, id: string) => {
    const f = find(d, id);
    const pb = f?.parent ? laid.boxes.get(f.parent.id) : undefined;
    return pb ? { x: pb.x, y: pb.y } : { x: 0, y: 0 };
  };
  const single = selected.length === 1 ? laid.boxes.get(selected[0]) : undefined;
  const singleNode = selected.length === 1 ? find(design, selected[0]) : undefined;

  /** Adds a new node where the pointer started: into the board under it (relative to it), else the page. */
  const place = (node: DesignNode, wx: number, wy: number) => {
    const target = dropTarget(page, laid.boxes, wx, wy);
    const tb = target && laid.boxes.get(target.id);
    if (target && tb && node.type !== "connector") Object.assign(node, { x: node.x - tb.x, y: node.y - tb.y });
    p.onEdit((d) => insertNodes(d, target?.id ?? page.id, [node]));
    p.onSelect([node.id]);
  };

  const finishPen = (closed: boolean) => {
    const node = pen.length >= 2 ? pathFrom(pen, closed) : undefined;
    setPen([]);
    setPenAt(undefined);
    if (node) place(node, pen[0][0], pen[0][1]);
    p.onTool("select");
  };
  useEffect(() => {
    if (tool !== "pen" || !pen.length) return;
    const key = (e: KeyboardEvent) => {
      if (e.key === "Enter" || e.key === "Escape") {
        e.preventDefault();
        finishPen(false);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (p.editing) return;
    wrap.current?.focus({ preventScroll: true });
    const [wx, wy] = toWorld(e.clientX, e.clientY);
    const r = wrap.current!.getBoundingClientRect();
    const sx = e.clientX - r.left, sy = e.clientY - r.top;
    e.currentTarget.setPointerCapture(e.pointerId);
    if (e.button === 1 || space || tool === "hand") {
      setDrag({ kind: "pan", sx: e.clientX, sy: e.clientY, cam: camera });
      return;
    }
    if (e.button !== 0) return;
    if (readOnly && tool !== "select") return;
    if (tool === "pen") {
      if (pen.length >= 3 && Math.hypot((pen[0][0] - wx) * camera.z, (pen[0][1] - wy) * camera.z) < 8) return finishPen(true);
      setPen((pts) => [...pts, [Math.round(wx * 100) / 100, Math.round(wy * 100) / 100]]);
      return;
    }
    if (tool === "text") {
      const t = newText(Math.round(wx), Math.round(wy - 10), "", 16);
      place(t, wx, wy);
      p.onEditing(t.id);
      p.onTool("select");
      e.preventDefault(); // the editor autofocuses now; the default mousedown focus would blur (and close) it
      return;
    }
    if (tool === "sticky") {
      const s = newSticky(Math.round(wx - STICKY_SIZE / 2), Math.round(wy - STICKY_SIZE / 2));
      place(s, wx, wy);
      p.onEditing(s.id);
      p.onTool("select");
      e.preventDefault(); // the editor autofocuses now; the default mousedown focus would blur (and close) it
      return;
    }
    if (tool === "connector") {
      const from = connectTarget(hitPath(page, laid.boxes, wx, wy, 2 / camera.z));
      if (from) setDrag({ kind: "connect", from: from.id, wx, wy, x: wx, y: wy });
      return;
    }
    if (tool !== "select") {
      setDrag({ kind: "create", tool, wx, wy, x: wx, y: wy });
      return;
    }
    // A handle of the single selection resizes it.
    if (single && singleNode && !readOnly && singleNode.node.type !== "connector") {
      const s = toScreen(single);
      const h = HANDLES.find((h) => {
        const [hx, hy] = handlePoint(s, h);
        return Math.abs(hx - sx) <= 6 && Math.abs(hy - sy) <= 6;
      });
      if (h) {
        setDrag({ kind: "resize", id: selected[0], handle: h, wx, wy, box: single, origin: parentOrigin(design, selected[0]), key: `resize:${++gesture.current}` });
        return;
      }
    }
    const path = hitPath(page, laid.boxes, wx, wy, 2 / camera.z);
    const target = pick(path, selected, e.metaKey || e.ctrlKey ? "deep" : "click");
    if (!target) {
      setDrag({ kind: "marquee", wx, wy, x: wx, y: wy, additive: e.shiftKey, base: e.shiftKey ? selected : [] });
      if (!e.shiftKey) p.onSelect([]);
      return;
    }
    let ids = selected;
    if (e.shiftKey) ids = selected.includes(target.id) ? selected.filter((s) => s !== target.id) : [...selected, target.id];
    else if (!selected.includes(target.id)) ids = [target.id];
    if (ids !== selected) p.onSelect(ids);
    if (readOnly || e.shiftKey) return;
    const start = new Map<string, { x: number; y: number }>();
    for (const id of ids) {
      const f = find(design, id);
      if (f && f.node.type !== "connector" && !inAutoLayout(f.parent) && !f.node.locked) start.set(id, { x: f.node.x, y: f.node.y });
    }
    setDrag({ kind: "move", wx, wy, start, key: `move:${++gesture.current}`, moved: false });
  };

  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const [wx, wy] = toWorld(e.clientX, e.clientY);
    if (tool === "pen") setPenAt([wx, wy]);
    if (!drag) {
      if (tool === "select" && !space) {
        const path = hitPath(page, laid.boxes, wx, wy, 2 / camera.z);
        const h = pick(path, selected, e.metaKey || e.ctrlKey ? "deep" : "click")?.id;
        if (h !== hover) setHover(h);
      }
      return;
    }
    if (drag.kind === "pan") {
      p.onCamera({ ...drag.cam, x: drag.cam.x + e.clientX - drag.sx, y: drag.cam.y + e.clientY - drag.sy });
    } else if (drag.kind === "move") {
      const dx = Math.round(wx - drag.wx), dy = Math.round(wy - drag.wy);
      if (!drag.moved && Math.hypot(dx * camera.z, dy * camera.z) < 3) return;
      if (!drag.moved) setDrag({ ...drag, moved: true });
      p.onEdit((d) => mapNodes(d, new Set(drag.start.keys()), (n) => {
        const s = drag.start.get(n.id)!;
        return n.type === "connector" ? n : ({ ...n, x: s.x + dx, y: s.y + dy } as DesignNode);
      }), drag.key);
    } else if (drag.kind === "resize") {
      const b = drag.box, h = drag.handle;
      let dx = wx - drag.wx, dy = wy - drag.wy;
      let x0 = b.x, y0 = b.y, x1 = b.x + b.w, y1 = b.y + b.h;
      if (e.shiftKey && h.length === 2 && b.w && b.h) {
        // Keep the aspect ratio on corner handles.
        const k = Math.max((h.includes("e") ? dx : -dx) / b.w, (h.includes("s") ? dy : -dy) / b.h);
        dx = (h.includes("e") ? 1 : -1) * k * b.w;
        dy = (h.includes("s") ? 1 : -1) * k * b.h;
      }
      if (h.includes("w")) x0 = Math.min(x1 - 1, b.x + dx);
      if (h.includes("e")) x1 = Math.max(x0 + 1, b.x + b.w + dx);
      if (h.includes("n")) y0 = Math.min(y1 - 1, b.y + dy);
      if (h.includes("s")) y1 = Math.max(y0 + 1, b.y + b.h + dy);
      const box = { x: Math.round(x0 - drag.origin.x), y: Math.round(y0 - drag.origin.y), w: Math.round(x1 - x0), h: Math.round(y1 - y0) };
      p.onEdit((d) => resizeIn(d, drag.id, box), drag.key);
    } else if (drag.kind === "connect") {
      const over = connectTarget(hitPath(page, laid.boxes, wx, wy, 2 / camera.z));
      setDrag({ ...drag, x: wx, y: wy, over: over && over.id !== drag.from ? over.id : undefined });
    } else {
      setDrag({ ...drag, x: wx, y: wy });
      if (drag.kind === "marquee") {
        const m = { x: Math.min(drag.wx, wx), y: Math.min(drag.wy, wy), w: Math.abs(wx - drag.wx), h: Math.abs(wy - drag.wy) };
        const ids = inMarquee(page.children, laid.boxes, m);
        p.onSelect(drag.additive ? [...new Set([...drag.base, ...ids])] : ids);
      }
    }
  };

  const onPointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag;
    setDrag(null);
    if (!d) return;
    if (d.kind === "move" && d.moved && d.start.size === 1) {
      // Dropped on another board (or out of one onto the page): it moves in, staying where it is.
      const id = [...d.start.keys()][0];
      const cur = live.current;
      const b = cur.laid.boxes.get(id);
      const f = find(cur.design, id);
      if (b && f) {
        const target = dropTarget(cur.page, cur.laid.boxes, b.x + b.w / 2, b.y + b.h / 2, new Set([id]));
        const to = target?.id ?? cur.page.id;
        if (to !== (f.parent?.id ?? cur.page.id) && !(target && inAutoLayout(target))) {
          const tb = target ? cur.laid.boxes.get(target.id) : undefined;
          cur.onEdit((des) => reparent(des, id, to, b, tb ? { x: tb.x, y: tb.y } : { x: 0, y: 0 }), d.key);
        }
      }
    }
    if (d.kind === "connect") {
      const [wx, wy] = toWorld(e.clientX, e.clientY);
      const to = connectTarget(hitPath(page, laid.boxes, wx, wy, 2 / camera.z));
      if (to && to.id !== d.from && connectNodes(design, d.from, to.id)) {
        const id = newId("n");
        p.onEdit((des) => connectNodes(des, d.from, to.id, { id })?.design ?? des);
        p.onSelect([id]);
      }
      p.onTool("select");
      return;
    }
    if (d.kind === "create" && d.tool === "section") {
      const [wx, wy] = toWorld(e.clientX, e.clientY);
      const click = Math.max(Math.abs(wx - d.wx), Math.abs(wy - d.wy)) * camera.z < 4;
      const b = click
        ? { x: Math.round(d.wx), y: Math.round(d.wy), w: 640, h: 400 }
        : { x: Math.round(Math.min(d.wx, wx)), y: Math.round(Math.min(d.wy, wy)), w: Math.round(Math.abs(wx - d.wx)), h: Math.round(Math.abs(wy - d.wy)) };
      const s = newSection(design, b);
      // Drawn over nodes, it takes them in, so moving it moves them.
      p.onEdit((des) => wrapInSection(des, page.id, s));
      p.onSelect([s.id]);
      p.onTool("select");
      return;
    }
    if (d.kind === "create") {
      const [wx, wy] = toWorld(e.clientX, e.clientY);
      const x = Math.min(d.wx, wx), y = Math.min(d.wy, wy);
      let w = Math.abs(wx - d.wx), h = Math.abs(wy - d.wy);
      const click = Math.max(w, h) * camera.z < 4;
      let node: DesignNode | undefined;
      if (d.tool === "frame") {
        const preset = PRESETS[0];
        node = click ? newFrame({ x: Math.round(d.wx), y: Math.round(d.wy), w: preset.w, h: preset.h }, preset.id, preset.label) : newFrame({ x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) });
      } else if (d.tool === "line") {
        node = click ? pathFrom([[d.wx, d.wy], [d.wx + 120, d.wy]], false, "Line") : pathFrom([[d.wx, d.wy], [wx, wy]], false, "Line");
      } else {
        if (click) {
          w = 100;
          h = 100;
        }
        node = newShape(d.tool === "ellipse" ? "ellipse" : "rect", { x: Math.round(click ? d.wx : x), y: Math.round(click ? d.wy : y), w: Math.round(w), h: Math.round(h) });
      }
      if (node) place(node, d.wx, d.wy);
      p.onTool("select");
    }
  };

  const onDoubleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (tool === "pen") return finishPen(false);
    if (tool !== "select") return;
    const [wx, wy] = toWorld(e.clientX, e.clientY);
    const path = hitPath(page, laid.boxes, wx, wy, 2 / camera.z);
    const deepest = path.at(-1);
    const target = pick(path, selected, "deeper");
    if (!readOnly && (deepest?.type === "text" || deepest?.type === "sticky") && (target?.id === deepest.id || selected.includes(deepest.id))) {
      p.onSelect([deepest.id]);
      p.onEditing(deepest.id);
    } else if (target) p.onSelect([target.id]);
  };

  // ---- overlays (screen space) ----
  const boards = page.children.filter((n) => (n.type === "frame" || n.type === "component" || n.type === "section") && n.visible !== false && visible.has(n.id));
  const editingNode = p.editing ? find(design, p.editing)?.node : undefined;
  const editingBox = p.editing ? laid.boxes.get(p.editing) : undefined;
  const cursor = drag?.kind === "pan" || space || tool === "hand" ? (drag ? "grabbing" : "grab") : drag?.kind === "resize" ? CURSOR[drag.handle] : tool === "select" ? "default" : tool === "text" ? "text" : "crosshair";
  const s = single ? toScreen(single) : undefined;
  const preview = drag?.kind === "create" ? toScreen({ x: Math.min(drag.wx, drag.x), y: Math.min(drag.wy, drag.y), w: Math.abs(drag.x - drag.wx), h: Math.abs(drag.y - drag.wy) }) : undefined;
  const marquee = drag?.kind === "marquee" ? toScreen({ x: Math.min(drag.wx, drag.x), y: Math.min(drag.wy, drag.y), w: Math.abs(drag.x - drag.wx), h: Math.abs(drag.y - drag.wy) }) : undefined;
  const hoverBox = hover && !selected.includes(hover) && !drag ? laid.boxes.get(hover) : undefined;
  const connecting = drag?.kind === "connect" ? { from: laid.boxes.get(drag.from), over: drag.over ? laid.boxes.get(drag.over) : undefined, at: toScreen({ x: drag.x, y: drag.y, w: 0, h: 0 }) } : undefined;

  return (
    <div
      ref={wrap}
      tabIndex={0}
      role="application"
      aria-label={`Canvas: ${page.name}`}
      data-testid="design-canvas"
      data-zoom={camera.z.toFixed(3)}
      className="relative h-full w-full touch-none select-none overflow-hidden outline-none"
      style={{
        cursor,
        background: laid.background ?? "var(--color-surface-sunken)",
        // A dot grid that thins out when zoomed far out.
        backgroundImage: camera.z >= 0.2 ? "radial-gradient(color-mix(in srgb, var(--color-fg-tertiary) 35%, transparent) 1px, transparent 1px)" : undefined,
        backgroundSize: `${24 * camera.z * (camera.z < 0.5 ? 4 : 1)}px ${24 * camera.z * (camera.z < 0.5 ? 4 : 1)}px`,
        backgroundPosition: `${camera.x}px ${camera.y}px`,
      }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerLeave={() => setHover(undefined)}
      onDoubleClick={onDoubleClick}
    >
      <svg className="absolute inset-0 h-full w-full" aria-hidden>
        <g data-testid="design-camera" transform={`translate(${camera.x} ${camera.y}) scale(${camera.z})`}>
          <LaidPageSvg laid={laid} uid={`pg${page.id}`} assetUrl={p.assetUrl} visible={visible} />
        </g>
      </svg>
      <svg className="pointer-events-none absolute inset-0 h-full w-full overflow-visible" aria-hidden data-testid="design-overlay">
        {p.wires && <Wires design={design} laid={laid} toScreen={toScreen} selected={selected} />}
        {boards.map((n) => {
          const b = laid.boxes.get(n.id);
          if (!b) return null;
          const sb = toScreen(b);
          if (n.type === "section") return null;
          return (
            <text key={n.id} x={sb.x} y={sb.y - 6} fontSize={11} fill={selected.includes(n.id) ? "var(--color-accent)" : "var(--color-fg-secondary)"} style={{ fontFamily: `"${FAMILY}", Inter, sans-serif` }}>
              {n.name ?? "Frame"}
            </text>
          );
        })}
        {hoverBox && (() => {
          const hb = toScreen(hoverBox);
          return <rect x={hb.x} y={hb.y} width={hb.w} height={hb.h} fill="none" stroke="var(--color-accent)" strokeWidth={1} opacity={0.6} />;
        })()}
        {selected.map((id) => {
          const b = laid.boxes.get(id);
          if (!b) return null;
          const sb = toScreen(b);
          const rot = b.rotation ? `rotate(${b.rotation} ${sb.x + sb.w / 2} ${sb.y + sb.h / 2})` : undefined;
          return <rect key={id} data-selected={id} x={sb.x} y={sb.y} width={sb.w} height={sb.h} fill="none" stroke="var(--color-accent)" strokeWidth={1.5} transform={rot} />;
        })}
        {s && !readOnly && singleNode?.node.type !== "connector" && (
          <>
            {HANDLES.map((h) => {
              const [hx, hy] = handlePoint(s, h);
              return <rect key={h} data-handle={h} x={hx - 4} y={hy - 4} width={8} height={8} rx={1.5} fill="var(--color-surface)" stroke="var(--color-accent)" strokeWidth={1.5} />;
            })}
            <g transform={`translate(${s.x + s.w / 2} ${s.y + s.h + 14})`}>
              <rect x={-36} y={-9} width={72} height={18} rx={4} fill="var(--color-accent)" />
              <text textAnchor="middle" y={4} fontSize={11} fill="var(--color-fg-on-accent, #fff)" data-testid="design-size-badge">
                {Math.round(single!.w)} × {Math.round(single!.h)}
              </text>
            </g>
          </>
        )}
        {preview && <rect x={preview.x} y={preview.y} width={preview.w} height={preview.h} fill="var(--color-accent)" fillOpacity={0.08} stroke="var(--color-accent)" strokeDasharray="4 3" />}
        {marquee && <rect data-testid="design-marquee" x={marquee.x} y={marquee.y} width={marquee.w} height={marquee.h} fill="var(--color-accent)" fillOpacity={0.08} stroke="var(--color-accent)" />}
        {pen.length > 0 && (
          <>
            <polyline points={[...pen, ...(penAt ? [penAt] : [])].map(([x, y]) => `${x * camera.z + camera.x},${y * camera.z + camera.y}`).join(" ")} fill="none" stroke="var(--color-accent)" strokeWidth={1.5} />
            {pen.map(([x, y], i) => (
              <circle key={i} cx={x * camera.z + camera.x} cy={y * camera.z + camera.y} r={i === 0 ? 5 : 3.5} fill="var(--color-surface)" stroke="var(--color-accent)" strokeWidth={1.5} />
            ))}
          </>
        )}
        {connecting?.from && (() => {
          const f = toScreen(connecting.from);
          const o = connecting.over && toScreen(connecting.over);
          return (
            <g data-testid="design-connect-preview">
              <rect x={f.x} y={f.y} width={f.w} height={f.h} fill="none" stroke="var(--color-accent)" strokeWidth={1.5} />
              {o && <rect x={o.x} y={o.y} width={o.w} height={o.h} fill="var(--color-accent)" fillOpacity={0.08} stroke="var(--color-accent)" strokeWidth={1.5} />}
              <path d={`M${f.x + f.w / 2} ${f.y + f.h / 2}L${connecting.at.x} ${connecting.at.y}`} stroke="var(--color-accent)" strokeWidth={1.5} strokeDasharray="4 3" fill="none" />
            </g>
          );
        })()}
      </svg>
      {editingNode?.type === "sticky" && editingBox && (
        <StickyEditor
          key={editingNode.id}
          node={editingNode}
          box={toScreen(editingBox)}
          zoom={camera.z}
          onChange={(text, session) => p.onEdit((d) => mapNodes(d, new Set([editingNode.id]), (x) => (x.type === "sticky" ? { ...x, text } : x)), `sticky:${session}`)}
          onDone={() => p.onEditing(undefined)}
        />
      )}
      {editingNode?.type === "text" && editingBox && (
        <TextEditor
          key={editingNode.id}
          node={editingNode}
          box={toScreen(editingBox)}
          zoom={camera.z}
          // Typing updates the design live (one undo step per editing session), so the canvas shows the real layout.
          onChange={(text, session) => p.onEdit((d) => mapNodes(d, new Set([editingNode.id]), (x) => (x.type === "text" ? { ...x, paragraphs: withPlainText(x.paragraphs, text) } : x)), `text:${session}`)}
          onDone={(text, session) => {
            p.onEditing(undefined);
            // An emptied text goes away rather than leaving an invisible node.
            if (!text.trim()) p.onEdit((d) => removeNodes(d, new Set([editingNode.id])), `text:${session}`);
          }}
        />
      )}
      <div className={cx("pointer-events-none absolute right-3 top-3 rounded-md bg-surface/80 px-2 py-1 text-11 text-fg-tertiary", !readOnly && "hidden")}>Read-only</div>
    </div>
  );
}

/**
 * Prototype links as wires over the canvas (an overlay, ADR §3.8): from the
 * hotspot to its target frame on this page, a labelled stub to a frame on
 * another page, and a red dashed stub for a broken link.
 */
function Wires({ design, laid, toScreen, selected }: { design: Design; laid: LaidPage; toScreen: (b: Box) => Box; selected: string[] }) {
  const frames = new Map(screens(design).map((s) => [s.id, s]));
  return (
    <g data-testid="design-wires">
      <defs>
        <marker id="wire-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M0 0L8 4L0 8z" fill="var(--color-accent)" />
        </marker>
      </defs>
      {design.links.map((l) => {
        const fb = laid.boxes.get(l.from);
        if (!fb) return null;
        const f = toScreen(fb);
        const on = selected.includes(l.from);
        const target = frames.get(l.to);
        const tb = target ? laid.boxes.get(l.to) : undefined;
        const sx = f.x + f.w, sy = f.y + f.h / 2;
        if (!target) {
          return (
            <g key={l.id} data-wire={l.id} data-broken="true">
              <rect x={f.x} y={f.y} width={f.w} height={f.h} fill="none" stroke="var(--color-pill-blocked-fg)" strokeDasharray="3 3" />
              <path d={`M${sx} ${sy}h40`} stroke="var(--color-pill-blocked-fg)" strokeWidth={1.5} strokeDasharray="4 3" fill="none" />
              <text x={sx + 46} y={sy + 4} fontSize={11} fill="var(--color-pill-blocked-fg)">Broken link</text>
            </g>
          );
        }
        if (!tb) {
          return (
            <g key={l.id} data-wire={l.id}>
              <path d={`M${sx} ${sy}h40`} stroke="var(--color-accent)" strokeWidth={1.5} fill="none" markerEnd="url(#wire-arrow)" />
              <text x={sx + 46} y={sy + 4} fontSize={11} fill="var(--color-accent)">{target.pageName} / {target.name}</text>
            </g>
          );
        }
        const t = toScreen(tb);
        // Leave from the side facing the target, and arrive at its near edge.
        const right = t.x >= f.x + f.w / 2;
        const x0 = right ? f.x + f.w : f.x, x1 = right ? t.x : t.x + t.w, y1 = t.y + Math.min(t.h / 2, 40);
        const bend = Math.max(40, Math.abs(x1 - x0) / 2) * (right ? 1 : -1);
        return (
          <g key={l.id} data-wire={l.id} opacity={on || !selected.length ? 1 : 0.55}>
            <rect x={f.x} y={f.y} width={f.w} height={f.h} fill="var(--color-accent)" fillOpacity={0.08} stroke="var(--color-accent)" />
            <circle cx={x0} cy={sy} r={3.5} fill="var(--color-accent)" />
            <path d={`M${x0} ${sy}C${x0 + bend} ${sy} ${x1 - bend} ${y1} ${x1} ${y1}`} stroke="var(--color-accent)" strokeWidth={on ? 2 : 1.5} fill="none" markerEnd="url(#wire-arrow)" />
          </g>
        );
      })}
    </g>
  );
}

/** Editing a text node in place: a caret over the live-rendered text at the canvas zoom; done on blur, Escape or ⌘↵. */
function TextEditor({ node, box, zoom, onChange, onDone }: { node: TextNode; box: Box; zoom: number; onChange: (text: string, session: string) => void; onDone: (text: string, session: string) => void }) {
  const [value, setValue] = useState(() => plainText(node.paragraphs));
  const [session] = useState(() => `${node.id}:${Date.now()}`);
  const run = node.paragraphs[0]?.runs[0];
  const size = (run?.size ?? 16) * zoom;
  const done = useRef(false);
  const finish = () => {
    if (done.current) return;
    done.current = true;
    onDone(value, session);
  };
  return (
    <textarea
      autoFocus
      aria-label="Text"
      data-testid="design-text-editor"
      value={value}
      onChange={(e) => {
        setValue(e.target.value);
        onChange(e.target.value, session);
      }}
      onBlur={finish}
      onPointerDown={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Escape" || (e.key === "Enter" && (e.metaKey || e.ctrlKey))) {
          e.preventDefault();
          finish();
        }
      }}
      spellCheck={false}
      className="absolute resize-none overflow-hidden border-0 bg-transparent p-0 outline outline-1 outline-accent"
      style={{
        left: box.x,
        top: box.y,
        width: Math.max(box.w, 4),
        height: Math.max(box.h, size * 1.2),
        fontFamily: `"${FAMILY}", Inter, sans-serif`,
        fontSize: size,
        fontWeight: run?.weight ?? (run?.bold ? 700 : 400),
        fontStyle: run?.italic ? "italic" : undefined,
        lineHeight: 1.2 * (node.paragraphs[0]?.lineSpacing ?? 1),
        color: "transparent",
        caretColor: "var(--color-accent)",
        textAlign: node.paragraphs[0]?.align === "justify" ? "left" : (node.paragraphs[0]?.align ?? "left"),
        fontKerning: "none",
        whiteSpace: node.sizing?.w === "hug" ? "pre" : "pre-wrap",
      }}
    />
  );
}

/** Editing a sticky's text in place, on the sticky's own colour; done on blur, Escape or ⌘↵. */
function StickyEditor({ node, box, zoom, onChange, onDone }: { node: StickyNode; box: Box; zoom: number; onChange: (text: string, session: string) => void; onDone: () => void }) {
  const [value, setValue] = useState(node.text);
  const [session] = useState(() => `${node.id}:${Date.now()}`);
  return (
    <textarea
      autoFocus
      aria-label="Sticky text"
      data-testid="design-sticky-editor"
      value={value}
      onChange={(e) => {
        setValue(e.target.value);
        onChange(e.target.value, session);
      }}
      onBlur={onDone}
      onPointerDown={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Escape" || (e.key === "Enter" && (e.metaKey || e.ctrlKey))) {
          e.preventDefault();
          onDone();
        }
      }}
      spellCheck={false}
      className="absolute resize-none overflow-hidden rounded border-0 outline outline-1 outline-accent"
      style={{
        left: box.x,
        top: box.y,
        width: box.w,
        height: box.h,
        padding: STICKY_INSET * zoom,
        background: STICKY_COLORS[node.color ?? "yellow"],
        color: STICKY_TEXT,
        fontFamily: `"${FAMILY}", Inter, sans-serif`,
        fontSize: 14 * zoom,
        fontWeight: 500,
        lineHeight: 1.2,
        fontKerning: "none",
      }}
    />
  );
}
