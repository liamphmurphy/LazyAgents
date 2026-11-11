"use client";

import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent, type RefObject } from "react";
import { cx } from "@/lib/cx";
import { SlideSvg } from "@/lib/slides/render";
import { H, W, textOf, type Deck, type Element, type Frame, type Paragraph, type Slide } from "@/lib/slides/schema";
import { boxOf, contains, frameOf, guidesFor, scaleFrame, snapAxis, snapMove, snapTargets, unionBox, type Box, type SnapTargets } from "@/lib/slides/arrange";
import { TextEditor, type TextEditorHandle } from "./text-editor";

type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "rotate" | "move";
const HANDLES: Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
/** A multiple selection resizes from its corners only, keeping its proportions. */
const CORNERS: Handle[] = ["nw", "ne", "se", "sw"];
const LINE_ENDS: Handle[] = ["nw", "se"];
const MIN = 20;

/** The frame after dragging a handle by (dx, dy) slide units. Shift keeps the aspect ratio. */
function resized(f: Frame, h: Handle, dx: number, dy: number, keepRatio: boolean): Frame {
  if (h === "move") return { ...f, x: f.x + dx, y: f.y + dy };
  let { x, y, w, h: hh } = f;
  if (h.includes("w")) {
    x += dx;
    w -= dx;
  }
  if (h.includes("e")) w += dx;
  if (h.includes("n")) {
    y += dy;
    hh -= dy;
  }
  if (h.includes("s")) hh += dy;
  if (keepRatio && h.length === 2 && f.w && f.h) {
    const r = f.w / f.h;
    const nh = w / r;
    if (h.includes("n")) y += hh - nh;
    hh = nh;
  }
  if (w < MIN) {
    if (h.includes("w")) x -= MIN - w;
    w = MIN;
  }
  if (hh < MIN) {
    if (h.includes("n")) y -= MIN - hh;
    hh = MIN;
  }
  return { x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(hh) };
}

const handlePos = (h: Handle) => ({
  left: h.includes("w") ? "0%" : h.includes("e") ? "100%" : "50%",
  top: h.includes("n") ? "0%" : h.includes("s") ? "100%" : "50%",
  cursor: { nw: "nwse-resize", se: "nwse-resize", ne: "nesw-resize", sw: "nesw-resize", n: "ns-resize", s: "ns-resize", e: "ew-resize", w: "ew-resize" }[h as "n"],
});

const kindLabel = (e: Element) => (e.type === "text" ? `Text${e.placeholder ? ` · ${e.placeholder}` : ""}` : e.type === "shape" ? `Shape · ${e.shape}` : e.type === "line" ? "Line" : "Image");


/** Live frames while a drag is in progress, by element id. */
type Live = Record<string, { frame: Frame; rotation?: number }>;
type Guides = { x: number[]; y: number[] };
/** How close, in screen pixels, an edge or centre has to come to snap. */
const SNAP_PX = 6;

/** Snaps the edges a resize handle moves; the returned (dx, dy) correction is in slide units. */
function snapResize(f: Frame, h: Handle, targets: SnapTargets, threshold: number, keepRatio: boolean): { dx: number; dy: number } {
  const xs = h.includes("w") ? [f.x] : h.includes("e") ? [f.x + f.w] : [];
  const ys = h.includes("n") ? [f.y] : h.includes("s") ? [f.y + f.h] : [];
  // Keeping the ratio, the height follows the width: only one axis can snap.
  const sx = snapAxis(xs, targets.x, threshold);
  const sy = keepRatio && xs.length ? undefined : snapAxis(ys, targets.y, threshold);
  return { dx: sx?.d ?? 0, dy: sy?.d ?? 0 };
}

const pct = (b: Box) => ({ left: `${(b.l / W) * 100}%`, top: `${(b.t / H) * 100}%`, width: `${((b.r - b.l) / W) * 100}%`, height: `${(Math.max(b.b - b.t, 1) / H) * 100}%` });

/**
 * The slide being edited: the shared renderer underneath, and above it one
 * hit box per element for selecting, moving, resizing and rotating, plus the
 * in-place text editor. Everything is in slide units, scaled to the width.
 *
 * Shift- or ⌘-click adds or removes an element; dragging on empty canvas
 * selects what the marquee wholly covers. Moving and resizing act on the whole
 * selection and snap to the slide's edges and centre lines and to the other
 * elements (hold ⌘/Ctrl to place freely).
 */
