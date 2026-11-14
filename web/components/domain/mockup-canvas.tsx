"use client";

import { createElement, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import type { Zoom } from "@/lib/mockup";
import { sanitizeSvg, type SNode } from "@/lib/svg-sanitize";
import {
  addShape, bringForward, duplicate, findByPath, getText, parseSvg, pathOf, remove, sendBackward, serialize, setAttr, setText,
  translate, type ShapeKind,
} from "@/lib/svg-edit";
import { Button } from "@/components/ui/button";
import { TextInput } from "@/components/ui/primitives";
import { MockupImage } from "@/components/domain/mockup";

const HISTORY_CAP = 100;

/**
 * Undo/redo over whole source strings. `commit` with the same `key` as the
 * previous commit replaces it (typing in one inspector field is one undo step).
 */
export function useSourceHistory(source: string, setSource: (s: string) => void) {
  const cur = useRef(source);
  const past = useRef<string[]>([]);
  const future = useRef<string[]>([]);
  const lastKey = useRef<string | null>(null);
  const [depth, setDepth] = useState({ undo: 0, redo: 0 });

  useEffect(() => {
    cur.current = source;
  }, [source]);

  const set = (s: string) => {
    cur.current = s;
    setSource(s);
    setDepth({ undo: past.current.length, redo: future.current.length });
  };
  return {
    canUndo: depth.undo > 0,
    canRedo: depth.redo > 0,
    commit(next: string, key?: string) {
      if (next === cur.current) return;
      if (!(key && key === lastKey.current)) {
        past.current.push(cur.current);
        if (past.current.length > HISTORY_CAP) past.current.shift();
      }
      lastKey.current = key ?? null;
      future.current = [];
      set(next);
    },
    undo() {
      const prev = past.current.pop();
      if (prev === undefined) return;
      future.current.push(cur.current);
      lastKey.current = null;
      set(prev);
    },
    redo() {
      const next = future.current.pop();
      if (next === undefined) return;
      past.current.push(cur.current);
      lastKey.current = null;
      set(next);
    },
    breakCoalesce() {
      lastKey.current = null;
    },
  };
}
export type SourceHistory = ReturnType<typeof useSourceHistory>;

const checker =
  "bg-surface-sunken [background-image:conic-gradient(var(--border-subtle)_25%,transparent_0_50%,var(--border-subtle)_0_75%,transparent_0)] [background-size:16px_16px]";

export function renderNode(n: SNode, key?: number): ReactNode {
  if (n.tag === "#text") return n.text;
  return createElement(n.tag, { key, ...n.attrs, "data-path": n.path }, ...n.children.map(renderNode));
}

export function viewBoxOf(attrs: Record<string, string>): { x: number; y: number; w: number; h: number } {
  const vb = attrs.viewBox?.trim().split(/[\s,]+/).map(Number);
  if (vb && vb.length === 4 && vb.every(Number.isFinite) && vb[2] > 0 && vb[3] > 0) return { x: vb[0], y: vb[1], w: vb[2], h: vb[3] };
  const num = (v?: string) => (v && /^[\d.]+(px)?$/.test(v.trim()) ? parseFloat(v) : NaN);
  const w = num(attrs.width);
  const h = num(attrs.height);
  return { x: 0, y: 0, w: w > 0 ? w : 1280, h: h > 0 ? h : 800 };
}

type Drag = { path: string; el: SVGGraphicsElement; inv: DOMMatrix; sx: number; sy: number; cx: number; cy: number; orig: string | null; moved: boolean; dx: number; dy: number };
type Editing = { path: string; value: string; left: number; top: number; width: number; height: number };
type Op = (doc: Document, el: Element | null) => string | null | undefined | false;

const GEOMETRY: Record<string, string[]> = {
  rect: ["x", "y", "width", "height"],
  image: ["x", "y", "width", "height"],
  ellipse: ["cx", "cy", "rx", "ry"],
  circle: ["cx", "cy", "r"],
  line: ["x1", "y1", "x2", "y2"],
  text: ["x", "y"],
};

const focusInput = (n: HTMLInputElement | null) => {
  n?.focus();
  n?.select();
};

/**
 * Visual SVG editor. Renders a sanitized React tree (never the parsed nodes or
 * innerHTML) and edits by parsing `source`, mutating a fresh document and
 * re-serializing through `history.commit`.
 */
export function MockupCanvas({
  source,
  history,
  zoom,
  toolbarEnd,
  alt,
  inspectorSide,
}: {
  source: string;
  history: SourceHistory;
  zoom: Zoom;
  /** Rendered at the right end of the toolbar (the zoom control). */
  toolbarEnd?: ReactNode;
  alt: string;
  inspectorSide: boolean;
}) {
  const prefix = `mk-${useId().replace(/\W/g, "")}-`;
  const doc = useMemo(() => parseSvg(source), [source]);
  const san = useMemo(() => (doc ? sanitizeSvg(doc.documentElement, prefix) : null), [doc, prefix]);
  const tree = san?.tree ?? null;
  const children = useMemo(() => tree?.children.map(renderNode) ?? [], [tree]);

  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [showImage, setShowImage] = useState(false);
  const svgRef = useRef<SVGSVGElement>(null);
  const innerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const outline = useRef<SVGRectElement>(null);
  const drag = useRef<Drag | null>(null);
  const editDone = useRef(false);

  const selEl = doc && selected ? findByPath(doc, selected) : null;
  const selPath = selEl ? selected : null;
  const parentPath = selPath && selPath.includes(".") ? selPath.slice(0, selPath.lastIndexOf(".")) : null;
  const vb = tree ? viewBoxOf(tree.attrs) : { x: 0, y: 0, w: 1280, h: 800 };

  // Selection outline: drawn inside the svg as a non-exported, non-interactive
  // rect, positioned imperatively from the rendered element's bounding box.
  const updateOutline = () => {
    const rect = outline.current;
    const svg = svgRef.current;
    if (!rect || !svg) return;
    const target = selPath ? svg.querySelector(`[data-path="${selPath}"]`) : null;
    try {
      const root = svg.getScreenCTM();
      const own = target instanceof SVGGraphicsElement ? target.getScreenCTM() : null;
      if (!root || !own || !(target instanceof SVGGraphicsElement)) throw new Error("hidden");
      const m = root.inverse().multiply(own);
      const b = target.getBBox();
      const pts = [
        new DOMPoint(b.x, b.y), new DOMPoint(b.x + b.width, b.y), new DOMPoint(b.x, b.y + b.height), new DOMPoint(b.x + b.width, b.y + b.height),
      ].map((p) => p.matrixTransform(m));
      const x0 = Math.min(...pts.map((p) => p.x));
      const y0 = Math.min(...pts.map((p) => p.y));
      const x1 = Math.max(...pts.map((p) => p.x));
      const y1 = Math.max(...pts.map((p) => p.y));
      rect.setAttribute("x", String(x0 - 2));
      rect.setAttribute("y", String(y0 - 2));
      rect.setAttribute("width", String(x1 - x0 + 4));
      rect.setAttribute("height", String(y1 - y0 + 4));
      rect.setAttribute("visibility", "visible");
    } catch {
      rect.setAttribute("visibility", "hidden");
    }
  };
  useLayoutEffect(updateOutline);

  /** Parse the current source, run an edit on the element at `path`, commit the result. */
  const apply = (path: string | null, op: Op, key?: string) => {
    const d = parseSvg(source);
    if (!d) return;
    const out = op(d, path ? findByPath(d, path) : null);
    if (out === false) return;
    history.commit(serialize(d), key);
    if (out !== undefined) setSelected(out);
  };

  const hitPath = (e: { target: EventTarget }) => (e.target as Element).closest?.("[data-path]") ?? null;

  const onPointerDown = (e: PointerEvent<SVGSVGElement>) => {
    if (e.button !== 0 || editing) return;
    history.breakCoalesce();
    const t = hitPath(e);
    const path = t?.getAttribute("data-path");
    if (!t || !path) {
      setSelected(null);
      return;
    }
    setSelected(path);
    const el = t as SVGGraphicsElement;
    const ctm = (el.parentNode as SVGGraphicsElement | null)?.getScreenCTM?.();
    if (!ctm) return;
    const inv = ctm.inverse();
    const p = new DOMPoint(e.clientX, e.clientY).matrixTransform(inv);
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { path, el, inv, sx: p.x, sy: p.y, cx: e.clientX, cy: e.clientY, orig: el.getAttribute("transform"), moved: false, dx: 0, dy: 0 };
  };

  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    const d = drag.current;
    if (!d) return;
    if (!d.moved && Math.hypot(e.clientX - d.cx, e.clientY - d.cy) < 3) return;
    d.moved = true;
    const p = new DOMPoint(e.clientX, e.clientY).matrixTransform(d.inv);
    d.dx = p.x - d.sx;
    d.dy = p.y - d.sy;
    // Live feedback on the rendered element only; the commit rewrites the source.
    d.el.setAttribute("transform", `translate(${d.dx} ${d.dy})${d.orig ? " " + d.orig : ""}`);
    updateOutline();
  };

  const endDrag = (commit: boolean) => {
    const d = drag.current;
    drag.current = null;
    if (!d?.moved) return;
    if (d.orig == null) d.el.removeAttribute("transform");
    else d.el.setAttribute("transform", d.orig);
    if (commit) apply(d.path, (_, el) => (el ? (translate(el, d.dx, d.dy), undefined) : false));
    else updateOutline();
  };

  const onDoubleClick = (e: React.MouseEvent<SVGSVGElement>) => {
    let path = hitPath(e)?.getAttribute("data-path");
    if (!doc || !path) return;
    let el = findByPath(doc, path);
    if (el?.localName === "tspan" && path.includes(".")) {
      path = path.slice(0, path.lastIndexOf("."));
      el = findByPath(doc, path);
    }
    const shown = svgRef.current?.querySelector(`[data-path="${path}"]`);
    const inner = innerRef.current;
    if (!el || el.localName !== "text" || !shown || !inner) return;
    const r = shown.getBoundingClientRect();
    const o = inner.getBoundingClientRect();
    editDone.current = false;
    setSelected(path);
    setEditing({ path, value: getText(el), left: r.left - o.left, top: r.top - o.top, width: r.width, height: r.height });
  };

  const finishEdit = (commit: boolean) => {
    if (!editing || editDone.current) return;
    editDone.current = true;
    const { path, value } = editing;
    setEditing(null);
    canvasRef.current?.focus({ preventScroll: true });
    if (commit) apply(path, (_, el) => (el ? (setText(el, value), undefined) : false));
  };

  const center = { x: vb.x + vb.w / 2, y: vb.y + vb.h / 2 };
  const add = (kind: ShapeKind) => apply(null, (d) => pathOf(d, addShape(d, kind, center)));
  const dup = () => selPath && apply(selPath, (d, el) => { const c = el && duplicate(el); return c ? pathOf(d, c) : false; });
  const del = () => selPath && apply(selPath, (_, el) => (el && remove(el) ? null : false));
  const reorder = (fn: (el: Element) => boolean) => selPath && apply(selPath, (d, el) => (el && fn(el) ? pathOf(d, el) : false));
  const nudge = (dx: number, dy: number) => selPath && apply(selPath, (_, el) => (el ? (translate(el, dx, dy), undefined) : false));

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    const t = e.target as HTMLElement;
    if (t.closest("input, textarea, select, [contenteditable]")) return;
    const mod = e.metaKey || e.ctrlKey;
    const k = e.key.toLowerCase();
    if (mod && (k === "z" || k === "y")) {
      e.preventDefault();
      if (k === "y" || e.shiftKey) history.redo();
      else history.undo();
    } else if (mod && k === "d") {
      e.preventDefault();
      dup();
    } else if (!mod && !e.altKey && canvasRef.current?.contains(t) && selPath) {
      const step = e.shiftKey ? 10 : 1;
      const arrows: Record<string, [number, number]> = { arrowleft: [-step, 0], arrowright: [step, 0], arrowup: [0, -step], arrowdown: [0, step] };
      if (arrows[k]) {
        e.preventDefault();
        nudge(...arrows[k]);
      } else if (k === "delete" || k === "backspace") {
        e.preventDefault();
        del();
      } else if (k === "escape") setSelected(null);
    }
  };

  if (!tree) {
    return <p className="rounded-xl border border-border bg-surface-sunken p-4 text-13 text-fg-tertiary">Visual editing needs a well-formed SVG. Fix the markup in Source.</p>;
  }

  const { width: _w, height: _h, x: _x, y: _y, ...rootAttrs } = tree.attrs;
  void [_w, _h, _x, _y];
  const scale = zoom === "200" ? 2 : 1;
  const size = zoom === "fit" ? { width: "100%", height: "auto", aspectRatio: `${vb.w} / ${vb.h}` } : { width: vb.w * scale, height: vb.h * scale, maxWidth: "none" };

  const tb = (label: string, onClick: () => void, opts: { disabled?: boolean; hint?: string; icon?: "plus" | "trash" } = {}) => (
    <Button variant="outline" icon={opts.icon} hint={opts.hint} disabled={opts.disabled} onClick={onClick} title={opts.hint ? `${label} (${opts.hint})` : label} aria-label={label}>
      {label}
    </Button>
  );

  return (
    <div className={cx("flex min-w-0 flex-1 gap-3", inspectorSide ? "flex-col xl:flex-row" : "flex-col")} onKeyDown={onKeyDown} onBlur={() => history.breakCoalesce()}>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex flex-wrap items-center gap-1.5" role="toolbar" aria-label="Mockup tools">
          {tb("Rectangle", () => add("rect"), { icon: "plus" })}
          {tb("Ellipse", () => add("ellipse"), { icon: "plus" })}
          {tb("Line", () => add("line"), { icon: "plus" })}
          {tb("Text", () => add("text"), { icon: "plus" })}
          <span className="mx-1 h-5 w-px bg-border" aria-hidden />
          {tb("Duplicate", dup, { disabled: !selPath, hint: "⌘D" })}
          {tb("Delete", del, { disabled: !selPath, icon: "trash" })}
          {tb("Forward", () => reorder(bringForward), { disabled: !selPath })}
          {tb("Back", () => reorder(sendBackward), { disabled: !selPath })}
          {tb("Parent", () => setSelected(parentPath), { disabled: parentPath == null })}
          <span className="mx-1 h-5 w-px bg-border" aria-hidden />
          {tb("Undo", history.undo, { disabled: !history.canUndo, hint: "⌘Z" })}
          {tb("Redo", history.redo, { disabled: !history.canRedo, hint: "⇧⌘Z" })}
          <span className="ml-auto">{toolbarEnd}</span>
        </div>
        {san && san.skipped > 0 && (
          <p className="flex flex-wrap items-center gap-2 text-12 text-fg-tertiary">
            <span>
              {san.skipped} element{san.skipped === 1 ? "" : "s"} can&apos;t be shown in the visual editor; they&apos;re kept in the source. The image preview is authoritative.
            </span>
            <button type="button" className="text-accent hover:underline" onClick={() => setShowImage((v) => !v)}>
              {showImage ? "Back to the editor" : "Show image preview"}
            </button>
          </p>
        )}
        {showImage ? (
          <MockupImage source={source} alt={alt} zoom={zoom} className="min-h-[320px] flex-1" />
        ) : (
          <div
            ref={canvasRef}
            tabIndex={0}
            aria-label="Mockup canvas. Arrow keys nudge the selection, Shift for 10 units, Delete removes it."
            className={cx("min-h-[320px] flex-1 overflow-auto rounded-xl border border-border focus-visible:border-accent", checker)}
          >
            <div ref={innerRef} className="relative mx-auto" style={zoom === "fit" ? undefined : { width: vb.w * scale }}>
              <svg
                ref={svgRef}
                {...rootAttrs}
                viewBox={`${vb.x} ${vb.y} ${vb.w} ${vb.h}`}
                data-path=""
                role="img"
                aria-label={alt}
                className="block max-w-full select-none"
                style={size}
                onPointerDown={onPointerDown}
                onPointerMove={onPointerMove}
                onPointerUp={() => endDrag(true)}
                onPointerCancel={() => endDrag(false)}
                onDoubleClick={onDoubleClick}
              >
                {children}
                <g pointerEvents="none">
                  <rect ref={outline} visibility="hidden" fill="none" vectorEffect="non-scaling-stroke" style={{ stroke: "var(--accent)", strokeWidth: 1.5 }} />
                </g>
              </svg>
              {editing && (
                <input
                  ref={focusInput}
                  aria-label="Edit text"
                  value={editing.value}
                  onChange={(e) => setEditing({ ...editing, value: e.target.value })}
                  onBlur={() => finishEdit(true)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") finishEdit(true);
                    else if (e.key === "Escape") finishEdit(false);
                    e.stopPropagation();
                  }}
                  style={{ position: "absolute", left: editing.left - 4, top: editing.top - 4, minWidth: Math.max(editing.width + 24, 96), height: Math.max(editing.height + 8, 26) }}
                  className="rounded-sm border border-accent bg-surface px-1.5 text-13 text-fg focus:outline-none"
                />
              )}
            </div>
          </div>
        )}
      </div>
      <Inspector el={selEl} path={selPath} apply={apply} wide={inspectorSide} />
    </div>
  );
}

