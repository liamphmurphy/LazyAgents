"use client";

import { useEffect, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { fitView, panBy, startView, zoomAt, ZOOM_STEP, type View } from "@/lib/mockup-review";
import { Button } from "@/components/ui/button";

/** Movement in px before a press counts as a drag rather than a click. */
const DRAG_PX = 4;
const WHEEL_ZOOM = 0.0015;

type Drag = { id: number; x: number; y: number; moved: boolean };

/**
 * A pannable, zoomable surface for one canvas, like the workflow editor's.
 * `width` is the content's natural width; its height is measured. A drag pans
 * except on `selectable` content, where it must stay free to select text:
 * there it pans from the margins or with Space held.
 */
export function CanvasBoard({ width, selectable, children }: { width: number; selectable: boolean; children: ReactNode }) {
  const host = useRef<HTMLDivElement>(null);
  const world = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<View | null>(null);
  const [panning, setPanning] = useState(false);
  const drag = useRef<Drag | null>(null);
  const space = useRef(false);
  const justDragged = useRef(false);

  const sizes = () => {
    const h = host.current;
    const w = world.current;
    return h && w ? { drawing: { w: w.offsetWidth, h: w.offsetHeight }, viewport: { w: h.clientWidth, h: h.clientHeight } } : null;
  };

  // The first layout places the content; after that the view is the person's.
  useLayoutEffect(() => {
    const s = sizes();
    // eslint-disable-next-line react-hooks/set-state-in-effect -- needs the laid-out size, which only exists after commit
    if (s && s.viewport.w) setView(startView(s.drawing, s.viewport));
  }, []);

  // A scroll zooms a drawing; a document scrolls, and pinch or ctrl zooms it.
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      setView((v) => {
        if (!v) return v;
        if (selectable && !e.ctrlKey && !e.metaKey) return panBy(v, -e.deltaX, -e.deltaY);
        return zoomAt(v, Math.exp(-e.deltaY * WHEEL_ZOOM * (e.ctrlKey ? 6 : 1)), e.clientX - r.left, e.clientY - r.top);
      });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [selectable]);

  useEffect(() => {
    const typing = (t: EventTarget | null) => !!(t as Element | null)?.closest?.("input, textarea, select, button");
    const down = (e: KeyboardEvent) => {
      if (e.code !== "Space" || typing(e.target)) return;
      e.preventDefault();
      space.current = true;
    };
    const up = (e: KeyboardEvent) => {
      if (e.code === "Space") space.current = false;
    };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, []);

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    const t = e.target as Element;
    if (t.closest("button, a, input, textarea, select")) return;
    const onContent = !!world.current?.contains(t);
    if (e.button === 1 || (e.button === 0 && (space.current || !selectable || !onContent))) drag.current = { id: e.pointerId, x: e.clientX, y: e.clientY, moved: false };
  };

  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const d = drag.current;
    if (!d || d.id !== e.pointerId) return;
    if (!d.moved) {
      if (Math.hypot(e.clientX - d.x, e.clientY - d.y) < DRAG_PX) return;
      // Captured only now, so a plain click still reaches the element under it.
      d.moved = true;
      e.currentTarget.setPointerCapture(e.pointerId);
      setPanning(true);
    }
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    d.x = e.clientX;
    d.y = e.clientY;
    setView((v) => v && panBy(v, dx, dy));
  };

  const endDrag = () => {
    justDragged.current = !!drag.current?.moved;
    drag.current = null;
    setPanning(false);
  };

  const zoomBy = (factor: number) => {
    const s = sizes();
    if (s) setView((v) => v && zoomAt(v, factor, s.viewport.w / 2, s.viewport.h / 2));
  };
  const fit = () => {
    const s = sizes();
    if (s) setView(fitView(s.drawing, s.viewport));
  };

  const gap = 20 * (view?.scale ?? 1);
  return (
    <div
      ref={host}
      data-testid="canvas-board"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onClickCapture={(e) => {
        // The click that ends a drag isn't a click on what's under it.
        if (justDragged.current) e.stopPropagation();
        justDragged.current = false;
      }}
      className={cx("absolute inset-0 touch-none overflow-hidden", panning ? "cursor-grabbing" : "cursor-grab", !selectable && "select-none")}
      style={{
        backgroundImage: "radial-gradient(circle, var(--border-strong) 1px, transparent 1px)",
        backgroundSize: `${gap}px ${gap}px`,
        backgroundPosition: `${view?.x ?? 0}px ${view?.y ?? 0}px`,
      }}
    >
      <div
        ref={world}
        data-testid="canvas-board-content"
        className={cx("absolute left-0 top-0 origin-top-left", selectable && "cursor-auto")}
        style={{ width, transform: view ? `translate(${view.x}px, ${view.y}px) scale(${view.scale})` : undefined, visibility: view ? undefined : "hidden" }}
      >
        {children}
      </div>
      <div role="group" aria-label="Zoom" className="absolute bottom-3 left-3 z-10 flex cursor-auto items-center gap-1 rounded-lg border border-border-float bg-surface-raised p-1 shadow-[var(--shadow-float)]">
        <Button variant="secondary" aria-label="Zoom out" onClick={() => zoomBy(1 / ZOOM_STEP)}>−</Button>
        <span data-testid="canvas-zoom" className="w-12 text-center text-xs text-fg-secondary">{Math.round((view?.scale ?? 1) * 100)}%</span>
        <Button variant="secondary" aria-label="Zoom in" onClick={() => zoomBy(ZOOM_STEP)}>+</Button>
        <Button variant="secondary" onClick={fit}>Fit</Button>
      </div>
    </div>
  );
}
