"use client";

import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent, type RefObject } from "react";
import { cx } from "@/lib/cx";
import { SlideSvg } from "@/lib/slides/render";
import { H, W, textOf, type Deck, type Element, type Frame, type Paragraph, type Slide } from "@/lib/slides/schema";
import { TextEditor, type TextEditorHandle } from "./text-editor";

type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "rotate" | "move";
const HANDLES: Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
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

/**
 * The slide being edited: the shared renderer underneath, and above it one
 * hit box per element for selecting, moving, resizing and rotating, plus the
 * in-place text editor. Everything is in slide units, scaled to the width.
 */
export function SlideCanvas({ deck, slide, assetUrl, fontsReady, selected, onSelect, editing, onEdit, onFrame, onText, readOnly, editorRef, overflow }: {
  deck: Deck;
  slide: Slide;
  assetUrl: (id: string) => string;
  fontsReady: boolean;
  selected?: string;
  onSelect: (id?: string) => void;
  editing?: string;
  onEdit: (id?: string) => void;
  onFrame: (id: string, frame: Frame, rotation?: number) => void;
  onText: (id: string, paragraphs: Paragraph[]) => void;
  readOnly: boolean;
  editorRef: RefObject<TextEditorHandle | null>;
  overflow: Set<string>;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0.4);
  const [drag, setDrag] = useState<{ id: string; frame: Frame; rotation?: number } | null>(null);
  useLayoutEffect(() => {
    const el = box.current!;
    const ro = new ResizeObserver(() => setScale(el.clientWidth / W));
    ro.observe(el);
    setScale(el.clientWidth / W);
    return () => ro.disconnect();
  }, []);
  // A drag in progress is drawn from its live frame, then committed once.
  const shown: Slide = drag ? { ...slide, elements: slide.elements.map((e) => (e.id === drag.id ? { ...e, frame: drag.frame, rotation: drag.rotation } : e)) } : slide;
  const sel = shown.elements.find((e) => e.id === selected);
  const ed = shown.elements.find((e) => e.id === editing);

  const start = (e: ReactPointerEvent, el: Element, h: Handle) => {
    if (readOnly || e.button !== 0) return;
    e.stopPropagation();
    e.preventDefault();
    onSelect(el.id);
    const x0 = e.clientX, y0 = e.clientY, f0 = el.frame, r0 = el.rotation ?? 0;
    const rect = box.current!.getBoundingClientRect();
    let moved = false;
    let last: { frame: Frame; rotation?: number } = { frame: f0, rotation: el.rotation };
    const onMove = (ev: PointerEvent) => {
      const dx = (ev.clientX - x0) / scale, dy = (ev.clientY - y0) / scale;
      if (!moved && Math.hypot(dx, dy) < 3 / scale) return;
      moved = true;
      if (h === "rotate") {
        const cx = rect.left + (f0.x + f0.w / 2) * scale, cy = rect.top + (f0.y + f0.h / 2) * scale;
        let deg = (Math.atan2(ev.clientY - cy, ev.clientX - cx) * 180) / Math.PI + 90;
        if (ev.shiftKey) deg = Math.round(deg / 15) * 15;
        deg = Math.round(((deg + 540) % 360) - 180);
        last = { frame: f0, rotation: deg || undefined };
      } else {
        // Resize in the element's own (unrotated) axes.
        const rad = (-r0 * Math.PI) / 180;
        const lx = dx * Math.cos(rad) - dy * Math.sin(rad), ly = dx * Math.sin(rad) + dy * Math.cos(rad);
        last = { frame: resized(f0, h, h === "move" ? dx : lx, h === "move" ? dy : ly, ev.shiftKey || el.type === "image"), rotation: el.rotation };
        if (h === "move") last.frame = { ...last.frame, x: Math.round(last.frame.x), y: Math.round(last.frame.y) };
      }
      setDrag({ id: el.id, ...last });
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      setDrag(null);
      if (moved) onFrame(el.id, last.frame, last.rotation);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  };

  // Clicking empty canvas clears the selection.
  useEffect(() => {
    if (editing && !slide.elements.some((e) => e.id === editing)) onEdit(undefined);
  }, [editing, slide, onEdit]);

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Tab" && slide.elements.length) {
      e.preventDefault();
      const i = slide.elements.findIndex((x) => x.id === selected);
      const n = slide.elements.length;
      onSelect(slide.elements[(i + (e.shiftKey ? -1 : 1) + n + (i < 0 && e.shiftKey ? 1 : 0)) % n].id);
    }
  };

  return (
    <div
      ref={box}
      tabIndex={0}
      role="group"
      aria-label="Slide canvas. Tab selects elements; arrow keys move the selection; Enter edits its text."
      onKeyDown={onKey}
      onPointerDown={() => !editing && onSelect(undefined)}
      className="relative aspect-video w-full select-none overflow-hidden rounded-[4px] border border-border bg-[#fff] shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
    >
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
            aria-hidden
            className={cx("absolute", !readOnly && "cursor-move", el.id === selected && "z-10")}
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
          {!readOnly &&
            (sel.type === "line" ? (["nw", "se"] as Handle[]) : HANDLES).map((h) => (
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