const HEX = /^#[0-9a-f]{6}$/i;
function toHex(v: string): string {
  if (HEX.test(v)) return v;
  const m = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v);
  return m ? `#${m[1]}${m[1]}${m[2]}${m[2]}${m[3]}${m[3]}` : "#000000";
}

function Inspector({ el, path, apply, wide }: { el: Element | null; path: string | null; apply: (path: string | null, op: Op, key?: string) => void; wide: boolean }) {
  const shell = cx("flex-none rounded-xl border border-border bg-surface p-3", wide && "xl:w-[240px]");
  if (!el || !path) {
    return (
      <aside className={shell} aria-label="Inspector">
        <p className="text-12 text-fg-tertiary">Select an element on the canvas to edit it. Drag to move, double-click text to edit it.</p>
      </aside>
    );
  }
  const tag = el.localName;
  const set = (name: string, v: string) => apply(path, (_, e) => (e ? (setAttr(e, name, v), undefined) : false), `attr:${path}:${name}`);
  const attr = (name: string) => el.getAttribute(name) ?? "";
  const text = (id: string, label: string, name: string) => (
    <label key={name} className="flex flex-col gap-1 text-12 text-fg-secondary" htmlFor={`${id}-${name}`}>
      {label}
      <TextInput mono id={`${id}-${name}`} value={attr(name)} onChange={(e) => set(name, e.target.value)} className="!h-7" />
    </label>
  );
  const color = (label: string, name: string) => (
    <div key={name} className="flex flex-col gap-1 text-12 text-fg-secondary">
      <label htmlFor={`mk-insp-${name}`}>{label}</label>
      <div className="flex items-center gap-1.5">
        <input type="color" aria-label={`${label} colour`} value={toHex(attr(name))} onChange={(e) => set(name, e.target.value.toUpperCase())} className="h-7 w-8 flex-none cursor-pointer rounded-sm border border-border-strong bg-surface p-0.5" />
        <TextInput mono id={`mk-insp-${name}`} value={attr(name)} placeholder="inherit" onChange={(e) => set(name, e.target.value)} className="!h-7" />
      </div>
    </div>
  );
  const geometry = GEOMETRY[tag] ?? [];
  return (
    <aside className={cx(shell, "flex flex-col gap-2.5")} aria-label="Inspector" key={path}>
      <h3 className="font-mono text-13 text-fg-2">&lt;{tag}&gt;</h3>
      {tag === "text" && (
        <label className="flex flex-col gap-1 text-12 text-fg-secondary" htmlFor="mk-insp-content">
          Text
          <TextInput id="mk-insp-content" value={getText(el)} onChange={(e) => apply(path, (_, x) => (x ? (setText(x, e.target.value), undefined) : false), `text:${path}`)} className="!h-7" />
        </label>
      )}
      {color("Fill", "fill")}
      {color("Stroke", "stroke")}
      {tag !== "text" && tag !== "tspan" && text("mk-insp", "Stroke width", "stroke-width")}
      {(tag === "text" || tag === "tspan") && text("mk-insp", "Font size", "font-size")}
      {geometry.length > 0 && (
        <div className="grid grid-cols-2 gap-2">{geometry.map((g) => text("mk-insp", g, g))}</div>
      )}
    </aside>
  );
}