export function SlideCanvas({ deck, slide, assetUrl, fontsReady, selected, onSelect, editing, onEdit, onFrames, onText, readOnly, editorRef, overflow }: {
  deck: Deck;
  slide: Slide;
  assetUrl: (id: string) => string;
  fontsReady: boolean;
  selected: string[];
  onSelect: (ids: string[]) => void;
  editing?: string;
  onEdit: (id?: string) => void;
  /** One change (one undo step) to several elements' frames. */
  onFrames: (frames: Live) => void;
  onText: (id: string, paragraphs: Paragraph[]) => void;
  readOnly: boolean;
  editorRef: RefObject<TextEditorHandle | null>;
  overflow: Set<string>;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0.4);
  const [drag, setDrag] = useState<Live | null>(null);
  const [guides, setGuides] = useState<Guides | null>(null);
  const [marquee, setMarquee] = useState<Box | null>(null);
  useLayoutEffect(() => {
    const el = box.current!;
    const ro = new ResizeObserver(() => setScale(el.clientWidth / W));
    ro.observe(el);
    setScale(el.clientWidth / W);
    return () => ro.disconnect();
  }, []);
  // A drag in progress is drawn from its live frames, then committed once.
  const shown: Slide = drag ? { ...slide, elements: slide.elements.map((e) => (drag[e.id] ? { ...e, ...drag[e.id] } : e)) } : slide;
  const chosen = new Set(selected);
  const sels = shown.elements.filter((e) => chosen.has(e.id));
  const sel = sels.length === 1 ? sels[0] : undefined;
  const group = sels.length > 1 ? unionBox(sels.map((e) => e.frame)) : undefined;
  const ed = shown.elements.find((e) => e.id === editing);

  /** Slide units under a pointer event. */
  const at = (ev: { clientX: number; clientY: number }) => {
    const r = box.current!.getBoundingClientRect();
    return { x: (ev.clientX - r.left) / scale, y: (ev.clientY - r.top) / scale };
  };

  /** Follows the pointer until it's released, calling onMove with the offset in slide units once it has moved 3 px. */
  const follow = (e: ReactPointerEvent, onMove: (dx: number, dy: number, ev: PointerEvent) => void, onUp: (moved: boolean, ev: PointerEvent) => void) => {
    const x0 = e.clientX, y0 = e.clientY;
    let moved = false;
    const move = (ev: PointerEvent) => {
      const dx = (ev.clientX - x0) / scale, dy = (ev.clientY - y0) / scale;
      if (!moved && Math.hypot(dx, dy) < 3 / scale) return;
      moved = true;
      onMove(dx, dy, ev);
    };
    const up = (ev: PointerEvent) => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      onUp(moved, ev);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const start = (e: ReactPointerEvent, el: Element | null, h: Handle) => {
    if (readOnly || e.button !== 0) return;
    e.stopPropagation();
    e.preventDefault();
    // Keyboard shortcuts (nudge, delete, Enter to edit) follow the selection.
    box.current?.focus({ preventScroll: true });
    // Shift- or ⌘-click adds an element to the selection, or takes it out.
    if (h === "move" && el && (e.shiftKey || e.metaKey || e.ctrlKey)) {
      onSelect(chosen.has(el.id) ? selected.filter((id) => id !== el.id) : [...selected, el.id]);
      return;
    }
    // Pressing an unselected element selects it alone; a selected one drags the whole selection.
    const ids = el && !chosen.has(el.id) ? [el.id] : selected;
    if (el && !chosen.has(el.id)) onSelect(ids);
    const els = slide.elements.filter((x) => ids.includes(x.id));
    if (!els.length) return;
    const one = els.length === 1 ? els[0] : undefined;
    const f0 = one ? one.frame : frameOf(unionBox(els.map((x) => x.frame))!);
    const r0 = one?.rotation ?? 0;
    const targets = snapTargets(W, H, slide.elements.filter((x) => !ids.includes(x.id)).map((x) => x.frame));
    const threshold = SNAP_PX / scale;
    const rect = box.current!.getBoundingClientRect();
    let last: Live = {};
    follow(
      e,
      (dx, dy, ev) => {
        const free = ev.metaKey || ev.ctrlKey;
        let g: Guides | null = null;
        if (h === "rotate" && one) {
          const cx = rect.left + (f0.x + f0.w / 2) * scale, cy = rect.top + (f0.y + f0.h / 2) * scale;
          let deg = (Math.atan2(ev.clientY - cy, ev.clientX - cx) * 180) / Math.PI + 90;
          if (ev.shiftKey) deg = Math.round(deg / 15) * 15;
          deg = Math.round(((deg + 540) % 360) - 180);
          last = { [one.id]: { frame: f0, rotation: deg || undefined } };
        } else if (h === "move") {
          let mx = dx, my = dy;
          if (!free) {
            const b = boxOf(f0);
            const s = snapMove({ l: b.l + dx, r: b.r + dx, t: b.t + dy, b: b.b + dy }, targets, threshold);
            mx += s.dx;
            my += s.dy;
            g = s.guides;
          }
          mx = Math.round(mx);
          my = Math.round(my);
          last = Object.fromEntries(els.map((x) => [x.id, { frame: { ...x.frame, x: x.frame.x + mx, y: x.frame.y + my }, rotation: x.rotation }]));
        } else if (one) {
          // Resize in the element's own (unrotated) axes; rotated elements don't snap.
          const keep = ev.shiftKey || one.type === "image";
          const rad = (-r0 * Math.PI) / 180;
          let lx = dx * Math.cos(rad) - dy * Math.sin(rad), ly = dx * Math.sin(rad) + dy * Math.cos(rad);
          let f = resized(f0, h, lx, ly, keep);
          if (!free && !r0) {
            const s = snapResize(f, h, targets, threshold, keep);
            lx += s.dx;
            ly += s.dy;
            f = resized(f0, h, lx, ly, keep);
            g = guidesFor(boxOf(f), targets);
          }
          last = { [one.id]: { frame: f, rotation: one.rotation } };
        } else {
          // A group resizes from its corners, keeping its proportions.
          let lx = dx, ly = dy;
          let f = resized(f0, h, lx, ly, true);
          if (!free) {
            const s = snapResize(f, h, targets, threshold, true);
            lx += s.dx;
            ly += s.dy;
            f = resized(f0, h, lx, ly, true);
            g = guidesFor(boxOf(f), targets);
          }
          const from = boxOf(f0), to = boxOf(f);
          last = Object.fromEntries(els.map((x) => [x.id, { frame: scaleFrame(x.frame, from, to), rotation: x.rotation }]));
        }
        setGuides(g && (g.x.length || g.y.length) ? g : null);
        setDrag(last);
      },
      (moved) => {
        setDrag(null);
        setGuides(null);
        if (moved) onFrames(last);
        // A click (no drag) on one element of a selection selects it alone.
        else if (h === "move" && el && ids.length > 1) onSelect([el.id]);
      },
    );
  };

  // Dragging on empty canvas draws a marquee; a click there clears the selection.
  const startMarquee = (e: ReactPointerEvent) => {
    if (editing || e.button !== 0) return;
    box.current?.focus({ preventScroll: true });
    const add = e.shiftKey || e.metaKey || e.ctrlKey;
    const p0 = at(e);
    let m: Box | null = null;
    follow(
      e,
      (_dx, _dy, ev) => {
        const p = at(ev);
        m = { l: Math.min(p0.x, p.x), t: Math.min(p0.y, p.y), r: Math.max(p0.x, p.x), b: Math.max(p0.y, p.y) };
        setMarquee(m);
      },
      (moved) => {
        setMarquee(null);
        if (!moved || !m) {
          if (!add) onSelect([]);
          return;
        }
        const hit = slide.elements.filter((x) => contains(m!, boxOf(x.frame))).map((x) => x.id);
        onSelect(add ? [...selected, ...hit.filter((id) => !chosen.has(id))] : hit);
      },
    );
  };

  useEffect(() => {
    if (editing && !slide.elements.some((e) => e.id === editing)) onEdit(undefined);
  }, [editing, slide, onEdit]);

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Tab" && slide.elements.length) {
      e.preventDefault();
      const i = slide.elements.findIndex((x) => x.id === selected[selected.length - 1]);
      const n = slide.elements.length;
      onSelect([slide.elements[(i + (e.shiftKey ? -1 : 1) + n + (i < 0 && e.shiftKey ? 1 : 0)) % n].id]);
    }
  };

  return (
    <div
      ref={box}
      tabIndex={0}
      role="group"
      aria-label="Slide canvas. Tab selects elements, Shift-click or ⌘A selects several; arrow keys move the selection; Enter edits its text."
      onKeyDown={onKey}
      onPointerDown={startMarquee}
      className="relative aspect-video w-full select-none overflow-hidden rounded-[4px] border border-border bg-[#fff] shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
    >
      <span role="status" className="sr-only">{sels.length > 1 ? `${sels.length} elements selected` : sel ? `${kindLabel(sel)} selected` : ""}</span>
      <SlideSvg deck={deck} slide={shown} assetUrl={assetUrl} fontsReady={fontsReady} className="absolute inset-0 h-full w-full" showOverflow hideTextOf={editing} />
      {shown.elements.map((el) => {
        const f = el.frame;
        const isLine = el.type === "line";
        // Lines are thin: give them a fatter hit box.
        const pad = isLine ? Math.max(12 / scale, 20) : 0;
        return (
          <div
            key={el.id}
            data-element={el.id}
            data-selected={chosen.has(el.id) || undefined}
            aria-hidden
            className={cx("absolute", !readOnly && "cursor-move")}
            style={{
              left: `${((Math.min(f.x, f.x + f.w) - pad) / W) * 100}%`,
              top: `${((Math.min(f.y, f.y + f.h) - pad) / H) * 100}%`,
              width: `${((Math.abs(f.w) + 2 * pad) / W) * 100}%`,
              height: `${((Math.abs(f.h) + 2 * pad) / H) * 100}%`,
              transform: el.rotation ? `rotate(${el.rotation}deg)` : undefined,
            }}
            onPointerDown={(e) => start(e, el, "move")}
            onDoubleClick={() => !readOnly && textOf(el) && onEdit(el.id)}
          />
        );
      })}
      {/* Outlines of a multiple selection, each on its own rotation, inside the group's box. */}
      {group &&
        sels.map((el) => (
          <div key={el.id} aria-hidden className="pointer-events-none absolute z-10 outline outline-1 outline-accent" style={{ ...pct(boxOf(el.frame)), transform: el.rotation ? `rotate(${el.rotation}deg)` : undefined }} />
        ))}
      {group && (
        <div className="pointer-events-none absolute z-20" style={pct(group)} data-testid="slide-selection-group">
          <div className="absolute inset-0 outline-dashed outline-[1.5px] outline-accent" />
          <span className="absolute -top-6 right-0 whitespace-nowrap rounded-md border border-border bg-surface-raised px-1.5 py-0.5 font-mono text-[10.5px] text-fg-secondary">{sels.length} elements</span>
          {!readOnly && !drag && CORNERS.map((h) => (
              <span
                key={h}
                aria-hidden
                className="pointer-events-auto absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-[2px] border-[1.5px] border-accent bg-[#fff]"
                style={handlePos(h)}
                onPointerDown={(e) => start(e, null, h)}
              />
            ))}
        </div>
      )}
      {sel && !ed && (
        <div
          className="pointer-events-none absolute z-20"
          style={{
            left: `${(Math.min(sel.frame.x, sel.frame.x + sel.frame.w) / W) * 100}%`,
            top: `${(Math.min(sel.frame.y, sel.frame.y + sel.frame.h) / H) * 100}%`,
            width: `${(Math.abs(sel.frame.w) / W) * 100}%`,
            height: `${(Math.max(Math.abs(sel.frame.h), 1) / H) * 100}%`,
            transform: sel.rotation ? `rotate(${sel.rotation}deg)` : undefined,
          }}
        >
          <div className="absolute inset-0 outline outline-[1.5px] outline-accent" />
          {overflow.has(sel.id) && (
            <span className="absolute left-0 top-full mt-1 whitespace-nowrap rounded-[5px] bg-state-human px-1.5 py-0.5 text-[11px] font-semibold text-approve-fg">Text doesn’t fit</span>
          )}
          <span className="absolute -top-6 right-0 whitespace-nowrap rounded-md border border-border bg-surface-raised px-1.5 py-0.5 font-mono text-[10.5px] text-fg-secondary">{kindLabel(sel)}</span>
          {!readOnly && (sel.type === "line" ? LINE_ENDS : HANDLES).map((h) => (
              <span
                key={h}
                aria-hidden
                className="pointer-events-auto absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-[2px] border-[1.5px] border-accent bg-[#fff]"
                style={handlePos(h)}
                onPointerDown={(e) => start(e, sel, h)}
              />
            ))}
          {!readOnly && sel.type !== "line" && (
            <>
              <span className="absolute left-1/2 top-0 h-4 w-px -translate-y-full bg-accent" />
              <span
                aria-hidden
                title="Rotate (Shift snaps to 15°)"
                className="pointer-events-auto absolute left-1/2 top-0 size-3 -translate-x-1/2 -translate-y-[22px] cursor-grab rounded-full border-[1.5px] border-accent bg-[#fff]"
                onPointerDown={(e) => start(e, sel, "rotate")}
              />
            </>
          )}
        </div>
      )}
      {guides && (
        <div aria-hidden data-testid="slide-snap-guides" className="pointer-events-none absolute inset-0 z-30">
          {guides.x.map((x) => <span key={`x${x}`} className="absolute inset-y-0 w-px bg-[#E5468F]" style={{ left: `${(x / W) * 100}%` }} />)}
          {guides.y.map((y) => <span key={`y${y}`} className="absolute inset-x-0 h-px bg-[#E5468F]" style={{ top: `${(y / H) * 100}%` }} />)}
        </div>
      )}
      {marquee && <div aria-hidden data-testid="slide-marquee" className="pointer-events-none absolute z-30 border border-accent bg-accent/10" style={pct(marquee)} />}
      {ed && textOf(ed) && (
        <TextEditor
          key={ed.id}
          ref={editorRef}
          theme={deck.theme}
          box={textOf(ed)!}
          frame={ed.frame}
          rotation={ed.rotation}
          scale={scale}
          onCommit={(paragraphs) => {
            onText(ed.id, paragraphs);
            onEdit(undefined);
          }}
        />
      )}
    </div>
  );
}
